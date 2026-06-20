# 部署契约

本文定义 Soniq 在本地开发环境之外运行时需要遵守的部署契约。它描述 Soniq 自身管理的进程和外部依赖，不规定必须使用某一种 Kubernetes、Helm、Terraform 或云厂商实现。

## 托管进程

Soniq 当前需要部署三类后端进程：

- `soniq-api`：HTTP API server。它提供 `/healthz`、`/readyz`、认证、workspace、recording、upload、retry、details、Trash、restore 和 purge API。
- `soniq-worker`：Temporal worker。它轮询 `TEMPORAL_TASK_QUEUE`，执行音频探测、归一化、转写、总结、思维导图活动，并重试 purge artifact cleanup rows。
- `soniq-migrate`：一次性 migration 命令，用于 Soniq application Postgres 数据库。

API 和 worker 都应该是无状态的。上传音频和生成的音频 artifact 必须放在 S3-compatible object storage 中，不能依赖 pod-local 或 node-local disk。

## 外部依赖

生产部署在启动 API 和 worker pod 之前，必须准备这些依赖：

- Soniq application Postgres。它和 Temporal 自己使用的 Postgres 是分开的。
- API 和 worker 都能访问的 Temporal frontend。
- API、worker，以及任何需要读取 presigned audio URL 的真实外部 ASR provider 都能访问的 S3-compatible object storage。
- 可选的外部 ASR 和 LLM provider。

Soniq migrations 只作用于 Soniq application Postgres 数据库，不能指向 Temporal 的数据库。

## Migration 策略

在 rollout API 和 worker 之前，应该把 `soniq-migrate` 作为独立部署步骤运行。对于同一个 release，同一时间只应该有一个 migration runner。

推荐顺序：

1. 构建并发布 `soniq-migrate`、`soniq-api` 和 `soniq-worker` 镜像。
2. 应用运行时 config 和 secrets。
3. 对 Soniq application Postgres 运行 `soniq-migrate` job。
4. 启动或 rollout `soniq-api`。
5. 启动或 rollout `soniq-worker`。
6. 在把流量发给 API 之前检查 `/readyz`。

不要让每个 API 或 worker pod 在启动时自动运行 migrations。

## 配置

非敏感值应该放在类似 ConfigMap 的配置中：

