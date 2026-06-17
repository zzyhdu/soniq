# 企业级可观测性基础设施计划

## 背景

Soniq 的目标是企业级、自托管的 audio intelligence 平台。随着系统从本地
pipeline 进入账号、workspace、Temporal workflow、对象存储、Trash purge
等真实产品路径，仅靠临时 `log.Printf`、单个 `/healthz` 和手动查库已经不够。

我们需要建立一套可逐步演进的可观测性系统，让开发者和未来的管理员能够回答：

- 系统现在是否可用？
- 哪些请求变慢或失败？
- 某个 recording 从 API 到 Temporal worker 发生了什么？
- 后台任务是否堆积？
- purge artifact cleanup 是否失败、为什么失败、什么时候重试？
- 线上错误属于哪个 release、哪个环境、哪个用户动作触发？

## 行业依据

本计划基于当前主流实践：

- OpenTelemetry 将可观测性信号抽象为 logs、metrics、traces。
- Google SRE 的 four golden signals 用 latency、traffic、errors、
  saturation 作为服务健康监控核心。
- Prometheus 推荐清晰命名、合理 label，并避免高基数 label。
- OWASP 强调认证、权限、安全相关事件需要可审计，同时必须避免记录敏感数据。
- Go 标准库 `log/slog` 是当前 Go 原生结构化日志基础。
- Temporal Go SDK 自身支持 metrics、logging、tracing、visibility，应纳入
  Soniq 的整体可观测性，而不是只观察 HTTP API。
- Sentry 适合 error tracking 和 release 回归定位，但不能替代 metrics、
  logs、traces 和业务状态表。

参考：

- https://opentelemetry.io/docs/concepts/signals/
- https://sre.google/workbook/monitoring/
- https://prometheus.io/docs/practices/naming/
- https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html
- https://pkg.go.dev/log/slog
- https://docs.temporal.io/develop/go/platform/observability
- https://opentelemetry.io/docs/collector/
- https://docs.sentry.io/product/

## 总体目标

建立一套企业级、可自托管、可渐进接入外部平台的 observability foundation：

- 默认本地开发可用，不依赖云服务。
- 生产部署可接 Prometheus、Grafana、OpenTelemetry Collector、Sentry 或同类后端。
- 关键路径有结构化日志、request ID、错误上下文和基础 metrics。
- 后台任务状态可被查询、可报警、可排查。
- 不记录敏感信息，不把 transcript、summary、audio 内容或 token 打进日志。
- 不一次性引入过重平台，按最小可验证阶段交付。

## 非目标

第一阶段不做：

- 完整 admin dashboard。
- 多租户运维后台权限体系。
- 全量 OpenTelemetry tracing。
- 强依赖 Sentry SaaS。
- 复杂告警编排和 on-call 流程。
- 长期日志存储平台选型。

这些会在后续阶段逐步接入。

## 设计原则

### 1. 三类信号分工明确

- Logs：解释具体发生了什么，适合排查单个请求、单个 workflow、单个后台任务。
- Metrics：表达趋势和报警条件，适合看失败率、延迟、队列堆积。
- Traces：串联一次操作跨 API、DB、Temporal worker、object storage 的路径。

不要用一种信号替代全部。

### 2. 低基数 metrics

Prometheus label 不能包含：

- `user_id`
- `workspace_id`
- `recording_id`
- `artifact_id`
- `object_key`
- email
- 文件名

这些应放在 logs、traces 或 DB debug 查询里。

### 3. 默认保护隐私

日志和错误上报不得包含：

- password
- session token
- CSRF token
- API key
- transcript 原文
- summary 原文
- mind map 内容
- audio 内容
- 原始 Authorization/Cookie header

`object_key` 可能包含用户文件名，默认只在 debug 级别或显式业务排查日志中出现。

### 4. 先内部可用，再外部集成

第一步先让本地开发和 self-host 部署能查、能看、能定位。
之后再接 Prometheus、OpenTelemetry Collector、Sentry。

### 5. 可观测性本身也要测试

新增 middleware、metrics、debug scripts 和 readiness checks 都要有测试或 smoke 覆盖，
不能只靠手动观察。

## 目标架构

长期形态：

```txt
Soniq API
  |-- structured JSON logs
  |-- /healthz
  |-- /readyz
  |-- /metrics
  |-- OpenTelemetry traces
  |-- optional Sentry error capture

Soniq Worker
  |-- structured JSON logs
  |-- worker metrics endpoint
  |-- Temporal SDK metrics
  |-- OpenTelemetry activity/workflow traces
  |-- optional Sentry error capture

Soniq Postgres
  |-- business state tables
  |-- purge artifact status
  |-- audit/event tables in future

Local/self-host observability stack
  |-- Prometheus for metrics
  |-- Grafana for dashboards
  |-- Loki or equivalent for logs, optional
  |-- Tempo/Jaeger for traces, optional
  |-- Sentry or equivalent for error tracking, optional
```

