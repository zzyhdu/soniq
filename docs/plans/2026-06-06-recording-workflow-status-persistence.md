# Recording Workflow Status Persistence Implementation Plan

> **For Hermes:** Execute this plan task-by-task. Keep repository docs tool-agnostic and use small commits. Ask before every commit.

**Goal:** Make `RecordingProcessingWorkflow` update recording status in Soniq Postgres so uploaded recordings move from `uploaded` to `processing` and then `completed` or `failed` through the real worker path.

**Architecture:** Extend the recording persistence seam with status updates, then convert the current placeholder processing activities into store-backed activity methods. Keep workflow orchestration deterministic: workflow code only schedules activities; activities own database I/O. Wire the worker command to Soniq Postgres separately from Temporal's internal database, mirroring the API command's Soniq application database boundary.

**Tech Stack:** Go, Temporal Go SDK, Postgres via existing pgx pool wiring, existing `recordings.PostgresStore`, existing local Docker Compose smoke stack.

---

## Current-state findings

- `backend/internal/workflows/recording_processing.go` already runs three activities in order:
  1. `ValidateRecordingActivity`
  2. `MarkRecordingProcessingActivity`
  3. `CompleteRecordingProcessingActivity`
- `backend/internal/activities/recording_processing.go` currently contains function-style activity stubs. They validate input and return a completed result, but they do not read or write the recording store.
- `backend/internal/domain/recording.go` already defines the needed statuses:
  - `uploaded`
  - `processing`
  - `completed`
  - `failed`
- `backend/internal/recordings/store.go` supports `Create` and `Get`, but does not yet support status updates.
- `backend/internal/recordings/postgres_store.go` inserts and reads audio metadata, but does not yet update `status` / `updated_at`.
- `backend/cmd/worker/main.go` currently registers function activities directly and does not open Soniq Postgres.
- `scripts/smoke-postgres-temporal.sh` already uploads audio, waits for Temporal `COMPLETED`, and checks metadata after API restart; it does not yet assert `GET /recordings/{id}/status` eventually returns `completed`.

## Non-goals for this milestone

- No ffmpeg probe/normalize implementation.
- No ASR or LLM provider calls.
- No transcript/summary persistence.
- No retry UI, cancellation UI, or reprocessing API.
- No S3-compatible object store.
- No new production migration unless the existing `recordings.status` and `recordings.updated_at` columns prove insufficient.

## Target behavior

After a successful audio upload:

```txt
POST /recordings/upload
  -> recordings.status = uploaded
  -> Temporal workflow starts
  -> MarkRecordingProcessingActivity updates status = processing
  -> CompleteRecordingProcessingActivity updates status = completed
  -> GET /recordings/{id}/status returns completed
```

If a processing activity fails after the workflow has enough context to identify the recording, a failure-marking activity should be available so the workflow can best-effort persist `status = failed` before returning the original error.

---

## Task F1: Add workflow status persistence plan and work items

**Objective:** Save this plan and create parked work items for the status persistence milestone without modifying implementation files.

**Files:**

- Create: `docs/plans/2026-06-06-recording-workflow-status-persistence.md`

**Plan-only guardrail:**

- Capture `git status --short` before and after.
- Only this plan file and task-board metadata may change.
- Do not edit Go source, tests, migrations, or runtime docs in this task.

**Verification:**

```bash
git status --short
```

Expected: only the plan file is modified/created in git.

**Suggested commit:**

```txt
docs: add recording workflow status persistence plan
```

---

## Task F2: Add recording status update RED tests

**Objective:** Define the recording store contract for status updates before implementation.

**Files:**

- Modify: `backend/internal/recordings/store_test.go`
- Modify: `backend/internal/recordings/postgres_store_test.go`

**TDD expectation:**

Add failing tests for:

1. `MemoryStore.UpdateStatus` changes `Status` and advances `UpdatedAt` while preserving title, workflow type, language, and audio metadata.
2. `MemoryStore.UpdateStatus` returns an error for missing recording IDs.
3. `PostgresStore.UpdateStatus` issues an `UPDATE recordings SET status = ..., updated_at = ... WHERE id = ... RETURNING ...` style operation and scans the full updated recording row.
4. `PostgresStore.UpdateStatus` returns a not-found style error when no row exists.

**Implementation guidance for tests:**

