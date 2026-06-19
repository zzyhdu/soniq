# Kubernetes 部署

本文描述 Soniq 第一版 Kubernetes deployment foundation。它包含
`deploy/kubernetes/base/` 下便于直接检查的 raw manifests，也包含
`deploy/helm/soniq/` 下用于参数化部署的第一版 Helm chart。

## 范围

raw manifests 和 Helm chart 只部署 Soniq 自己拥有的资源：

- `soniq-migrate` Job。
- `soniq-api` Deployment 和 Service。
- `soniq-worker` Deployment。
- `soniq-api` 和 `soniq-worker` PodDisruptionBudgets。
- raw manifests 里的 `soniq-api` HorizontalPodAutoscaler。
- `soniq-api`、`soniq-worker` 和 `soniq-migrate` NetworkPolicies。
- `soniq-config` ConfigMap。
- `soniq-secret` 引用；raw manifests 里有 example Secret，Helm 里可以选择性渲染 Secret。
- raw manifests 里有 `soniq` Namespace，Helm 里可以选择性渲染 Namespace。

它们不部署 Postgres、Temporal、MinIO、Ingress、TLS certificates、Prometheus、Grafana 或 external secret controllers。

## 前置条件

在把 manifests 应用到真实集群之前，需要先准备：

- 一个 Soniq application Postgres 数据库。
- 一个 Temporal frontend address 和 namespace。
- 一个 S3-compatible bucket 和 credentials。
- 已构建并推送的镜像：
  - `soniq-api`
  - `soniq-worker`
  - `soniq-migrate`

在仓库根目录构建本地镜像 tag：

```bash
make docker-build
```

如果是远程集群，需要把镜像推送到 registry，并更新这些文件里的 `image` 字段：

- `deploy/kubernetes/base/api-deployment.yaml`
- `deploy/kubernetes/base/worker-deployment.yaml`
- `deploy/kubernetes/base/migrate-job.yaml`

## 配置

编辑 `deploy/kubernetes/base/configmap.yaml`，设置非敏感配置：

- `APP_PUBLIC_URL`
- `TEMPORAL_ADDRESS`
- `TEMPORAL_NAMESPACE`
- `TEMPORAL_TASK_QUEUE`
- `WORKER_METRICS_ADDRESS`
- worker concurrency settings：
  - `WORKER_MAX_CONCURRENT_WORKFLOW_TASKS`
  - `WORKER_MAX_CONCURRENT_ACTIVITIES`
  - `WORKER_MAX_CONCURRENT_LOCAL_ACTIVITIES`
  - `WORKER_TASK_QUEUE_ACTIVITIES_PER_SECOND`
- `S3_ENDPOINT`
- `S3_REGION`
- `S3_BUCKET`
- `S3_FORCE_PATH_STYLE`
- provider selection 和 model names

基于 `deploy/kubernetes/base/secret.example.yaml` 创建真实 Secret，并在真实部署前替换所有 `change-me` 值。这个 example 文件可以安全提交，因为它只包含占位值。

必需的 Secret keys：

- `POSTGRES_DSN`
- `S3_ACCESS_KEY`
- `S3_SECRET_KEY`

Provider-specific Secret keys：

- `TRANSCRIPTION_API_KEY`
- `MIMO_API_KEY`
- `DASHSCOPE_API_KEY`
- `LLM_API_KEY`

## 校验 Manifests

本地渲染 base manifests：

```bash
make k8s-render
```

这个 target 会执行 `kubectl kustomize deploy/kubernetes/base`。如果本机没有
安装 `kubectl`，Makefile 会 fallback 到 Docker 里的 kubectl 镜像。

Helm chart 相关工作也使用同样的“本机工具优先，否则 Docker fallback”模式。可以用下面命令检查 Helm CLI：

```bash
make helm-version
```

如果本机没有安装 `helm`，Makefile 会 fallback 到 Docker 里的 Helm 镜像。

校验 Helm chart：

```bash
make helm-lint
make helm-template
```

默认 chart 会渲染 ServiceAccount、ConfigMap、API Service、API Deployment、
worker Deployment、API/worker PodDisruptionBudgets、API/worker/migration
NetworkPolicies，以及作为 pre-install/pre-upgrade hook 的 migration Job。它会引用
`soniq-secret`，但默认不会渲染 Secret，所以真实 secret material 不会进入已提交的
values。它也默认跟随 Helm release namespace，而不是创建 Namespace resource。

chart 里也包含一个生产覆盖示例：