## 分阶段实现

## Phase 1 — 结构化日志和 request ID

目标：让 API 和 worker 的日志可搜索、可过滤、可关联。

### 后端 logger 基础

新增内部 logger 包，例如：

- `backend/internal/observability/logger.go`
- `backend/internal/observability/request_id.go`

使用 Go 标准库 `log/slog`。

配置项：

- `LOG_FORMAT=json|text`，默认 local 为 `text` 或 `json` 需要再评估。
- `LOG_LEVEL=debug|info|warn|error`，默认 `info`。

建议生产默认：

```txt
LOG_FORMAT=json
LOG_LEVEL=info
```

### API request logging middleware

新增 middleware：

- 生成或透传 `X-Request-ID`。
- 将 `request_id` 写入 request context。
- 响应 header 返回 `X-Request-ID`。
- 每个请求完成后打一条结构化 access log。

字段：

```txt
service=soniq-api
event=http_request_completed
request_id
method
route
status
duration_ms
remote_addr
user_agent
user_id, if authenticated
workspace_id, if available
```

注意：

- route 必须是模板路径，例如 `/workspaces/{workspace_id}/recordings/{recording_id}`。
- 不记录完整 URL query，避免泄露敏感参数。
- 不记录 Cookie、Authorization、CSRF token。

### API error logging

对 5xx 错误记录 error log。

字段：

```txt
event=api_error
request_id
route
status
error_code
error
```

4xx 默认不作为 error 级别，认证失败、CSRF 失败等安全事件可单独作为 security event。

### Worker structured logging

将 worker 主流程、provider 初始化、cleanup runner 日志从 `log.Printf` 迁移到 `slog`。

字段：

```txt
service=soniq-worker
event
temporal_task_queue
workflow_id
recording_id
activity
artifact_id
attempt_count
```

### Purge cleanup 日志

关键事件：

- `purge_artifact_cleanup_claimed`
- `purge_artifact_cleanup_deleted`
- `purge_artifact_cleanup_failed`
- `purge_artifact_cleanup_run_failed`

失败日志包含：

```txt
artifact_id
recording_id
workspace_id
artifact_kind
attempt_count
next_attempt_at
error
```

`object_key` 默认不打；如确实需要，可以 debug 级别打脱敏/截断后的 key。

### Phase 1 验收标准

- API 每个请求都有 request ID。
- API 响应包含 `X-Request-ID`。
- 5xx 错误日志可以通过 request ID 找到。
- worker cleanup 失败日志是结构化字段，不再只是字符串拼接。
- 日志不包含 password、session token、CSRF token。

### Phase 1 测试

- request ID middleware unit test。
- access log middleware 测试 route/status/duration/request_id 字段。
- error response 测试不泄露敏感 header。
- cleanup runner 失败日志可用测试，至少验证 logger 被调用或事件字段可构造。

## Phase 2 — readiness 和内部 debug 工具

目标：快速判断系统是否真正可服务，并提供业务状态排查入口。

### `/healthz` 保持轻量

当前 `/healthz` 继续表示进程存活，不做外部依赖检查。

### 新增 `/readyz`

API readiness 检查：

- Postgres ping。
- migration version 是否达到当前应用要求。
- Temporal client 可连接，或至少可 dial。
- S3-compatible object storage bucket 可访问。

返回：

```json
{
  "status": "ready",
  "checks": {
    "postgres": "ok",
    "migrations": "ok",
    "temporal": "ok",
    "object_storage": "ok"
  }
}
```

失败时：

- HTTP `503`
- 返回每个 check 的状态和简短错误。
- 不返回 DSN、secret、绝对敏感路径。

### Worker readiness

worker 不一定需要 HTTP server。第一步可以通过 startup log 和 metrics 观察。
后续 Phase 3 可为 worker 增加独立 metrics/readiness 端口。

### Debug scripts

新增脚本：

- `scripts/debug-purge-artifacts.sh`

功能：

- 显示 purge artifact 状态统计。
- 列出 failed rows。
- 列出长期卡在 deleting 的 rows。
- 支持 `LIMIT`。
- 默认读取 Makefile/.env 的 `POSTGRES_DSN`。

输出示例：