Prefer a small input type:

```go
type UpdateRecordingStatusInput struct {
    ID     string
    Status domain.RecordingStatus
}
```

Expected method shape:

```go
UpdateStatus(input UpdateRecordingStatusInput) (domain.Recording, error)
```

**RED verification:**

```bash
cd backend && go test ./internal/recordings -run 'UpdateStatus' -v
```

Expected: FAIL because `UpdateRecordingStatusInput` / `UpdateStatus` do not exist yet.

**Suggested commit after GREEN in F3:** no commit in this RED-only task unless the user explicitly wants RED tests committed separately.

---

## Task F3: Implement recording status updates in stores

**Objective:** Make the RED store tests pass for memory and Postgres-backed recording stores.

**Files:**

- Modify: `backend/internal/recordings/store.go`
- Modify: `backend/internal/recordings/postgres_store.go`
- Modify if needed: `backend/internal/recordings/store_test.go`
- Modify if needed: `backend/internal/recordings/postgres_store_test.go`

**Behavior:**

- Validate non-empty recording ID.
- Validate requested status is non-empty and one of the domain statuses that this milestone writes: `processing`, `completed`, `failed`.
- Update `UpdatedAt` to a fresh timestamp.
- Preserve all existing metadata fields, especially `AudioObjectKey`, `AudioContentType`, and `AudioSizeBytes`.
- Return an error for missing recording IDs.

**Postgres SQL shape:**

```sql
UPDATE recordings
SET status = $2, updated_at = $3
WHERE id = $1
RETURNING id, title, status, workflow_type, language,
          audio_object_key, audio_content_type, audio_size_bytes,
          created_at, updated_at
```

**Verification:**

```bash
cd backend && go test ./internal/recordings -v
cd backend && go test ./...
```

**Suggested commit:**

```txt
feat: add recording status update persistence
```

---

## Task F4: Add store-backed processing activity RED tests

**Objective:** Define how activities update recording status through an injected store seam.

**Files:**

- Modify: `backend/internal/activities/recording_processing_test.go`

**Design:**

Introduce an activity struct instead of relying only on package-level function activities:

```go
type RecordingStore interface {
    Get(id string) (domain.Recording, bool)
    UpdateStatus(recordings.UpdateRecordingStatusInput) (domain.Recording, error)
}

type RecordingProcessingActivities struct {
    store RecordingStore
}

func NewRecordingProcessingActivities(store RecordingStore) *RecordingProcessingActivities
```

Method activities:

```go
func (a *RecordingProcessingActivities) ValidateRecording(ctx context.Context, input RecordingProcessingInput) error
func (a *RecordingProcessingActivities) MarkRecordingProcessing(ctx context.Context, recordingID string) error
func (a *RecordingProcessingActivities) CompleteRecordingProcessing(ctx context.Context, recordingID string) (RecordingProcessingResult, error)
func (a *RecordingProcessingActivities) FailRecordingProcessing(ctx context.Context, recordingID string) error
```

**RED tests:**

- `ValidateRecording` fails if the recording does not exist in the store.
- `MarkRecordingProcessing` calls `UpdateStatus(...processing...)`.
- `CompleteRecordingProcessing` calls `UpdateStatus(...completed...)` and returns completed result.
- `FailRecordingProcessing` calls `UpdateStatus(...failed...)`.
- Missing recording ID errors remain covered.

**RED verification:**

```bash
cd backend && go test ./internal/activities -run 'RecordingProcessing' -v
```

Expected: FAIL because the new struct/method activities do not exist yet.

**Suggested commit after GREEN in F5:** no commit in this RED-only task unless the user explicitly wants RED tests committed separately.

---

## Task F5: Implement store-backed processing activities

**Objective:** Convert placeholder activities into store-backed activity methods while keeping workflow activity names stable enough for tests.

**Files:**

- Modify: `backend/internal/activities/recording_processing.go`
- Modify if needed: `backend/internal/activities/recording_processing_test.go`

**Behavior:**

- `ValidateRecording` still validates recording ID and workflow type, and now also checks that `store.Get(input.RecordingID)` exists.
- `MarkRecordingProcessing` updates the recording to `processing`.
- `CompleteRecordingProcessing` updates the recording to `completed` and returns `RecordingProcessingResult{RecordingID, Status: completed}`.
- `FailRecordingProcessing` best-effort updates the recording to `failed`.
- Constructor should error or create a struct that returns clear errors when store is nil; avoid panics.