```bash
deploy/helm/soniq/values.production.example.yaml
```

可以把它作为私有 `values.production.yaml` 的起点。真实部署前需要替换 image
repository、public URL、Temporal、object storage、resources、autoscaling 和
provider 配置。示例里刻意保持 `secret.create=false`；真实 credentials 应该放在
预先创建的 Kubernetes Secret 或 external secret manager 中。

校验这个生产 values 示例：

```bash
HELM_LINT_ARGS='-f deploy/helm/soniq/values.production.example.yaml' make helm-lint
HELM_TEMPLATE_ARGS='-f deploy/helm/soniq/values.production.example.yaml' make helm-template
```

如果只是想在本地渲染可选的 Namespace 和占位 Secret 路径：

```bash
HELM_TEMPLATE_ARGS='--set namespace.create=true --set secret.create=true --set migrate.hook.enabled=false --set-string secret.stringData.POSTGRES_DSN=postgres://soniq_user:change-me@postgres.example.com:5432/soniq?sslmode=require --set-string secret.stringData.S3_ACCESS_KEY=change-me --set-string secret.stringData.S3_SECRET_KEY=change-me' \
  make helm-template
```

真实 Helm install 时，先创建 namespace 和 Secret，然后安装到这个 namespace：

```bash
kubectl create namespace soniq --dry-run=client -o yaml | kubectl apply -f -
kubectl -n soniq create secret generic soniq-secret \
  --from-literal=POSTGRES_DSN='postgres://soniq_user:change-me@postgres.example.com:5432/soniq?sslmode=require' \
  --from-literal=S3_ACCESS_KEY='change-me' \
  --from-literal=S3_SECRET_KEY='change-me' \
  --dry-run=client -o yaml | kubectl apply -f -
```

如果生产 values 开启了外部 ASR 或 LLM provider，需要把对应 provider key 也放进同一个
Secret，例如 `DASHSCOPE_API_KEY`、`TRANSCRIPTION_API_KEY` 或 `LLM_API_KEY`。

然后带上私有配置覆盖来应用 chart：

```bash
helm upgrade --install soniq deploy/helm/soniq \
  --namespace soniq \
  --wait \
  --timeout 5m \
  -f values.production.yaml
```

默认情况下，migration Job 会作为 Helm `pre-install,pre-upgrade` hook 执行，所以
migrations 成功完成后才会继续应用 API/worker 等应用资源。因为 pre-install hook
会在普通 chart resources 创建前执行，所以 `soniq-secret` 引用的 Secret 必须在
install 前已经存在。hook 会直接从 chart values 接收非敏感 config，从预先存在的
Secret 接收敏感值。只有在明确想单独管理 migration Job 时，才设置
`migrate.hook.enabled=false`。chart 会拒绝 `secret.create=true` 和默认 migration
hook 同时开启，因为这个组合在首次 install 时无法工作。

运行本地 manifest smoke：

```bash
make k8s-smoke
```

这个脚本会渲染 base manifests，并检查预期的 Kubernetes resource contract。
这个 smoke 需要 `python3` 和 PyYAML，不需要 Kubernetes API server。

当有可用集群时，运行 server-side dry-run：

```bash
make k8s-dry-run
```

Server-side dry-run 会请求 Kubernetes API server 校验 resource kinds 和 schemas，
但不会真的持久化这些应用对象。如果 `soniq` namespace 不存在，带
`K8S_SMOKE_SERVER_DRY_RUN=1` 的 `make k8s-smoke` 会为了校验临时创建它，并在结束后删除。

运行本地 kind 部署 smoke：

```bash
make k8s-smoke-kind
```

这个脚本会启动 Compose 管理的 Postgres、Temporal 和 MinIO，创建或复用
`soniq` kind 集群，把 kind control-plane 容器连接到 Compose Docker network，
然后应用一份临时 smoke manifest。Soniq API、worker 和 migrate 仍然运行在
Kubernetes 里，但 Compose 依赖会通过 Kubernetes `Service` 和 `EndpointSlice` 暴露给 Pod：

- `soniq-postgresql:5432`
- `temporal:7233`
- `minio:9000`

这样 Pod 不需要使用 `localhost`。在 Pod 里，`localhost` 指的是 Pod 自己，
不是开发机，也不是 Compose 容器。

