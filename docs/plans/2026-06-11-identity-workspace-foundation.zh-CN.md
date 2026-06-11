# 用户与工作区基础体系实施计划

**目标：** 给 Soniq 建立第一层产品级身份与数据归属边界。完成后，所有用户可见的 recording 都必须属于一个 workspace；前端显式选择 workspace；后端每次请求都校验用户是否属于该 workspace；Temporal worker 处理 recording 时也携带并校验同一个 `workspace_id`。

**重要结论：** 本阶段不是做完整登录系统，而是做 **Identity / Tenancy / Resource Scoping** 基础。也就是先回答清楚：

- 当前请求是谁发起的？
- 这个用户属于哪些 workspace？
- 当前请求要操作哪个 workspace？
- recording 是否属于这个 workspace？
- 后台 workflow 和音频产物是否也继承同一个 workspace 归属？

---

## 1. 明确设计决策

### 1.1 后端不保存 active workspace

不采用：

```txt
GET /recordings
# 后端从 session 里猜当前 workspace
```

采用：

```txt
GET /workspaces/{workspace_id}/recordings
POST /workspaces/{workspace_id}/recordings/upload
GET /workspaces/{workspace_id}/recordings/{recording_id}
```

原因：

- 多浏览器 tab 不会互相影响。
- CLI/API 调用更直观。
- 审计和权限判断更清楚。
- 后端没有隐藏的 active workspace 状态。

### 1.2 不做 `GET /session`

本阶段拆成两个更明确的接口：

```txt
GET /me
GET /workspaces
```

`GET /me` 只回答“我是谁”。

`GET /workspaces` 只回答“我能访问哪些 workspace”。

前端负责选择 workspace。后端只负责校验。

### 1.3 不加 workspace slug

本阶段不加：

```sql
slug TEXT NOT NULL UNIQUE
```

原因：

- 当前不做用户可读 workspace URL。
- 当前不做邀请链接或公开分享链接。
- API 使用稳定的 `workspace_id` 已足够。

后续如果需要 `/w/yangsan-lab/...` 这类 URL，再单独加 `slug`。

### 1.4 Dev identity 只是本地兼容层

默认本地开发模式：

```env
AUTH_MODE=dev
DEV_USER_ID=usr_dev
```

`AUTH_MODE=dev` 下，每个请求都被认为来自 `usr_dev`。这不是生产认证，只是为了让本地 smoke、Web UI、fake provider 和真实 provider 手动验证继续低摩擦运行。

### 1.5 ID 生成规则

本阶段继续沿用当前 recording id 的 opaque prefixed id 风格，不改成 UUID，也不使用自增整数。

规则：

```txt
dev user id: usr_dev
dev workspace id: wsp_default
generated user id: usr_<random_hex>
generated workspace id: wsp_<random_hex>
generated recording id: rec_<random_hex>
```

要求：

- ID 必须 URL-safe，因为 `workspace_id` 会直接出现在 API path 中。
- 公开 API path 中不要使用自增整数。
- 不要用 workspace name、email、中文名或展示名作为 id。
- 当前已有 `rec_` 生成方式是 `rec_ + hex`，已经 URL-safe；workspace id 应保持同样性质。

---

## 2. 数据库变更

### 2.1 新增 users

```sql
CREATE TABLE users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
```

默认 seed：

```sql
INSERT INTO users (id, email, display_name, created_at, updated_at)
VALUES ('usr_dev', 'dev@local.soniq', 'Local Developer', NOW(), NOW());
```

### 2.2 新增 workspaces

```sql
CREATE TABLE workspaces (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_by_user_id TEXT REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
```

默认 seed：

```sql
INSERT INTO workspaces (id, name, created_by_user_id, created_at, updated_at)
VALUES ('wsp_default', 'Default Workspace', 'usr_dev', NOW(), NOW());
```

### 2.3 新增 workspace_members

