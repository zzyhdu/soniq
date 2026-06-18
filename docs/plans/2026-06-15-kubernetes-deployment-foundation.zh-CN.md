# Kubernetes 部署基础计划

## 背景

Soniq 的目标是企业级、自托管部署。Kubernetes 是目标部署形态之一，但它不是
第一步。Kubernetes 只负责调度和运行容器；如果应用本身还依赖本地文件系统、
缺少 readiness、缺少 migration 策略、配置和 secret 边界不清，直接写 Helm
chart 只会把问题搬进集群。

因此 Kubernetes 引入应先建立 deployment contract，再落 Helm chart。

## 当前判断

当前 Soniq 已经具备：

- Go API 进程。
- Go worker 进程。
- Soniq application Postgres。
- Temporal worker/task queue。
- S3-compatible object storage。
- Docker Compose 本地 Temporal/Postgres。
- 基础 `/healthz`。
- 正在计划中的企业级 observability foundation。

Kubernetes 前置条件已经补齐一部分：

- API/worker/migrate production Docker image。
- S3-compatible object storage provider。
- `/readyz`。
- migration command。
- raw Kubernetes manifests。
- Deployment contract 文档。

后续还缺：

- Helm values schema 和 chart。
- Kubernetes smoke。
- 更完整的 resource requests/limits 调优。
- graceful shutdown 明确化。
- HPA/PDB/NetworkPolicy/topology spread 等生产硬化。

## 核心原则

### 1. API 和 worker 必须 stateless

Kubernetes 下 pod 会被调度到不同节点，也会随时重启。

API 和 worker 不应依赖本地持久文件：

```txt
API pod 上传文件到本地磁盘
worker pod 在另一台机器上读取不到
```

所以 Kubernetes 前必须支持 shared object storage：

```txt
STORAGE_PROVIDER=s3_compatible
```

本地 filesystem 只保留为开发模式。

### 2. 第一版 Helm 不内置所有依赖

第一版 chart 推荐采用 bring your own dependencies：

- 外部 Postgres。
- 外部 Temporal。
- 外部 S3-compatible object storage。
- 外部 ingress controller。

不要第一版就同时管理 Postgres、Temporal、MinIO。这会让 chart 复杂度过高，也会混淆生产责任边界。

后续可以加 optional bundle：

- dev/self-host MinIO。
- dev Postgres。
- self-host Temporal。

但它们不应是第一版生产路径。

### 3. migration 必须是独立 Job

不要让每个 API pod 启动时自动 migrate。

推荐方式：

- Helm pre-install/pre-upgrade Job。
- 或独立 `soniq-migrate` Job。
- 或运维手动执行 migration command。

同一时间只能有一个 migration 执行者。

### 4. readiness 和 observability 是前置条件

Kubernetes 需要知道 pod 是否可以接流量。

`/healthz` 只表示进程活着；`/readyz` 才表示服务依赖可用。

因此 Kubernetes 前应先完成 observability Phase 1/2：

- structured logs。
- request ID。
- `/readyz`。
- debug scripts。

### 5. secret 不进入镜像和默认 values

镜像不包含 `.env`。

Helm 默认 values 不包含真实 secret。

生产 secret 来自：

- Kubernetes Secret。
- External Secrets Operator。
- Vault。
- 云厂商 secret manager。

## 目标部署模型

第一版 Kubernetes 部署只管理 Soniq 自身进程：

```txt
soniq-api Deployment
  -> Service
  -> Ingress, optional

soniq-worker Deployment
  -> polls Temporal task queue

soniq-migrate Job
  -> applies Soniq application migrations only

External dependencies
  -> Soniq Postgres
  -> Temporal frontend
  -> S3-compatible object storage
  -> optional external ASR/LLM providers
```

明确边界：

- Soniq migration 只作用于 Soniq application Postgres。
- 不迁移 Temporal database。
- 不在 chart 中默认创建 Temporal schema。
- 不重新引入 filesystem object storage；部署统一走 S3-compatible object storage。

## 分阶段实现

## Phase K0 — 前置条件

目标：先让应用适合进入 Kubernetes。

依赖：