默认情况下，kind smoke 还会注册或登录一个 smoke 用户，通过 port-forward 后的
API 上传一个生成的 WAV 文件，等待 Temporal workflow 完成，校验 transcript、
summary 和 mind map 已经持久化，然后继续验证 recording 生命周期：soft delete、
Trash restore、再次 soft delete、永久 purge、purge artifact cleanup rows，以及
S3 object 删除。如果只想跑部署 readiness 部分，可以设置 `KIND_SMOKE_WORKFLOW=0`。

通过 Helm chart 运行同一条 kind smoke：

```bash
make k8s-smoke-kind-helm
```

这条路径仍然使用 Compose 管理的 Postgres、Temporal 和 MinIO，但会用
`helm upgrade --install` 部署 Soniq API、worker 和 migration hook。因为 migration
hook 需要在普通 chart resources 应用前读取 Secret，smoke 脚本会先创建外部
`soniq-secret`，再调用 Helm。如果本机没有安装 `helm`，这个 target 会通过
`scripts/helm-cluster.sh` 使用 Dockerized Helm，并通过 host network 和挂载的
`~/.kube` 访问本地 kind API server。

## Apply

先应用 config 和 secret：

```bash
kubectl apply -f deploy/kubernetes/base/namespace.yaml
kubectl apply -f deploy/kubernetes/base/configmap.yaml
kubectl apply -f deploy/kubernetes/base/secret.example.yaml
```

运行 migrations：

```bash
kubectl apply -f deploy/kubernetes/base/migrate-job.yaml
kubectl -n soniq wait --for=condition=complete job/soniq-migrate --timeout=120s
kubectl -n soniq logs job/soniq-migrate
```

然后启动 API 和 worker：

```bash
kubectl apply -f deploy/kubernetes/base/api-deployment.yaml
kubectl apply -f deploy/kubernetes/base/api-service.yaml
kubectl apply -f deploy/kubernetes/base/worker-deployment.yaml
kubectl apply -f deploy/kubernetes/base/api-pdb.yaml
kubectl apply -f deploy/kubernetes/base/worker-pdb.yaml
kubectl apply -f deploy/kubernetes/base/api-hpa.yaml
kubectl apply -f deploy/kubernetes/base/api-networkpolicy.yaml
kubectl apply -f deploy/kubernetes/base/worker-networkpolicy.yaml
kubectl apply -f deploy/kubernetes/base/migrate-networkpolicy.yaml
```

等待 API ready：

```bash
kubectl -n soniq rollout status deployment/soniq-api
kubectl -n soniq get pods
```

如果没有 Ingress，只是本地检查：

```bash
kubectl -n soniq port-forward service/soniq-api 8080:80
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```

如果 `AUTH_COOKIE_SECURE=true`，浏览器登录需要 HTTPS。仅通过本地 port-forward 使用 HTTP 测试时，可以临时在 `soniq-config` 中设置 `AUTH_COOKIE_SECURE=false`。

## Shutdown

API 和 worker Deployment 都设置了 `terminationGracePeriodSeconds: 30`。
`soniq-api` 收到停止信号后会调用 `http.Server.Shutdown`，最多等待 25 秒，让
Kubernetes 有时间把 Pod 从 Service endpoints 移除，并让正在处理的 HTTP 请求结束。
`soniq-worker` 会把同一个停止信号传给 Temporal worker 和 purge artifact cleanup
loop；Temporal worker 使用 25 秒 `WorkerStopTimeout`，在 Pod 级别的 30 秒宽限期
结束前尽量完成 drain。

## Disruption Budgets

raw manifests 和 Helm chart 会为 API 和 worker 创建 PodDisruptionBudget。
两者默认都是 `minAvailable: 1`。在默认 2 个 API replicas 和 2 个 worker replicas
的情况下，Kubernetes 做节点维护或集群升级时可以自愿驱逐其中一个 Pod，同时保留
至少一个 API Pod 和一个 worker Pod 继续运行。

## Pod Placement

raw manifests 和 Helm chart 会给 API 和 worker pods 加 topology spread
constraints。默认按 `kubernetes.io/hostname` 分散副本，`maxSkew: 1`，并使用
`whenUnsatisfiable: ScheduleAnyway`。

`ScheduleAnyway` 的意思是 Kubernetes 会尽量把 Pod 分散到不同节点，但如果集群只有
一个可用节点，也仍然允许调度。生产 values 可以按需要把它改成 `DoNotSchedule`，
这样会更严格地要求分散，但在节点不足时 Pod 可能无法调度。

## Security Hardening