```sql
CREATE TABLE workspace_members (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (workspace_id, user_id),
  CONSTRAINT workspace_members_role_check
    CHECK (role IN ('owner', 'member'))
);
```

默认 seed：

```sql
INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
VALUES ('wsp_default', 'usr_dev', 'owner', NOW());
```

### 2.4 recordings 增加 workspace_id

迁移顺序必须可兼容已有本地数据：

```sql
ALTER TABLE recordings
  ADD COLUMN workspace_id TEXT;

UPDATE recordings
SET workspace_id = 'wsp_default'
WHERE workspace_id IS NULL;

ALTER TABLE recordings
  ALTER COLUMN workspace_id SET NOT NULL;

ALTER TABLE recordings
  ADD CONSTRAINT recordings_workspace_id_fkey
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id);

CREATE INDEX recordings_workspace_created_at_idx
  ON recordings (workspace_id, created_at DESC);
```

### 2.5 子表暂不重复存 workspace_id

以下表继续通过 `recording_id` 间接归属 workspace：

- `recording_audio_probes`
- `recording_normalized_audios`
- `recording_transcripts`
- `recording_transcript_segments`
- `recording_summaries`

用户侧 API 读取这些子表之前，必须先用：

```sql
WHERE recordings.workspace_id = $1
  AND recordings.id = $2
```

确认 recording 属于当前 workspace。

---

## 3. Domain 模型

新增：

```go
type User struct {
    ID          string    `json:"id"`
    Email       string    `json:"email"`
    DisplayName string    `json:"display_name"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type Workspace struct {
    ID              string    `json:"id"`
    Name            string    `json:"name"`
    CreatedByUserID string    `json:"created_by_user_id"`
    CreatedAt       time.Time `json:"created_at"`
    UpdatedAt       time.Time `json:"updated_at"`
}

type WorkspaceRole string

const (
    WorkspaceRoleOwner  WorkspaceRole = "owner"
    WorkspaceRoleMember WorkspaceRole = "member"
)

type WorkspaceMembership struct {
    WorkspaceID string        `json:"workspace_id"`
    UserID      string        `json:"user_id"`
    Role        WorkspaceRole `json:"role"`
    CreatedAt   time.Time     `json:"created_at"`
}
```

修改：

```go
type Recording struct {
    ID          string `json:"id"`
    WorkspaceID string `json:"workspace_id"`
    // existing fields...
}
```

验证规则：

- `WorkspaceRole` 只允许 `owner` / `member`。
- `Recording.WorkspaceID` 不能为空。
- 创建 recording 时必须传入 `WorkspaceID`。

---

## 4. 后端身份与权限边界

### 4.1 当前用户解析

引入 dev auth resolver：

```go
type CurrentUser struct {
    UserID string
}