| Key | Required | Notes |
| --- | --- | --- |
| `APP_ENV` | no | 部署环境使用 `production`。 |
| `APP_PUBLIC_URL` | no | 未来 callback 和链接使用的公开 API/web origin。 |
| `LOG_FORMAT` | yes | 日志采集场景使用 `json`。 |
| `LOG_LEVEL` | yes | `debug`、`info`、`warn` 或 `error`。 |
| `AUTH_SESSION_TTL_HOURS` | yes | opaque session cookie 的有效期。 |
| `AUTH_COOKIE_SECURE` | yes | HTTPS 后面使用 `true`。 |
| `API_ADDRESS` | API required | 容器内通常是 `:8080`。 |
| `TEMPORAL_ADDRESS` | yes | Temporal frontend host 和 port。 |
| `TEMPORAL_NAMESPACE` | yes | API 和 worker 使用的 Temporal namespace。 |
| `TEMPORAL_TASK_QUEUE` | yes | API 和 worker 必须一致。 |
| `WORKER_METRICS_ADDRESS` | worker required | worker Prometheus metrics 监听地址，通常是 `:9091`。只有明确禁用 metrics 时才设为空。 |
| `WORKER_MAX_CONCURRENT_WORKFLOW_TASKS` | worker required | 单个 worker 进程最多同时执行多少 Temporal workflow tasks。必须大于 1。 |
| `WORKER_MAX_CONCURRENT_ACTIVITIES` | worker required | 单个 worker 进程最多同时执行多少 Temporal activities。用于限制每个 worker pod 内的音频处理和外部 model 调用并发。 |
| `WORKER_MAX_CONCURRENT_LOCAL_ACTIVITIES` | worker required | 单个 worker 进程最多同时执行多少 Temporal local activities。 |
| `WORKER_TASK_QUEUE_ACTIVITIES_PER_SECOND` | worker required | 可选的整个 Temporal task queue activity 速率限制。设置为 `0` 表示使用 SDK 默认值，等价于不额外限速。 |
| `PURGE_ARTIFACT_CLEANUP_INTERVAL_SECONDS` | worker required | 后台 purge cleanup 间隔。 |
| `PURGE_ARTIFACT_CLEANUP_BATCH_SIZE` | worker required | cleanup batch size。 |
| `STORAGE_PROVIDER` | yes | 当前只支持 `s3_compatible`。 |
| `S3_ENDPOINT` | yes | S3-compatible endpoint。API 和 worker 必须能访问。 |
| `S3_REGION` | yes | S3-compatible region value。 |
| `S3_BUCKET` | yes | 上传和生成 artifact 使用的 bucket。 |
| `S3_FORCE_PATH_STYLE` | yes | MinIO 使用 `true`，云 object storage 通常使用 `false`。 |
| `TRANSCRIPTION_PROVIDER` | yes | `fake_transcription`、`openai_compatible_asr` 或 `dashscope_asr`。 |
| `TRANSCRIPTION_BASE_URL` | provider-specific | OpenAI-compatible ASR base URL。 |
| `TRANSCRIPTION_MODEL` | provider-specific | ASR model name。 |
| `TRANSCRIPTION_AUTH_HEADER` | provider-specific | `api-key` 或 `bearer`。 |
| `TRANSCRIPTION_LANGUAGE` | no | 语言提示，例如 `auto`、`zh` 或 `en`。 |
| `DASHSCOPE_BASE_URL` | provider-specific | DashScope native ASR endpoint。 |
| `DASHSCOPE_ASR_MODEL` | provider-specific | DashScope ASR model。 |
| `LLM_PROVIDER` | yes | `fake_llm`、`openai_compatible` 或支持的 local provider。 |
| `LLM_BASE_URL` | provider-specific | OpenAI-compatible LLM base URL。 |
| `LLM_MODEL` | provider-specific | LLM model name。 |
| `PRIVACY_ALLOW_EXTERNAL_MODEL_PROVIDERS` | yes | 设置为 `false` 时拒绝外部 model provider。 |
| `PRIVACY_DELETE_ORIGINAL_AUDIO_AFTER_TRANSCRIPTION` | yes | 转写成功后删除原始上传音频。 |

敏感值应该放在类似 Secret 的配置中：

| Key | Required | Notes |
| --- | --- | --- |
| `POSTGRES_DSN` | yes | Soniq application Postgres DSN。 |
| `S3_ACCESS_KEY` | yes | S3-compatible access key。 |
| `S3_SECRET_KEY` | yes | S3-compatible secret key。 |
| `TRANSCRIPTION_API_KEY` | provider-specific | OpenAI-compatible ASR key。 |
| `MIMO_API_KEY` | optional | Xiaomi MiMo ASR 的 alias fallback。 |
| `DASHSCOPE_API_KEY` | provider-specific | DashScope native ASR key。 |
| `LLM_API_KEY` | provider-specific | External LLM key。 |

Helm chart 提供了 `deploy/helm/soniq/values.production.example.yaml`，作为不包含
secret 的生产 values 起点。它需要和预先创建的 Kubernetes Secret 或 external secret
manager 一起使用，用来提供上面这些敏感值。

当前认证使用存储在 Postgres 中的 opaque session token，并使用绑定到 session token 的 CSRF token。目前还没有单独的 session signing secret。

## Storage 要求

Object storage 必须支持 S3-compatible 的 `PutObject`、`GetObject`、`DeleteObject`、`HeadBucket` 和 presigned GET URLs。

真实外部 ASR provider 会收到 presigned normalized-audio URL。在这种运行方式下，`S3_ENDPOINT` 必须生成该 provider 能访问的 URL。只在集群内部可访问的本地 MinIO endpoint 适合 fake-provider 验证，但不适合外部 ASR 调用。