```txt
Purge artifact status
deleted   18
failed    1
pending   0
deleting  0

Failed artifacts
rpa_xxx rec_xxx normalized_audio attempt=3 next=2026-06-15T10:00:00Z error="delete object: permission denied"
```

### Phase 2 验收标准

- `/healthz` 和 `/readyz` 语义区分清楚。
- Postgres down 时 `/readyz` 返回 `503`。
- migration 未达标时 `/readyz` 返回 `503`。
- debug 脚本无需手写 SQL 即可查看 purge cleanup 状态。

### Phase 2 测试

- readiness handler 使用 fake checks 覆盖 ready/unready。
- debug script 至少有 shellcheck 或基础 smoke，若 shellcheck 未引入则用 bash dry-run 风格测试。
- docs 更新 `docs/development.md`。

## Phase 3 — Prometheus metrics

目标：建立趋势监控和报警基础。

### API metrics

新增 `/metrics`。

初始 metrics：

```txt
soniq_http_requests_total{route,method,status}
soniq_http_request_duration_seconds{route,method}
soniq_recording_uploads_total{result}
soniq_recording_deletes_total{mode,result}
soniq_auth_attempts_total{operation,result}
```

其中：

- `mode=soft|purge`
- `result=success|error`
- `route` 是模板 route，不是真实 path。

### Worker metrics

可以独立开端口，例如：

```txt
WORKER_METRICS_ADDRESS=:9091
```

初始 metrics：

```txt
soniq_purge_artifacts_claimed_total
soniq_purge_artifacts_deleted_total
soniq_purge_artifacts_failed_total
soniq_purge_artifacts_pending
soniq_purge_cleanup_run_duration_seconds
soniq_recording_processing_completed_total
soniq_recording_processing_failed_total
```

### Temporal SDK metrics

接入 Temporal Go SDK metrics。

要关注：

- worker poll 成功/失败。
- activity execution latency。
- workflow task latency。
- worker task queue backlog 相关指标。

具体接入方式按 Temporal Go SDK 官方 observability 文档实现。

### 本地 Compose

新增可选 observability compose：

- `compose.observability.yml`
- Prometheus
- Grafana

默认不随 `make temporal-up` 启动，避免本地开发过重。

Makefile：

```txt
make observability-up
make observability-down
```

### Phase 3 验收标准

- API `/metrics` 可被 Prometheus scrape。
- metrics 没有高基数 label。
- Grafana 至少有基础 dashboard：
  - API request rate/error/latency
  - purge artifact status
  - worker cleanup failures

### Phase 3 测试

- metrics endpoint test。
- route label 使用模板路径的测试。
- 不出现 recording/user/workspace id label 的测试。

## Phase 4 — OpenTelemetry tracing

目标：串起一次操作跨组件的路径。

### API tracing

关键 span：

- HTTP request。
- DB query/transaction，高层语义即可，不需要每条 SQL 都自动埋点。
- object storage put/delete。
- Temporal workflow start。
- purge transaction。

字段：

```txt
service.name=soniq-api
deployment.environment
request_id
recording_id, only span attribute, not metrics label
workflow_id
```

### Worker tracing

关键 span：

- activity execution。
- provider call。
- ffprobe。
- ffmpeg normalization。
- object storage delete。
- purge artifact cleanup run。

Temporal Go SDK tracing 接入应优先使用官方推荐方式。

### Collector

本地可选：

- OpenTelemetry Collector。
- Tempo 或 Jaeger。

配置：

```txt
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
OTEL_SERVICE_NAME=soniq-api / soniq-worker
```

### Phase 4 验收标准

- 一次 upload 请求能关联到 Temporal workflow start。
- 一个 worker activity failure 能在 trace 中看到上下文。
- purge cleanup failure 能通过 trace 找到 object delete 失败点。

## Phase 5 — Sentry / error tracking

目标：捕获异常、panic、前端错误和 release 回归。

### 后端 Sentry

配置项：

```txt
SENTRY_DSN=
SENTRY_ENVIRONMENT=development
SENTRY_RELEASE=
```

默认空 DSN，表示关闭。

捕获：

- panic。
- 5xx handler errors。
- worker cleanup unexpected errors。
- activity/provider unexpected errors。

不捕获：

- 预期 4xx。
- 密码错误。
- 未认证错误。

### 前端 Sentry

配置项：

```txt
VITE_SENTRY_DSN=
VITE_SENTRY_ENVIRONMENT=
VITE_SENTRY_RELEASE=
```

要求：

- source maps 只在配置 DSN/release 时上传。
- 默认关闭。
- scrub PII。

### Phase 5 验收标准