type AuthResolver interface {
    ResolveCurrentUser(r *http.Request) (CurrentUser, error)
}
```

dev 实现：

```go
type DevAuthResolver struct {
    UserID string
}
```

`AUTH_MODE=dev` 下返回：

```txt
usr_dev
```

### 4.2 Workspace membership 校验

引入 store 方法：

```go
type WorkspaceStore interface {
    GetUser(ctx context.Context, userID string) (domain.User, bool, error)
    ListWorkspacesForUser(ctx context.Context, userID string) ([]domain.WorkspaceWithRole, error)
    GetWorkspaceForUser(ctx context.Context, userID string, workspaceID string) (domain.WorkspaceWithRole, bool, error)
}
```

每个 workspace-scoped API 都必须执行：

```txt
current user -> path workspace_id -> membership check
```

如果用户不属于该 workspace：

```txt
404 Not Found
```

而不是 `403`。这样可以避免泄露 workspace 是否存在。后续如果产品需要更明确的权限错误，可再调整。

---

## 5. API 合约

### 5.1 GET /me

返回当前用户。

```http
GET /me
```

响应：

```json
{
  "id": "usr_dev",
  "email": "dev@local.soniq",
  "display_name": "Local Developer",
  "created_at": "2026-06-11T00:00:00Z",
  "updated_at": "2026-06-11T00:00:00Z"
}
```

### 5.2 GET /workspaces

返回当前用户可访问的 workspace 列表。

```http
GET /workspaces
```

响应：

```json
{
  "workspaces": [
    {
      "id": "wsp_default",
      "name": "Default Workspace",
      "role": "owner",
      "created_at": "2026-06-11T00:00:00Z",
      "updated_at": "2026-06-11T00:00:00Z"
    }
  ]
}
```

### 5.3 GET /workspaces/{workspace_id}/recordings

返回当前 workspace 的 recording 列表。

```http
GET /workspaces/wsp_default/recordings?limit=50
```

响应：

```json
{
  "recordings": [
    {
      "id": "rec_...",
      "workspace_id": "wsp_default",
      "title": "Weekly sync",
      "status": "completed",
      "workflow_type": "meeting",
      "language": "zh",
      "audio_object_key": "workspaces/wsp_default/recordings/...",
      "audio_content_type": "audio/wav",
      "audio_size_bytes": 32078,
      "created_at": "2026-06-11T00:00:00Z",
      "updated_at": "2026-06-11T00:00:00Z"
    }
  ]
}
```

初始规则：

- 默认 `limit=50`。
- 最大 `limit=100`。
- 按 `created_at DESC` 排序。
- 暂不做 cursor pagination。

### 5.4 POST /workspaces/{workspace_id}/recordings

创建 metadata-only recording，不上传音频，不启动 workflow。

```http
POST /workspaces/wsp_default/recordings
Content-Type: application/json
```

请求：

```json
{
  "title": "Weekly sync",
  "workflow_type": "meeting",
  "language": "zh"
}
```

响应：`201 Created`

```json
{
  "id": "rec_...",
  "workspace_id": "wsp_default",
  "title": "Weekly sync",
  "status": "uploaded",
  "workflow_type": "meeting",
  "language": "zh",
  "created_at": "2026-06-11T00:00:00Z",
  "updated_at": "2026-06-11T00:00:00Z"
}
```

### 5.5 POST /workspaces/{workspace_id}/recordings/upload

上传音频、创建 recording、启动 Temporal workflow。

```http
POST /workspaces/wsp_default/recordings/upload
Content-Type: multipart/form-data
```

表单字段：

```txt
workflow_type=meeting
title=Weekly sync
language=zh
audio=@demo.wav
```

响应：

```json
{
  "recording": {
    "id": "rec_...",
    "workspace_id": "wsp_default",
    "title": "Weekly sync",
    "status": "uploaded",
    "workflow_type": "meeting",
    "language": "zh",
    "audio_object_key": "workspaces/wsp_default/recordings/20260611T120000.000000000Z/demo.wav",
    "audio_content_type": "audio/wav",
    "audio_size_bytes": 32078,
    "created_at": "2026-06-11T00:00:00Z",
    "updated_at": "2026-06-11T00:00:00Z"
  },
  "processing_enqueued": true
}
```

### 5.6 GET /workspaces/{workspace_id}/recordings/{recording_id}

获取 recording metadata。

```http
GET /workspaces/wsp_default/recordings/rec_...
```

如果 recording 不属于该 workspace，返回 `404`。

### 5.7 GET /workspaces/{workspace_id}/recordings/{recording_id}/status

获取处理状态。

响应：

```json
{
  "id": "rec_...",
  "workspace_id": "wsp_default",
  "status": "transcribing"
}
```

### 5.8 GET /workspaces/{workspace_id}/recordings/{recording_id}/details

获取 recording、transcript segments、summary。

响应结构保持现有 details API，但 `recording` 中必须包含 `workspace_id`。

---

## 6. Recording Store 合约

### 6.1 创建

```go
type CreateRecordingInput struct {
    WorkspaceID      string
    Title            string
    WorkflowType     domain.WorkflowType
    Language         string
    AudioObjectKey   string
    AudioContentType string
    AudioSizeBytes   int64
}
```

校验：

- `WorkspaceID` 必填。
- `WorkflowType` 必须合法。
- `AudioSizeBytes` 如果非零必须为正数。

### 6.2 查询单条

```go
type GetRecordingInput struct {
    WorkspaceID string
    ID          string
}

