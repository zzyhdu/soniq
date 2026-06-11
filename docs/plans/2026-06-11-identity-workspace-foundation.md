# Identity and Workspace Foundation Implementation Plan

**Goal:** Establish Soniq's first identity, tenancy, and resource-scoping foundation. After this milestone, every user-visible recording belongs to a workspace, the frontend explicitly selects the workspace, the backend checks membership on every workspace-scoped request, and Temporal workflows carry the same `workspace_id` as the API request that created the recording.

This milestone is **not** production authentication. It adds the ownership model that production authentication will later plug into.

---

## 1. Design Decisions

### Backend Does Not Store Active Workspace

Do not use an implicit API shape like:

```txt
GET /recordings
# backend infers active workspace from session state
```

Use explicit workspace-scoped paths:

```txt
GET  /workspaces/{workspace_id}/recordings
POST /workspaces/{workspace_id}/recordings/upload
GET  /workspaces/{workspace_id}/recordings/{recording_id}
```

Benefits:

- Multiple browser tabs can use different workspaces safely.
- CLI/API callers are explicit.
- Authorization and audit boundaries are obvious.
- The backend has no hidden active-workspace session state.

### No `GET /session`

Use two focused bootstrap endpoints instead:

```txt
GET /me
GET /workspaces
```

`GET /me` answers "who am I?"

`GET /workspaces` answers "which workspaces can I access?"

The frontend chooses a workspace. The backend validates that choice.

### No Workspace Slug Yet

Do not add `workspaces.slug` in this milestone. Stable `workspace_id` is enough for API paths. Add a slug later only when Soniq needs user-readable URLs such as `/w/yangsan-lab/...`, invitations, or public sharing links.

### Dev Identity Is a Local Compatibility Layer

Local defaults:

```env
AUTH_MODE=dev
DEV_USER_ID=usr_dev
```

In `AUTH_MODE=dev`, every request resolves to `usr_dev`. This is not security; it keeps local smoke tests, fake providers, manual real-provider runs, and the Web UI low-friction while all resources become workspace-scoped.

### ID Strategy

Keep the existing opaque prefixed id style used by recordings. Do not switch this milestone to UUIDs, slugs, or auto-incrementing public ids.

Rules:

```txt
dev user id: usr_dev
dev workspace id: wsp_default
generated user id: usr_<random_hex>
generated workspace id: wsp_<random_hex>
generated recording id: rec_<random_hex>
```

Requirements:

- IDs must be URL-safe because `workspace_id` appears directly in API paths.
- Public API paths must not use auto-incrementing integers.
- Do not use workspace names, emails, Chinese names, or display names as ids.
- The existing `rec_ + hex` recording id strategy is already URL-safe; workspace ids should keep the same property.

---

## 2. Database Contract

### users

```sql
CREATE TABLE users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

INSERT INTO users (id, email, display_name, created_at, updated_at)
VALUES ('usr_dev', 'dev@local.soniq', 'Local Developer', NOW(), NOW());
```

### workspaces

```sql
CREATE TABLE workspaces (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_by_user_id TEXT REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

INSERT INTO workspaces (id, name, created_by_user_id, created_at, updated_at)
VALUES ('wsp_default', 'Default Workspace', 'usr_dev', NOW(), NOW());
```

### workspace_members

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

INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
VALUES ('wsp_default', 'usr_dev', 'owner', NOW());
```

### recordings.workspace_id

Migration order must preserve existing local data:

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

Child tables continue to inherit workspace ownership through `recording_id -> recordings.workspace_id`:

- `recording_audio_probes`
- `recording_normalized_audios`
- `recording_transcripts`
- `recording_transcript_segments`
- `recording_summaries`

User-facing detail reads must first prove the recording belongs to the requested workspace.

---

## 3. Domain Contract

Add identity models:

```go
type User struct {
    ID          string
    Email       string
    DisplayName string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type Workspace struct {
    ID              string
    Name            string
    CreatedByUserID string
    CreatedAt       time.Time
    UpdatedAt       time.Time
}

type WorkspaceRole string

const (
    WorkspaceRoleOwner  WorkspaceRole = "owner"
    WorkspaceRoleMember WorkspaceRole = "member"
)

type WorkspaceMembership struct {
    WorkspaceID string
    UserID      string
    Role        WorkspaceRole
    CreatedAt   time.Time
}
```

Add `WorkspaceID` to `domain.Recording`:

```go
type Recording struct {
    ID          string `json:"id"`
    WorkspaceID string `json:"workspace_id"`
    // existing fields...
}
```

Validation:

- `WorkspaceRole` accepts only `owner` and `member`.
- `Recording.WorkspaceID` is required.
- recording creation requires `WorkspaceID`.

---

## 4. Backend Identity and Membership Boundary

Resolve only the current user from auth:

```go
type CurrentUser struct {
    UserID string
}

type AuthResolver interface {
    ResolveCurrentUser(r *http.Request) (CurrentUser, error)
}
```

Then authorize the requested workspace with membership lookups:

```go
type WorkspaceStore interface {
    GetUser(ctx context.Context, userID string) (domain.User, bool, error)
    ListWorkspacesForUser(ctx context.Context, userID string) ([]domain.WorkspaceWithRole, error)
    GetWorkspaceForUser(ctx context.Context, userID string, workspaceID string) (domain.WorkspaceWithRole, bool, error)
}
```

Every workspace-scoped API follows:

```txt
current user -> path workspace_id -> membership check -> resource operation
```

If membership is missing, return `404 Not Found` to avoid disclosing workspace existence.

---

## 5. API Contract

### `GET /me`

Returns the current user:

```json
{
  "id": "usr_dev",
  "email": "dev@local.soniq",
  "display_name": "Local Developer",
  "created_at": "2026-06-11T00:00:00Z",
  "updated_at": "2026-06-11T00:00:00Z"
}
```

### `GET /workspaces`

Returns workspaces accessible to the current user:

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

### `GET /workspaces/{workspace_id}/recordings`

Lists recordings in the requested workspace.

Rules:

- Default `limit=50`.
- Maximum `limit=100`.
- Sort by `created_at DESC`.
- No cursor pagination in this milestone.

### `POST /workspaces/{workspace_id}/recordings`

Creates a metadata-only recording in the requested workspace. It does not upload audio or start processing.

### `POST /workspaces/{workspace_id}/recordings/upload`

Uploads audio, creates a recording in the requested workspace, and starts the Temporal workflow.

New upload object keys use:

```txt
workspaces/{workspace_id}/recordings/{timestamp}/{filename}
```

### `GET /workspaces/{workspace_id}/recordings/{recording_id}`

Returns recording metadata only if the recording belongs to the requested workspace.

### `GET /workspaces/{workspace_id}/recordings/{recording_id}/status`

Returns:

```json
{
  "id": "rec_...",
  "workspace_id": "wsp_default",
  "status": "transcribing"
}
```

### `GET /workspaces/{workspace_id}/recordings/{recording_id}/details`

Returns the existing details shape, with `recording.workspace_id` included.

---

## 6. Recording Store Contract

Creation:

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

User-facing get:

```go
type GetRecordingInput struct {
    WorkspaceID string
    ID          string
}

Get(input GetRecordingInput) (domain.Recording, bool, error)
```

List:

```go
type ListRecordingsInput struct {
    WorkspaceID string
    Limit       int
}

ListByWorkspace(input ListRecordingsInput) ([]domain.Recording, error)
```

Workflow status update:

```go
type UpdateRecordingStatusInput struct {
    WorkspaceID string
    ID          string
    Status      domain.RecordingStatus
}
```

User-facing recording queries and workflow status updates must use `workspace_id + recording_id`, not recording id alone.

---

## 7. Storage Key Contract

New uploads:

```txt
workspaces/{workspace_id}/recordings/{timestamp}/{filename}
```

Existing keys stay readable:

```txt
recordings/{timestamp}/{filename}
```

The database `audio_object_key` remains the source of truth. Do not migrate old objects in this milestone.

Normalized audio continues to use the deterministic sibling key, so new uploads produce:

```txt
workspaces/{workspace_id}/recordings/{timestamp}/normalized.wav
```

---

## 8. Temporal Contract

Add `WorkspaceID`:

```go
type RecordingProcessingInput struct {
    WorkspaceID string
    RecordingID string
    WorkflowType string
    Language string
    DeleteOriginalAudioAfterTranscription bool
}
```

Requirements:

- API enqueue uses `recording.WorkspaceID`.
- `ValidateRecording` loads by `WorkspaceID + RecordingID`.
- Status updates include `WorkspaceID`.
- A mismatched workspace/recording pair fails the workflow with clear context.

---

## 9. Frontend Behavior

On startup, load:

```txt
GET /me
GET /workspaces
```

Workspace selection order:

1. Use `workspace_id` from the URL query if present.
2. Otherwise use `localStorage.soniq.workspace_id`.
3. Otherwise auto-select if exactly one workspace is available.
4. Otherwise render a workspace picker.

After selection:

- Persist to localStorage.
- Reflect in the URL query.
- Pass the selected workspace id to every recording API call.

Web UI additions:

- Current user display.
- Workspace switcher.
- Recording list.
- Selecting an existing recording loads status/details.
- Upload refreshes the selected workspace's list.

---

## 10. TypeScript API Client Contract

Add:

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

`Recording` includes:

```ts
workspace_id: string;
```

Remove or migrate unscoped recording helpers so the Web UI cannot accidentally call global recording APIs.

---

## 11. Implementation Tasks

### L1: Update plan documents

Files:

- `docs/plans/2026-06-11-identity-workspace-foundation.md`
- `docs/plans/2026-06-11-identity-workspace-foundation.zh-CN.md`

Acceptance:

- The plan uses explicit workspace paths.
- The plan no longer uses `slug`, `GET /session`, or backend active workspace state.

### L2: Add database migration

Files:

- `backend/migrations/0006_create_identity_workspace_foundation.up.sql`
- `backend/migrations/0006_create_identity_workspace_foundation.down.sql`

Acceptance:

- Identity tables exist.
- `usr_dev`, `wsp_default`, and owner membership are seeded.
- `recordings.workspace_id` is backfilled and required.
- `(workspace_id, created_at DESC)` index exists.

### L3: Add domain and config

Files:

- `backend/internal/domain/identity.go`
- `backend/internal/domain/identity_test.go`
- `backend/internal/domain/recording.go`
- `backend/internal/domain/recording_test.go`
- `backend/internal/config/config.go`
- `backend/internal/config/config_test.go`
- `.env.example`

Acceptance:

- Identity domain types exist.
- `Recording.WorkspaceID` exists.
- `AUTH_MODE=dev` and `DEV_USER_ID=usr_dev` are configured.
- Unsupported non-dev auth modes fail clearly until implemented.

### L4: Add workspace store

Files:

- `backend/internal/workspaces/store.go`
- `backend/internal/workspaces/postgres_store.go`
- `backend/internal/workspaces/postgres_store_test.go`

Acceptance:

- Current dev user can be loaded.
- `ListWorkspacesForUser` returns `wsp_default`.
- `GetWorkspaceForUser` validates membership.
- Non-members get not found.

### L5: Scope recording persistence

Files:

- `backend/internal/recordings/store.go`
- `backend/internal/recordings/postgres_store.go`
- `backend/internal/recordings/postgres_store_test.go`

Acceptance:

- Create requires `WorkspaceID`.
- Get uses `workspace_id + id`.
- `ListByWorkspace` exists.
- Status updates use `workspace_id + id`.
- Tests cover cross-workspace read/update denial.

### L6: Add identity middleware and workspace routes

Files:

- `backend/internal/api/identity.go`
- `backend/internal/api/identity_test.go`
- `backend/internal/api/router.go`
- `backend/internal/api/recordings_test.go`

Acceptance:

- `GET /me` works.
- `GET /workspaces` works.
- Workspace-scoped recording endpoints work.
- Missing membership returns `404`.
- Web/client no longer use unscoped recording endpoints.

### L7: Pass workspace through Temporal

Files:

- `backend/internal/processing/temporal_recording_processor.go`
- `backend/internal/processing/temporal_recording_processor_test.go`
- `backend/internal/workflows/recording_processing.go`
- `backend/internal/workflows/recording_processing_test.go`
- `backend/internal/activities/recording_processing.go`
- focused activity tests

Acceptance:

- Workflow input includes `WorkspaceID`.
- Processor starts workflows with `recording.WorkspaceID`.
- Validation and status updates are workspace-aware.

### L8: Use workspace-scoped object keys

Files:

- `backend/internal/api/router.go`
- `backend/internal/api/recordings_test.go`
- optional storage key helper/test

Acceptance:

- New upload keys start with `workspaces/{workspace_id}/recordings/`.
- Existing `recordings/...` keys remain readable.

### L9: Update OpenAPI and API client

Files:

- `backend/doc/openapi.yaml`
- `packages/api-client/src/recordings.ts`
- `packages/api-client/src/workspaces.ts`
- `packages/api-client/src/users.ts`
- `packages/api-client/src/index.ts`
- focused API client tests

Acceptance:

- OpenAPI includes `/me`, `/workspaces`, and workspace-scoped recording paths.
- `Recording` includes `workspace_id`.
- Recording client functions require `workspaceId`.

### L10: Update Web UI

Files:

- `apps/web/src/api/queries.ts`
- `apps/web/src/App.tsx`
- likely new components:
  - `apps/web/src/components/WorkspaceSwitcher.tsx`
  - `apps/web/src/components/RecordingList.tsx`
  - `apps/web/src/components/UserMenu.tsx`

Acceptance:

- UI loads user and workspaces.
- UI selects a workspace.
- Recording list loads for the selected workspace.
- Upload uses the selected workspace.
- Selecting history loads status/details.

### L11: Update docs and smoke

Files:

- `docs/development.md`
- `docs/architecture.md`
- `docs/workflows.md`
- `.env.example`
- `scripts/smoke-postgres-temporal.sh`
- `scripts/smoke-openai-compatible-asr-fake.sh`

Acceptance:

- Docs explain dev identity.
- Smoke uses `/workspaces/wsp_default/recordings/upload`.
- Smoke asserts `recordings.workspace_id = 'wsp_default'`.
- Default fake-provider smoke still passes.

---

## 12. Verification

Backend:

```bash
make fmt
make lint
make test
```

Frontend:

```bash
pnpm test
pnpm typecheck
pnpm web:build
```

End-to-end smoke:

```bash
make smoke-postgres-temporal
```

Focused checks:

```bash
cd backend && go test ./internal/domain ./internal/config ./internal/workspaces ./internal/recordings ./internal/api -v
cd backend && go test ./internal/processing ./internal/workflows ./internal/activities -v
pnpm --filter @soniq/api-client test
pnpm --filter @soniq/web test
```

---

## 13. Acceptance Criteria

- No recording exists without `workspace_id`.
- No user-facing global recording read remains.
- Frontend recording API calls pass `workspace_id` explicitly.
- `GET /me` returns only the current user.
- `GET /workspaces` returns accessible workspaces.
- Every workspace-scoped request validates membership.
- New upload object keys include the workspace prefix.
- Temporal workflow input includes `workspace_id`.
- Workflow status updates cannot cross workspace boundaries.
- Web UI can choose a workspace, upload, list history, and show results.
- Default local smoke remains fake-provider based.

---

## 14. Follow-up Milestones

1. **Result Usability**
   - Better recording detail view.
   - Markdown export.
   - Failed-state recovery and retry.

2. **Production Auth**
   - Choose one main path: OIDC, email/password sessions, or reverse-proxy trusted identity.
   - Build one complete path instead of several partial auth modes.

3. **Workspace Provider Settings**
   - Store provider credentials and model settings at workspace scope.
   - Keep external provider usage explicit and auditable.

4. **Audit and Retention**
   - Audit upload, workflow completion/failure, transcript generation, summary generation, deletion, and provider config changes.
   - Add workspace-level retention policy.