- 后端 panic 能被捕获并带 release/environment。
- 前端 runtime error 能被捕获并映射到源码。
- Sentry event 不包含 token、password、transcript 内容。

## Phase 6 — Audit log 和 admin/debug UI

目标：企业环境下支持审计和管理员排查。

这阶段应在 auth/RBAC 更成熟后执行。

### Audit log

记录高价值业务事件：

- sign in success/failure。
- sign out。
- recording upload。
- recording soft delete。
- recording restore。
- recording purge。
- workspace membership change。
- provider config change。

字段：

```txt
id
workspace_id
actor_user_id
event_type
target_type
target_id
request_id
ip_hash or remote_addr policy
user_agent
created_at
metadata_json
```

### Admin/debug UI

只给 owner/admin：

- purge artifact failed/pending/deleting 状态。
- workflow failures。
- recent audit events。
- readiness status。
- provider config health。

必须有权限控制和安全审查，不放在第一阶段。

## 与 Kubernetes 部署计划的关系

Kubernetes 部署计划见
`docs/plans/2026-06-15-kubernetes-deployment-foundation.zh-CN.md`。

Observability Phase 1/2 是 Kubernetes 前置条件之一：

- `/readyz` 会作为 Kubernetes readiness probe。
- structured logs 会进入 pod log aggregation。
- request ID 用于 ingress、API、worker 之间的问题关联。
- cleanup/debug scripts 用于排查集群内后台任务状态。

后续阶段继续服务 Kubernetes 生产部署：

- Phase 3 的 `/metrics` 用于 Prometheus scrape。
- API/worker metrics 用于 Grafana dashboard 和 alert rules。
- Phase 4 tracing 需要带上 Kubernetes deployment metadata，例如 namespace、
  pod name、service name、release。
- Phase 5 Sentry 需要带上 release、environment，并继续遵守 PII scrub 规则。

因此 Kubernetes chart 不应早于 Observability Phase 1/2 落地；否则 pod 可以运行，
但出现 readiness、worker、purge cleanup 或 provider 问题时缺少定位手段。

## 实施顺序建议

优先执行：

1. Phase 1：`slog` + request ID + API/worker structured logs。
2. Phase 2：`/readyz` + `scripts/debug-purge-artifacts.sh`。
3. Phase 3：Prometheus `/metrics` + 最小 Grafana dashboard。
4. Phase 4：OpenTelemetry tracing。
5. Phase 5：Sentry optional integration。
6. Phase 6：Audit log + admin/debug UI。

下一次具体开发建议先做 Phase 1 + Phase 2，形成第一个可交付切片。

## Phase 1 + Phase 2 具体文件计划

后端：

- `backend/internal/observability/logger.go`
- `backend/internal/observability/request_id.go`
- `backend/internal/observability/readiness.go`
- `backend/internal/api/router.go`
- `backend/internal/api/middleware.go` 或现有 middleware 文件
- `backend/internal/api/health_handlers.go`
- `backend/cmd/api/main.go`
- `backend/cmd/worker/main.go`
- `backend/internal/cleanup/recording_purge_artifacts.go`
- `backend/internal/config/config.go`
- `backend/internal/config/config_test.go`

脚本：

- `scripts/debug-purge-artifacts.sh`

文档：

- `docs/development.md`
- `docs/architecture.md`，如涉及 observability 架构边界。

测试：

- `backend/internal/observability/*_test.go`
- `backend/internal/api/*_test.go`
- `backend/internal/cleanup/*_test.go`
- `backend/cmd/api/main_test.go`
- `backend/cmd/worker/main_test.go`

## 风险和取舍

- 过早引入完整 LGTM/OTel/Sentry 会增加本地开发复杂度，所以先做可插拔基础。
- 日志字段一旦散乱，后期很难统一，所以 Phase 1 要先定义字段规范。
- metrics label 如果误加高基数字段，Prometheus 成本会失控，所以需要测试约束。
- readiness 不能暴露 secrets，也不能做太慢的深度检查。
- Sentry 不能成为唯一错误来源；业务状态仍然要靠 DB 状态表、logs 和 metrics。

## Definition of Done

整体计划完成后，应满足：

- 本地开发可以通过 logs + debug script 定位 purge cleanup 和 workflow 问题。
- 部署环境可以通过 `/readyz` 判断服务是否可接流量。
- Prometheus 可以 scrape API/worker metrics。
- Grafana 可以看到 API、worker、purge cleanup 的基础健康状态。
- Trace 可以串起 API 请求和 worker activity。
- Sentry 可选接入，不配置时系统正常运行。
- 文档说明如何启用、如何查询、如何排查常见问题。