Get(input GetRecordingInput) (domain.Recording, bool, error)
```

SQL 必须类似：

```sql
SELECT ...
FROM recordings
WHERE workspace_id = $1
  AND id = $2;
```

不能只按 `id` 查询用户侧资源。

### 6.3 列表

```go
type ListRecordingsInput struct {
    WorkspaceID string
    Limit       int
}

ListByWorkspace(input ListRecordingsInput) ([]domain.Recording, error)
```

SQL：

```sql
SELECT ...
FROM recordings
WHERE workspace_id = $1
ORDER BY created_at DESC
LIMIT $2;
```

### 6.4 状态更新

后台 workflow 使用：

```go
type UpdateRecordingStatusInput struct {
    WorkspaceID string
    ID          string
    Status      domain.RecordingStatus
}
```

SQL：

```sql
UPDATE recordings
SET status = $3,
    updated_at = $4
WHERE workspace_id = $1
  AND id = $2
RETURNING ...;
```

这样即使 Temporal workflow 收到错误的 `workspace_id + recording_id` 组合，也不会跨 workspace 更新。

---

## 7. 对象存储 Key

新上传对象 key：

```txt
workspaces/{workspace_id}/recordings/{timestamp}/{filename}
```

示例：

```txt
workspaces/wsp_default/recordings/20260611T120000.000000000Z/demo.wav
```

旧 key 不迁移，仍可读：

```txt
recordings/{timestamp}/{filename}
```

原因：

- 数据库中的 `audio_object_key` 是 source of truth。
- worker 只需要按存储的 key 读取对象。
- 迁移对象文件不是本阶段目标。

normalized audio 继续沿用 sibling key：

```txt
workspaces/wsp_default/recordings/20260611T120000.000000000Z/normalized.wav
```

---

## 8. Temporal 合约

修改 workflow input：

```go
type RecordingProcessingInput struct {
    WorkspaceID string
    RecordingID string
    WorkflowType string
    Language string
    DeleteOriginalAudioAfterTranscription bool
}
```

要求：

- API enqueue 时使用 `recording.WorkspaceID`。
- workflow 第一阶段 `ValidateRecording` 使用 `WorkspaceID + RecordingID` 查询。
- 所有状态更新都带 `WorkspaceID`。
- 如果 workspace 不匹配，workflow 失败并记录清晰错误。

---

## 9. 前端行为

### 9.1 启动时加载

Web UI 启动时并行请求：

```txt
GET /me
GET /workspaces
```

### 9.2 workspace 选择规则

当前阶段不引入复杂路由库也可以完成，但选择规则必须明确：

1. 如果 URL query 有 `workspace_id`，优先使用。
2. 否则读取 `localStorage.soniq.workspace_id`。
3. 如果只有一个 workspace，自动选择它。
4. 如果有多个 workspace 且无法自动选择，显示 workspace picker。

选择后：

- 写入 localStorage。
- 更新 URL query。
- 所有 recording API 都使用该 workspace id。

### 9.3 Web UI 新状态

需要新增：

- 当前用户显示。
- 当前 workspace selector。
- recording list。
- 点击列表项后加载该 recording 的 status/details。
- 上传成功后刷新当前 workspace 的 recording list。

---

## 10. TypeScript API Client 合约

新增类型：

```ts
export type User = {
  id: string;
  email: string;
  display_name: string;
  created_at: string;
  updated_at: string;
};

export type WorkspaceRole = 'owner' | 'member';