API、worker 和 migration containers 都以 non-root 用户运行，drop 所有 Linux
capabilities，禁用 privilege escalation，并设置 `readOnlyRootFilesystem: true`。

manifests 会给每个 container 挂一个 `emptyDir` 到 `/tmp`。这样根文件系统保持只读，
但应用仍然有一个明确、受限的运行时可写目录，用于 multipart buffering、worker
音频临时文件，以及 ffmpeg/ffprobe 的临时文件。

## Autoscaling

raw manifests 会创建 API HorizontalPodAutoscaler。它指向 `soniq-api`
Deployment，保持 2 到 6 个 API replicas，并以平均 CPU 使用率 70% 作为扩容目标。

Helm chart 暴露了 `api.autoscaling` 和 `worker.autoscaling`，但默认都关闭。
只有在集群已经提供 resource metrics，比如安装了 metrics-server 时，才建议在生产
values 里开启。某个组件开启 autoscaling 后，Helm 不再渲染这个 Deployment 的固定
`replicas` 字段，而是让 HorizontalPodAutoscaler 接管副本数量。

worker 的 CPU autoscaling 只是基础选项。后续更适合用 Temporal task queue backlog
或自定义 worker metrics 来做 worker 扩缩容决策。

## Worker Concurrency

worker replicas 和单个 worker 内部并发需要一起调。基础配置是：

- `WORKER_MAX_CONCURRENT_WORKFLOW_TASKS=20`
- `WORKER_MAX_CONCURRENT_ACTIVITIES=4`
- `WORKER_MAX_CONCURRENT_LOCAL_ACTIVITIES=4`
- `WORKER_TASK_QUEUE_ACTIVITIES_PER_SECOND=0`

其中 activity limit 是主要保护项，用来限制每个 worker pod 内同时进行的 ffmpeg
工作和外部 ASR/LLM provider 调用。task queue activity rate 后续可以用于生产环境
需要所有 worker 共享一个整体速率上限的场景。

## Network Policies

raw manifests 和 Helm chart 会为 API、worker、migration pods 创建 NetworkPolicy。
这些策略只有在 Kubernetes 集群的 CNI plugin 支持并执行 NetworkPolicy 时才会真正生效。

基础策略会允许 API 的 TCP 8080 入站，拒绝 worker 和 migration 入站流量，并允许 DNS 以及
TCP 80、443、9000、5432、7233 出站。这些端口覆盖默认 HTTP/HTTPS、
MinIO/S3-compatible、Postgres 和 Temporal 路径。worker 容器仍然暴露 metrics 端口，但生产
部署应该通过 Helm `networkPolicy.worker.metricsIngress.from` 显式配置 Prometheus 或监控
namespace 才能访问 TCP 9091。如果生产依赖使用不同端口，或者需要按目标地址做更严格
allowlist，需要在安装前覆盖 Helm `networkPolicy` values。

## 更新 Migrations

Kubernetes Jobs 在 pod template 变化后不会原地更新。raw-manifest 阶段如果要重新跑 migration，需要删除并重新创建 Job：

```bash
kubectl -n soniq delete job soniq-migrate
kubectl apply -f deploy/kubernetes/base/migrate-job.yaml
```

Helm chart 已经通过 pre-install/pre-upgrade hooks 处理 migration 重跑；当前 raw-manifest 路径保持行为显式。

## 运维检查

API readiness：

```bash
kubectl -n soniq exec deploy/soniq-api -- /soniq-api --version
kubectl -n soniq logs deploy/soniq-api
```

API metrics：

```bash
kubectl -n soniq port-forward service/soniq-api 8080:80
curl -i http://localhost:8080/metrics
```

Worker metrics：

```bash
kubectl -n soniq port-forward deploy/soniq-worker 9091:9091
curl -i http://localhost:9091/metrics
```

Worker logs：

```bash
kubectl -n soniq logs deploy/soniq-worker
```

Migration logs：

```bash
kubectl -n soniq logs job/soniq-migrate
```

备份和恢复操作见 [backup-restore.zh-CN.md](backup-restore.zh-CN.md)。

## 当前限制

- 还没有 Ingress 或 TLS manifest。
- worker autoscaling 开启时仍是基于 CPU；还没有实现基于 backlog 的 worker 扩缩容。
- 还没有远程集群 release smoke；当前 cluster smoke 覆盖的是本地 kind 下的
  raw manifests 和 Helm 两条路径。
- Worker 还没有暴露 HTTP health endpoint，所以 worker health 当前主要依赖进程状态和日志。