**Compatibility note:**

The existing package-level function names may remain for unit tests or pure workflow tests, but the worker should register the store-backed methods in a later task. If keeping wrappers, make their limitations explicit in comments.

**Verification:**

```bash
cd backend && go test ./internal/activities -v
cd backend && go test ./...
```

**Suggested commit:**

```txt
feat: persist recording status from activities
```

---

## Task F6: Update workflow failure handling and tests

**Objective:** Ensure the workflow still completes the happy path and best-effort marks recordings failed when a processing step errors.

**Files:**

- Modify: `backend/internal/workflows/recording_processing.go`
- Modify: `backend/internal/workflows/recording_processing_test.go`

**Behavior:**

- Happy path remains: validate → mark processing → complete.
- If `MarkRecordingProcessing` fails, return that error. There may be no need to mark failed because the transition to processing did not succeed.
- If `CompleteRecordingProcessing` fails, call `FailRecordingProcessing` best-effort, then return the original complete error.
- Do not hide the original workflow error if failure marking also fails.

**Testing strategy:**

Use Temporal testsuite mocks to assert activity order for:

1. Happy path: validate, mark processing, complete.
2. Complete failure path: validate, mark processing, complete fails, fail activity is scheduled, workflow returns original error.

**Verification:**

```bash
cd backend && go test ./internal/workflows -v
cd backend && go test ./...
```

**Suggested commit:**

```txt
feat: mark recordings failed when workflow completion fails
```

---

## Task F7: Wire worker activities to Soniq Postgres

**Objective:** Make the production worker update Soniq application recordings instead of running stateless placeholder activities.

**Files:**

- Modify: `backend/cmd/worker/main.go`
- Modify: `backend/cmd/worker/main_test.go`

**Architecture:**

- Keep Temporal's internal database separate from Soniq Postgres.
- Worker should read `POSTGRES_DSN` from existing config and open a Soniq Postgres pool, similar to `cmd/api`.
- Worker registers:
  - `workflows.RecordingProcessingWorkflow`
  - store-backed activity methods from `activities.NewRecordingProcessingActivities(recordingStore)`

**Testing strategy:**

- Update registration tests so they verify workflow registration and activity registration with method activities.
- Add a test that worker setup closes the Postgres pool / store client on cleanup if the command structure supports it.
- Keep unit tests hermetic; do not require a running Postgres or Temporal server.

**Verification:**

```bash
cd backend && go test ./cmd/worker -v
cd backend && go test ./...
```

**Suggested commit:**

```txt
feat: wire worker activities to recording store
```

---

## Task F8: Extend smoke verification for completed DB status

**Objective:** Prove the full local stack updates recording status to `completed` after the Temporal workflow finishes.

**Files:**

- Modify: `scripts/smoke-postgres-temporal.sh`

**Behavior:**

After the smoke script sees Temporal workflow `COMPLETED`, verify both:

```bash
curl -fsS "$API_URL/recordings/$recording_id/status"
```

returns:

```json
{"id":"rec_...","status":"completed"}
```

and the Soniq Postgres row has `status = 'completed'`.

**Verification:**

```bash
bash -n scripts/smoke-postgres-temporal.sh
cd backend && go test ./...
API_URL=http://localhost:18080 API_ADDRESS=:18080 make smoke-postgres-temporal
```

Use the alternate port when `localhost:8080` is already occupied.

**Suggested commit:**

```txt
test: verify completed recording status in smoke workflow
```

---

## Task F9: Document workflow status persistence

**Objective:** Update docs so local development instructions match the new DB-backed workflow status behavior.

**Files:**

- Modify: `docs/development.md`
- Modify: `docs/workflows.md`

**Documentation updates:**

- Explain the status transition:

```txt
uploaded -> processing -> completed / failed
```

- Explain that worker activities update Soniq Postgres through the recording store.
- Explain that `GET /recordings/{id}/status` should eventually return `completed` after workflow completion in local smoke.
- Keep future ffmpeg/ASR/LLM boundaries explicit.

**Verification:**

```bash
git diff --check
cd backend && go test ./...
```

**Suggested commit:**

```txt
docs: document recording workflow status persistence
```