export type Workspace = {
  id: string;
  name: string;
  role: WorkspaceRole;
  created_at: string;
  updated_at: string;
};
```

`Recording` 增加：

```ts
workspace_id: string;
```

新增函数：

```ts
getMe()
listWorkspaces()
listRecordings(workspaceId)
createRecording(workspaceId, input)
uploadRecording(workspaceId, input)
getRecording(workspaceId, recordingId)
getRecordingStatus(workspaceId, recordingId)
getRecordingDetails(workspaceId, recordingId)
```

旧的无 workspace 参数函数应在本阶段迁移或删除，避免前端继续调用全局 API。

---

## 11. 实施任务

### L1：更新计划文档

**文件：**

- `docs/plans/2026-06-11-identity-workspace-foundation.md`
- `docs/plans/2026-06-11-identity-workspace-foundation.zh-CN.md`

**验收：**

- 文档明确采用显式 workspace path。
- 文档不再使用 `slug`、`GET /session`、后端 active workspace。

### L2：数据库迁移

**文件：**

- `backend/migrations/0006_create_identity_workspace_foundation.up.sql`
- `backend/migrations/0006_create_identity_workspace_foundation.down.sql`

**验收：**

- 创建 `users`、`workspaces`、`workspace_members`。
- seed `usr_dev`、`wsp_default`、owner membership。
- `recordings.workspace_id` 被回填并设置为 `NOT NULL`。
- 有 `(workspace_id, created_at DESC)` 索引。

### L3：Domain 和 config

**文件：**

- `backend/internal/domain/identity.go`
- `backend/internal/domain/identity_test.go`
- `backend/internal/domain/recording.go`
- `backend/internal/domain/recording_test.go`
- `backend/internal/config/config.go`
- `backend/internal/config/config_test.go`
- `.env.example`

**验收：**

- `User`、`Workspace`、`WorkspaceMembership`、`WorkspaceRole` 存在。
- `Recording.WorkspaceID` 存在。
- `AUTH_MODE=dev` 和 `DEV_USER_ID=usr_dev` 有默认值。
- 非 dev auth mode 如果未实现，启动时清晰失败。

### L4：Workspace store

**文件：**

- 可新建 `backend/internal/workspaces/store.go`
- 可新建 `backend/internal/workspaces/postgres_store.go`
- 可新建 `backend/internal/workspaces/postgres_store_test.go`

**验收：**

- `GetUser` 可以读取 dev user。
- `ListWorkspacesForUser` 返回 `wsp_default`。
- `GetWorkspaceForUser` 校验 membership。
- 非成员访问 workspace 返回 not found。

### L5：Recording store workspace scope

**文件：**

- `backend/internal/recordings/store.go`
- `backend/internal/recordings/postgres_store.go`
- `backend/internal/recordings/postgres_store_test.go`

**验收：**

- `CreateRecordingInput.WorkspaceID` 必填。
- `Get` 使用 `workspace_id + id`。
- `ListByWorkspace` 存在。
- `UpdateStatus` 使用 `workspace_id + id`。
- 测试覆盖跨 workspace 不能读取或更新。

### L6：API identity 和 workspace 路由

**文件：**

- `backend/internal/api/identity.go`
- `backend/internal/api/identity_test.go`
- `backend/internal/api/router.go`
- `backend/internal/api/recordings_test.go`

**验收：**

- `GET /me` 可用。
- `GET /workspaces` 可用。
- 新 workspace-scoped recording endpoints 可用。
- 无 membership 时返回 `404`。
- 旧无 workspace recording endpoints 不再被 Web/client 使用。

### L7：Temporal workspace 传递

**文件：**

- `backend/internal/processing/temporal_recording_processor.go`
- `backend/internal/processing/temporal_recording_processor_test.go`
- `backend/internal/workflows/recording_processing.go`
- `backend/internal/workflows/recording_processing_test.go`
- `backend/internal/activities/recording_processing.go`
- 相关 activity 测试

**验收：**

- workflow input 有 `WorkspaceID`。
- enqueue 使用 `recording.WorkspaceID`。
- `ValidateRecording` 按 workspace 查询。
- 状态更新按 workspace 更新。

### L8：对象 key workspace 前缀

**文件：**

- `backend/internal/api/router.go`
- `backend/internal/api/recordings_test.go`
- 必要时新增 storage key helper/test

**验收：**

- 新上传 key 以 `workspaces/{workspace_id}/recordings/` 开头。
- 旧 key 仍可通过已有 storage resolver 读取。

### L9：OpenAPI 和 API client

**文件：**

- `backend/doc/openapi.yaml`
- `packages/api-client/src/recordings.ts`
- `packages/api-client/src/workspaces.ts`
- `packages/api-client/src/users.ts`
- `packages/api-client/src/index.ts`
- 相关测试

**验收：**

- OpenAPI 包含 `/me`、`/workspaces`、workspace-scoped recording paths。
- `Recording` schema 包含 `workspace_id`。
- TypeScript client 所有 recording 函数都需要 `workspaceId`。

### L10：Web UI workspace 选择和历史列表

**文件：**

- `apps/web/src/api/queries.ts`
- `apps/web/src/App.tsx`
- 可新增：
  - `apps/web/src/components/WorkspaceSwitcher.tsx`
  - `apps/web/src/components/RecordingList.tsx`
  - `apps/web/src/components/UserMenu.tsx`

**验收：**

- UI 加载当前 user。
- UI 加载 workspace list。
- UI 选择 workspace 后加载 recording list。
- 上传使用选中的 workspace id。
- 上传成功刷新 list。
- 点击历史 recording 后加载 status/details。

### L11：文档和 smoke

**文件：**

- `docs/development.md`
- `docs/architecture.md`
- `docs/workflows.md`
- `.env.example`
- `scripts/smoke-postgres-temporal.sh`
- `scripts/smoke-openai-compatible-asr-fake.sh`

**验收：**

- 文档说明 dev identity。
- smoke 使用 `/workspaces/wsp_default/recordings/upload`。
- smoke 断言 `recordings.workspace_id = 'wsp_default'`。
- 默认 fake provider smoke 仍通过。

---

## 12. 验证命令

后端：

```bash
make fmt
make lint
make test
```

前端：

```bash
pnpm test
pnpm typecheck
pnpm web:build
```

完整 smoke：

```bash
make smoke-postgres-temporal
```

重点 focused checks：

```bash
cd backend && go test ./internal/domain ./internal/config ./internal/workspaces ./internal/recordings ./internal/api -v
cd backend && go test ./internal/processing ./internal/workflows ./internal/activities -v
pnpm --filter @soniq/api-client test
pnpm --filter @soniq/web test
```

---

## 13. 完成标准

本阶段完成后必须满足：

- 数据库中不存在没有 `workspace_id` 的 recording。
- 后端没有用户侧全局 recording 查询路径。
- 前端 recording API 全部显式传 `workspace_id`。
- `GET /me` 只返回当前用户。
- `GET /workspaces` 返回当前用户可访问 workspace。
- 后端每次 workspace-scoped 请求都校验 membership。
- 新上传对象 key 带 workspace 前缀。
- Temporal workflow input 带 `workspace_id`。
- workflow 状态更新不会跨 workspace。
- Web UI 可以选择 workspace、上传、查看历史 recording、查看结果。
- 默认本地 smoke 仍然不依赖真实外部模型 provider。

---

## 14. 后续阶段

完成该基础后，再进入：

1. **结果可用性**
   - 更完整的 recording detail 页面。
   - Markdown 导出。
   - failed 状态恢复和 retry。

2. **生产认证**
   - 在 OIDC、邮箱密码 session、反向代理 trusted identity 中选一个主路径。
   - 不同时铺多个半成品 auth 方案。

3. **Workspace Provider Settings**
   - provider credentials 和 model settings 挂到 workspace。
   - 外部 provider 使用必须显式、可审计。

4. **Audit / Retention**
   - upload、workflow completion/failure、transcript generation、summary generation、delete、provider config change 全部进入 audit。
   - 增加 workspace 级 retention policy。