## 备份边界

需要备份这些数据存储：

- Soniq application Postgres，包括 users、workspaces、recordings、transcripts、summaries、mind maps、sessions 和 purge cleanup rows。
- S3-compatible bucket objects，包括保留的 original audio 和 normalized audio artifacts。

Temporal history 和 visibility storage 由 Temporal 部署负责。Temporal 应按照 Temporal 自己的部署策略备份，而不是通过 Soniq application migrations 备份。

具体备份和恢复操作清单见 [backup-restore.zh-CN.md](backup-restore.zh-CN.md)。

## Readiness

`GET /healthz` 表示 API 进程还活着。

`GET /readyz` 表示 API 可以访问必要依赖：

- Soniq application Postgres。
- 当前 Soniq migration version。
- Temporal frontend。
- S3-compatible bucket。

Kubernetes 或任何 load balancer 都应该用 `/readyz` 判断 API 是否 ready，用 `/healthz` 判断 API 是否 live。

## Metrics

API 的 `GET /metrics` 暴露 Prometheus text-format API metrics：

- `soniq_http_requests_total{route,method,status}`
- `soniq_http_request_duration_seconds{route,method}`

worker 会在 `WORKER_METRICS_ADDRESS` 上暴露自己的 `/metrics`。当前 worker metrics 包括：

- `soniq_worker_activities_total{activity,result}`
- `soniq_worker_activity_duration_seconds{activity}`
- `soniq_recording_terminal_status_updates_total{status}`
- `soniq_purge_artifacts_claimed_total`
- `soniq_purge_artifacts_deleted_total`
- `soniq_purge_artifacts_failed_total`
- `soniq_purge_cleanup_run_duration_seconds{result}`
- `temporal_*` Temporal Go SDK metrics，用于观察 worker poll、activity/workflow task
  latency、SDK request latency 和 worker task slots。

API 的 `route` label 使用 router template，而不是真实请求 path。不要把 `user_id`、
`workspace_id`、`recording_id`、artifact ID、object key、email、文件名，或者其他高基数字段放进
Prometheus labels。`soniq_recording_terminal_status_updates_total` 统计的是已经写入
数据库的 `completed` / `failed` 终态更新，不是完整 Temporal workflow outcome counter。
Temporal SDK metrics 由 worker 暴露，只保留 `namespace`、`task_queue`、`workflow_type`、
`activity_type`、`poller_type`、`worker_type` 等低基数字段。

仓库还包含一个可选的本地 observability stack：

- `compose.observability.yml`
- Prometheus：存储和查询 metrics。
- Grafana：展示 dashboard。
- OpenTelemetry Collector：接收 OTLP traces，以及后续 OTLP metrics。
- Jaeger：存储和查看 traces。

启动命令：

```bash
make observability-up
```

Prometheus 会抓取本地 API 的 `host.docker.internal:8080/metrics`，以及本地 worker 的
`host.docker.internal:9091/metrics`。Grafana 地址是
`http://localhost:3001`，本地默认账号密码是 `admin` / `soniq_admin`；Jaeger 地址是
`http://localhost:16686`。这些默认值只用于本地开发，不应作为生产 credentials 或生产
observability 部署方式。

Tracing 默认关闭。需要导出 API、Temporal workflow/activity 和 purge cleanup traces 时，
设置 `OTEL_TRACES_ENABLED=true`，并把 `OTEL_EXPORTER_OTLP_ENDPOINT` 指向 collector
的 OTLP HTTP endpoint。本地通常是 `http://localhost:4318`，Kubernetes 集群内通常是
`http://otel-collector:4318`。

在 Kubernetes 里，worker 容器会暴露 TCP 9091 metrics 端口，但 baseline NetworkPolicy
默认不允许任何入站访问 worker pod。生产环境接 Prometheus 时，应只允许 Prometheus 或监控
namespace 访问这个端口，而不是把 9091 对整个集群开放。