- Observability Phase 1/2：
  - `slog` structured logs。
  - request ID。
  - `/readyz`。
  - debug purge script。
- S3-compatible storage：
  - MinIO 本地验证。
  - S3/R2/OSS/COS/OBS 兼容配置。
- 明确 migration command。

验收：

- API 和 worker 可以完全通过 env 配置启动。
- 本地文件存储不再是生产部署必需项。
- `/readyz` 可以表达 Postgres/Temporal/object storage/migration 状态。

当前进展：

- 已完成 Observability Phase 1/2：结构化日志、request ID、`/readyz`、purge artifact debug script。
- 已新增容器可执行的 Go migration command：`backend/cmd/migrate`，未来镜像中可编译为 `soniq-migrate`，用于 Kubernetes Job。
- `make migrate` 已切到 Go migration command；旧 Docker Compose migration 脚本已移除，避免 pre-release 兼容路径增加复杂度。

## Phase K1 — Production Docker images

目标：提供可部署的 API 和 worker 镜像。

范围：

- 多阶段 Dockerfile。
- API image。
- Worker image。
- Migration image 或同一 image 的 migration command。
- 非 root 用户运行。
- 最小 runtime layer。
- 镜像不包含 `.env`、本地上传文件、测试数据。
- 支持 `APP_VERSION` 或 build-time release metadata。

建议文件：

```txt
Dockerfile
docker/api.Dockerfile, optional
docker/worker.Dockerfile, optional
.dockerignore
```

镜像命令建议：

```txt
soniq api
soniq worker
soniq migrate
```

如果当前仍是 `go run ./cmd/api` 形态，应先构建二进制：

```txt
soniq-api
soniq-worker
soniq-migrate
```

验收：

- `docker run` 可启动 API。
- `docker run` 可启动 worker。
- 镜像内无 `.env`。
- 容器以非 root 用户运行。
- API `/healthz` 和 `/readyz` 可访问。

当前进展：

- 已新增根 `Dockerfile`，使用多阶段构建产出 `api`、`worker`、`migrate` 三个 target。
- 已新增 `.dockerignore`，排除 `.env`、依赖目录、本地上传数据、本地服务卷和测试素材。
- API/migrate 使用 distroless nonroot runtime；worker 使用 slim Debian nonroot runtime，并包含 `ffmpeg`/`ffprobe`。
- API/worker runtime image 不再设置已移除的 `LOCAL_STORAGE_PATH`。
- `make docker-build-api`、`make docker-build-worker`、`make docker-build-migrate`、`make docker-build` 已作为本地验证入口。
- `soniq-api`、`soniq-worker`、`soniq-migrate` 支持 `--version`，Docker build 可通过 `APP_VERSION`、`VCS_REF`、`BUILD_DATE` 注入 release metadata。

## Phase K2 — S3-compatible storage provider

目标：移除 Kubernetes 部署对本地共享磁盘的依赖。

当前进展：

- 已在 `compose.temporal.yml` 增加本地 MinIO 和一次性 `minio-init`
  bucket 初始化服务，用于 S3-compatible provider 验证。
- 已新增 `STORAGE_PROVIDER=s3_compatible` 配置读取和启动校验。
- 已实现 S3-compatible object store：API upload 使用 `PutObject`，worker
  processing 使用 `GetObject` 下载到临时文件并上传 normalized artifact，删除和
  purge cleanup 使用 `DeleteObject`。
- 已支持为真实 ASR provider 生成 normalized audio presigned URL；
  DashScope native ASR 和 OpenAI-compatible ASR 都走 URL，不再保留本地文件/Base64
  转写路径。
- `/readyz` 在 S3-compatible 模式下会检查 bucket 可访问。
- `STORAGE_PROVIDER=s3_compatible make smoke-postgres-temporal` 可用于本地
  MinIO smoke 验证。
- 文档已补充 Aliyun OSS 示例；当前 provider 使用 S3-compatible 协议配置，
  MinIO 只是本地验证目标，不是唯一生产目标。
- 已在 `.env.example` 记录本地 MinIO 对应的 S3-compatible 配置。

范围：

- K2 当前实现范围已完成；后续可以继续扩展到更多云厂商兼容性验证。

注意：

