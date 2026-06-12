# Recording 可找回与失败可恢复基础计划

**目标：** 让用户上传后的 recording 能通过稳定 URL 找回；处理失败时能看到原因；失败后可以从 Web UI/API 重新入队处理。

## 范围

- 新增 recording 失败/完成元数据：
  - `failure_reason`
  - `failed_at`
  - `completed_at`
- Workflow 失败时把错误原因写回 recording。
- 新增 retry API：
  - `POST /workspaces/{workspace_id}/recordings/{recording_id}/retry`
  - 仅允许 `failed` recording retry。
- Web UI 支持可刷新/可收藏的详情 hash route：
  - `#/workspaces/{workspace_id}/recordings/{recording_id}`
- Web UI 在失败状态显示失败原因，并提供 retry 操作。

## 非范围

- 不做完整 `workflow_runs` 表。
- 不做 provider 级重试策略配置。
- 不做跨 workspace 分享。
- 不做生产登录/RBAC 扩展。

## 数据库

新增迁移 `0002_add_recording_failure_metadata`：

```sql
ALTER TABLE recordings
  ADD COLUMN failure_reason TEXT NOT NULL DEFAULT '',
  ADD COLUMN completed_at TIMESTAMPTZ,
  ADD COLUMN failed_at TIMESTAMPTZ;
```

本地 `make migrate` 在 baseline `version=1` 后继续应用 `version=2`。

## API

Recording 响应增加：

```json
{
  "failure_reason": "transcribe recording audio: ...",
  "failed_at": "...",
  "completed_at": "..."
}
```

Retry 响应：

```json
{
  "recording": {},
  "processing_enqueued": true
}
```

## 验证

```bash
make fmt
make lint
make test
pnpm test
pnpm typecheck
pnpm web:build
make migrate
```