- worker 处理音频时如果 ffmpeg/ffprobe 需要本地 path，应下载到 pod 临时目录。
- 临时文件必须清理。
- 不将 bucket key 直接暴露给普通用户。

验收：

- API 和 worker 在不同进程/不同机器时仍可完成 upload -> process。
- purge 删除 S3-compatible object。
- MinIO 本地 smoke 通过。

## Phase K3 — Deployment contract

目标：先写清楚部署契约，再写 chart。

新增文档：

```txt
docs/deployment.md
docs/kubernetes.md
```

内容：

- required env。
- optional env。
- Secret 列表。
- ConfigMap 列表。
- external dependency 列表。
- migration 策略。
- storage provider 要求。
- Temporal namespace/task queue 要求。
- backup/restore 责任边界。

必需 Secret：

```txt
POSTGRES_DSN
AUTH_SESSION_SECRET or future equivalent
S3_ACCESS_KEY
S3_SECRET_KEY
TRANSCRIPTION_API_KEY, optional
LLM_API_KEY, optional
DASHSCOPE_API_KEY, optional
SENTRY_DSN, optional
```

当前代码没有独立的 session signing secret；登录 session 使用 Postgres 中的
opaque session token hash，CSRF token 绑定到 session token。后续如果改成 JWT
或签名 session，再把对应 secret 加进 Secret contract。

必需 Config：

```txt
APP_ENV
APP_PUBLIC_URL
API_ADDRESS
TEMPORAL_ADDRESS
TEMPORAL_NAMESPACE
TEMPORAL_TASK_QUEUE
STORAGE_PROVIDER
S3_ENDPOINT
S3_REGION
S3_BUCKET
S3_FORCE_PATH_STYLE
LOG_FORMAT
LOG_LEVEL
```

验收：

- 一个运维人员能根据文档准备外部 Postgres、Temporal、object storage。
- 文档明确哪些数据需要备份。

当前进展：

- 已新增 `docs/deployment.md`，记录进程、外部依赖、migration、ConfigMap/Secret、
  storage、backup 和 readiness 契约。
- 已新增 `docs/kubernetes.md`，记录 raw manifests 的配置、dry-run、apply、
  migrate Job、API/worker 启动和排查方式。

## Phase K3.5 — Raw Kubernetes manifests

目标：先用最小 raw manifests 验证 Kubernetes 资源边界，再抽象成 Helm。

新增目录：

```txt
deploy/kubernetes/base/
```

当前资源：

- `Namespace`：`soniq`。
- `ConfigMap`：非敏感运行时配置。
- `Secret` 示例：敏感配置占位值。
- `Job`：`soniq-migrate`。
- `Deployment`：`soniq-api`。
- `Service`：`soniq-api` ClusterIP。
- `Deployment`：`soniq-worker`。
- `kustomization.yaml`：用于 render base。

- `make k8s-render` 可渲染 raw manifests。
- 有可用集群时，`make k8s-dry-run` 可执行 server-side dry-run。
- `make k8s-smoke` 可执行本地 manifest smoke：渲染 base manifests，并断言
  资源集合、ConfigMap/Secret keys、Deployment/Job 基础运行约束、API probes 和
  Service selector。
- 资源不包含 Postgres、Temporal、MinIO、Ingress、TLS、Prometheus/Grafana。

## Phase K4 — Helm chart v0

目标：在 raw manifests 稳定后，把 API、worker、migration Job 抽象成 chart。

建议目录：

```txt
deploy/helm/soniq/
  Chart.yaml
  values.yaml
  values.schema.json
  templates/
    api-deployment.yaml
    api-service.yaml
    api-ingress.yaml
    worker-deployment.yaml
    migrate-job.yaml
    configmap.yaml
    secret.yaml
    serviceaccount.yaml
    _helpers.tpl
```

第一版资源：

- API Deployment。
- API Service。
- optional Ingress。
- Worker Deployment。
- Migration Job。
- ConfigMap。
- Secret references。
- ServiceAccount。
- Pod labels。
- resource requests/limits。
- liveness/readiness probes。

不包含：

- Postgres StatefulSet。
- Temporal chart。
- MinIO chart。
- Grafana/Prometheus chart。

这些作为外部依赖。

验收：

- `helm template` 通过。
- `helm lint` 通过。
- `helm install` 能在 kind/minikube 或真实 dev cluster 部署。
- migration Job 成功后 API ready。
- worker 能连接 Temporal task queue。

## Phase K5 — Kubernetes smoke

目标：验证 K8s 环境下真实路径可用。

当前进展：

- 已新增 manifest 级 smoke，用于在没有集群时验证 raw manifests 的结构和资源契约。
- 尚未实现真正部署到 kind/minikube 或 dev cluster 后的完整 workflow smoke。

Smoke 流程：

```txt
helm install
wait migration Job complete
wait API ready
create user
upload audio
worker completes workflow
read transcript/summary/mind map
soft delete
restore
soft delete
purge
verify object storage deleted
verify purge artifact rows deleted status
```

测试环境：

- kind 或 minikube。
- external dependency 可以先用 docker compose 暴露服务，后续再用 dev namespace 内组件。

验收：

- K8s smoke 通过。
- API pod 重启后 recording 仍可查询。
- Worker pod 重启后仍能继续处理新任务。
- API/worker 不依赖同一个本地 volume。

## Phase K6 — Production hardening

目标：补齐企业部署基础能力。

范围：

- PodDisruptionBudget。
- topology spread constraints。
- HPA。
- NetworkPolicy。
- securityContext：
  - non-root。
  - drop capabilities。
  - readOnlyRootFilesystem，若可行。
- resource limits。
- graceful shutdown 文档和测试。
- separate worker concurrency config。
- chart values schema 完整化。
- backup/restore runbook。

验收：

- rolling update 不造成明显中断。
- API readiness 在依赖不可用时正确下线。
- worker 收到 SIGTERM 后停止接新任务并优雅退出。
- 企业部署文档覆盖备份、恢复、升级、回滚。

## 推荐执行顺序

近期顺序：

1. Observability Phase 1/2。
2. S3-compatible storage provider。
3. Production Docker images。
4. Deployment contract 文档。
5. Helm chart v0。
6. K8s smoke。
7. Production hardening。

不要跳过 1 和 2。

原因：

- 没有 observability，K8s 出问题后很难定位。
- 没有 S3-compatible storage，多 pod 下 upload/worker 文件访问不可靠。

## 与 observability 计划的关系

Kubernetes 依赖 observability Phase 1/2：

- `/readyz` 用于 readiness probe。
- structured logs 用于 pod log aggregation。
- request ID 用于 ingress/API 排查。
- debug script 用于排查后台 cleanup 状态。

后续 Prometheus metrics 会继续增强 K8s 部署：

- `/metrics` 用于 scrape。
- Grafana dashboard 用于 API/worker/purge cleanup 监控。
- alert rules 用于生产告警。

## 与 S3-compatible storage 的关系

Kubernetes 部署应默认使用：

```txt
STORAGE_PROVIDER=s3_compatible
```

`local` filesystem storage provider 已移除；Kubernetes 和本地开发都应使用
S3-compatible object storage。多副本 API、API/worker 分布在不同节点、pod 重启
后保留上传文件，都依赖外部 object storage 边界。

## 风险

- 太早写 Helm 会固化错误的配置和存储边界。
- 内置 Postgres/Temporal/MinIO 会让第一版 chart 复杂度过高。
- Secret 处理不当会导致企业部署不可接受。
- object storage 配置错误会导致 worker 无法读取上传文件或生成 presigned ASR URL。
- readiness 太浅会让 K8s 把不可用 pod 加入流量。
- readiness 太深或太慢会导致误杀和启动慢。

## Definition of Done

Kubernetes foundation 完成后应满足：

- API/worker 镜像可被 Kubernetes 部署。
- Soniq API/worker 是 stateless。
- S3-compatible storage 是生产推荐路径。
- Migration 通过独立 Job 执行。
- Helm chart 不默认管理 Postgres/Temporal/object storage。
- `/readyz` 可作为 readiness probe。
- 生产部署文档明确 ConfigMap、Secret、外部依赖、备份恢复。
- K8s smoke 可验证 upload -> process -> results -> soft delete -> restore -> purge。
