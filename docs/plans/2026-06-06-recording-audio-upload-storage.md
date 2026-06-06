# Recording Audio Upload and Storage Abstraction Implementation Plan

> **For Hermes:** Execute this plan task-by-task. Keep repository docs tool-agnostic and use small commits. Ask before every commit.

**Goal:** Let Soniq accept real audio files for recordings, persist the audio through a storage abstraction, and keep recording metadata linked to the stored object.

**Architecture:** Add a storage seam under the backend so API handlers do not know whether bytes are written to local disk or an S3-compatible object store. Keep the existing JSON `POST /recordings` metadata endpoint working, and add a new multipart upload path for audio-backed recordings. Recordings remain persisted in Soniq Postgres; storage metadata such as object key, content type, and byte size is added to the recordings table in a new migration.

**Tech Stack:** Go standard library multipart HTTP handling, Postgres migrations, local filesystem storage provider for the first implementation slice, optional S3-compatible/MinIO provider as a later slice. Latest checked MinIO image tag visible from Docker Hub during planning: `minio/minio:RELEASE.2025-09-07T16-13-09Z`.

---

## Current-state findings

- Existing API endpoints:
  - `POST /recordings` accepts JSON metadata only and enqueues `RecordingProcessingWorkflow` after successful creation.
  - `GET /recordings/{id}` returns recording metadata.
  - `GET /recordings/{id}/status` returns status only.
- Existing domain model `backend/internal/domain/recording.go` has no audio/storage fields yet.
- Existing persistence `backend/internal/recordings/postgres_store.go` stores only `id`, `title`, `status`, `workflow_type`, `language`, `created_at`, and `updated_at`.
- Existing migration `backend/migrations/0001_create_recordings.up.sql` creates only the current metadata columns.
- Existing `.env.example` already contains future S3-compatible variables, but runtime config currently exposes only `StorageProvider` and not S3/local storage details.
- The production API already has a `RecordingProcessor` enqueue seam. Audio upload should continue to enqueue the same Temporal workflow after metadata + audio storage succeed.

## Non-goals for this milestone

- No ffmpeg, transcription, summarization, or LLM provider calls.
- No authenticated uploads or user ownership yet.
- No frontend UI yet.
- No large-file resumable upload protocol yet.
- No direct-to-S3 browser upload/presigned URL flow yet.
- No production object storage deployment design beyond local/dev configuration.

## API shape for this milestone

Keep the existing JSON endpoint for metadata-only tests and compatibility:

```txt
POST /recordings
Content-Type: application/json
```

Add a dedicated multipart endpoint for audio-backed recordings:

```txt
POST /recordings/upload
Content-Type: multipart/form-data
```

Required multipart fields:

| Field | Type | Purpose |
|---|---|---|
| `audio` | file | Audio file contents. |
| `title` | text | Recording title. |
| `workflow_type` | text | Existing workflow selector: `memo`, `meeting`, `lecture`, `interview`. |
| `language` | text | Language code/string, same as existing JSON endpoint. |

Successful response remains a recording JSON object, now including audio storage metadata when present:

```json
{
  "id": "rec_...",
  "title": "Weekly sync",
  "status": "uploaded",
  "workflow_type": "meeting",
  "language": "en",
  "audio_object_key": "recordings/rec_.../original.wav",
  "audio_content_type": "audio/wav",
  "audio_size_bytes": 12345,
  "created_at": "...",
  "updated_at": "..."
}
```

## Storage abstraction shape

Add package:

```txt
backend/internal/storage
```

Initial interface:

```go
type PutObjectInput struct {
    Key         string
    Body        io.Reader
    ContentType string
}

type PutObjectResult struct {
    Key       string
    SizeBytes int64
}

type ObjectStore interface {
    PutObject(ctx context.Context, input PutObjectInput) (PutObjectResult, error)
}
```

For the first implementation, add a local filesystem provider:

```txt
backend/internal/storage/local.go
```

It writes objects under a configured root, for example:

```txt
var/uploads/recordings/<recording_id>/original.<ext>
```

The API only depends on `ObjectStore`, not local filesystem details.

---

## Task E1: Add audio storage plan and work items

**Objective:** Save this plan and create parked work items for the upload/storage milestone.

**Files:**

- Create: `docs/plans/2026-06-06-recording-audio-upload-storage.md`

**Verification:**

```bash
git status --short
```

Only the plan file and task-board metadata should change.

**Suggested commit:**

```txt
docs: add recording audio upload storage plan
```

---

## Task E2: Add recording audio metadata to domain and migration

**Objective:** Make the recording model and Postgres schema able to describe the stored original audio object.

**Files:**

- Modify: `backend/internal/domain/recording.go`
- Modify: `backend/internal/domain/recording_test.go`
- Create: `backend/migrations/0002_add_recording_audio_metadata.up.sql`
- Create: `backend/migrations/0002_add_recording_audio_metadata.down.sql`
- Modify: `backend/internal/recordings/postgres_store.go`
- Modify: `backend/internal/recordings/postgres_store_test.go`
- Modify: `backend/internal/recordings/store.go`
- Modify: `backend/internal/recordings/store_test.go`

**Fields to add:**

```go
AudioObjectKey   string `json:"audio_object_key,omitempty"`
AudioContentType string `json:"audio_content_type,omitempty"`
AudioSizeBytes   int64  `json:"audio_size_bytes,omitempty"`
```

**Migration up:**

```sql
ALTER TABLE recordings
  ADD COLUMN audio_object_key TEXT NOT NULL DEFAULT '',
  ADD COLUMN audio_content_type TEXT NOT NULL DEFAULT '',
  ADD COLUMN audio_size_bytes BIGINT NOT NULL DEFAULT 0;
```

**Migration down:**

```sql
ALTER TABLE recordings
  DROP COLUMN audio_size_bytes,
  DROP COLUMN audio_content_type,
  DROP COLUMN audio_object_key;
```

**TDD expectation:**

- RED: store tests fail because fields are not persisted/scanned yet.
- GREEN: both memory and Postgres stores preserve audio metadata when input includes it.

**Verification:**

```bash
cd backend && go test ./internal/domain ./internal/recordings -v
cd backend && go test ./...
```

**Suggested commit:**

```txt
feat: add recording audio metadata fields
```

---

## Task E3: Add storage abstraction with local filesystem provider

**Objective:** Introduce an `ObjectStore` interface and a local filesystem implementation without wiring it into HTTP yet.

**Files:**

- Create: `backend/internal/storage/store.go`
- Create: `backend/internal/storage/local.go`
- Create: `backend/internal/storage/local_test.go`

**Behavior:**

- `LocalStore.PutObject` writes bytes under a configured root directory.
- It creates parent directories safely.
- It returns `Key` and exact `SizeBytes` written.
- It rejects empty keys and path traversal keys such as `../secret`.
- It should not log or expose file contents.

**Verification:**

```bash
cd backend && go test ./internal/storage -v
cd backend && go test ./...
```

**Suggested commit:**

```txt
feat: add local object storage provider
```

---

## Task E4: Add runtime config for local storage

**Objective:** Make storage provider configuration explicit enough for the API command to construct the local object store.

**Files:**

- Modify: `backend/internal/config/config.go`
- Modify: `backend/internal/config/config_test.go`
- Modify: `.env.example`

**Config fields:**

```go
StorageProvider string
LocalStoragePath string
```

**Environment variables:**

```txt
STORAGE_PROVIDER=local
LOCAL_STORAGE_PATH=var/uploads
```

**Important:** Keep existing S3-compatible placeholders in `.env.example` if useful, but mark them as future until an S3 provider is implemented. Prefer `local` as the current default to keep local development runnable without MinIO.

**Verification:**

```bash
cd backend && go test ./internal/config -v
cd backend && go test ./...
```

**Suggested commit:**

```txt
feat: add local storage configuration
```

---

## Task E5: Add upload endpoint RED tests

**Objective:** Specify the multipart upload API before implementation.

**Files:**

- Modify: `backend/internal/api/recordings_test.go`

**Tests to add:**

1. `POST /recordings/upload` with multipart `audio`, `title`, `workflow_type`, and `language` returns `201 Created`.
2. The response includes `audio_object_key`, `audio_content_type`, and `audio_size_bytes`.
3. The handler stores the uploaded bytes through an injected fake `ObjectStore`.
4. The handler creates a recording through the injected `RecordingStore`.
5. The handler enqueues the recording processor after storage + metadata persistence succeed.
6. Missing `audio` returns `400 Bad Request` and does not enqueue.
7. Storage failure returns `500 Internal Server Error` and does not enqueue.

**Expected RED:**

```txt
POST /recordings/upload returns 404 or compile fails because storage injection does not exist yet
```

**Verification:**

```bash
cd backend && go test ./internal/api -run Upload -v
```

**Suggested commit:**

```txt
test: add recording upload endpoint red tests
```

---

## Task E6: Implement multipart upload endpoint

**Objective:** Implement the smallest HTTP path that stores uploaded audio, persists metadata, and enqueues processing.

**Files:**

- Modify: `backend/internal/api/router.go`
- Modify: `backend/internal/api/recordings_test.go`
- Modify as needed: `backend/internal/recordings/store.go`
- Modify as needed: `backend/internal/recordings/postgres_store.go`

**Implementation notes:**

- Add an `ObjectStore` dependency to router construction.
- Keep existing constructors compatible by defaulting to a no-op or test-only store only where safe.
- Generate object key after recording ID is known. If the store must know audio metadata at creation time, use a two-step store API or create an explicit `CreateWithAudio` input; choose the smallest clear design.
- Do not enqueue Temporal workflow until both audio storage and metadata persistence have succeeded.
- Enforce a conservative max request size for this milestone, for example `100 MiB`, and document it.
- Preserve existing JSON `POST /recordings` behavior.

**Verification:**

```bash
cd backend && go test ./internal/api -v
cd backend && go test ./...
```

**Suggested commit:**

```txt
feat: add recording audio upload endpoint
```

---

## Task E7: Wire production API to local object storage

**Objective:** Make `make api` use the local filesystem storage provider for multipart uploads.

**Files:**

- Modify: `backend/cmd/api/main.go`
- Modify: `backend/cmd/api/main_test.go`
- Possibly modify: `backend/internal/config/config.go`

**Behavior:**

- `buildHandler` constructs `storage.NewLocalStore(cfg.LocalStoragePath)` when `STORAGE_PROVIDER=local`.
- Startup fails clearly for unsupported storage providers.
- Existing tests continue to use fakes and do not touch real filesystem unless explicitly scoped to `t.TempDir()`.

**Verification:**

```bash
cd backend && go test ./cmd/api -v
cd backend && go test ./...
```

**Suggested commit:**

```txt
feat: wire api to local object storage
```

---

## Task E8: Extend smoke verification for multipart audio upload

**Objective:** Prove the production path accepts an audio file, writes it to local storage, persists metadata in Postgres, survives API restart, and starts/completes the Temporal workflow.

**Files:**

- Modify: `scripts/smoke-postgres-temporal.sh`
- Possibly modify/create: `scripts/smoke-recording-upload.sh`
- Modify: `Makefile` only if a separate target is useful

**Smoke behavior:**

- Create a tiny deterministic local sample file in a temporary directory. It can be text bytes with `Content-Type: audio/wav` for HTTP/storage path verification; real audio decoding is out of scope.
- Call:

```bash
curl -f -sS -X POST http://localhost:8080/recordings/upload \
  -F "title=Weekly sync" \
  -F "workflow_type=meeting" \
  -F "language=en" \
  -F "audio=@sample.wav;type=audio/wav"
```

- Assert response includes non-empty `audio_object_key` and positive `audio_size_bytes`.
- Assert the local storage file exists under `LOCAL_STORAGE_PATH`.
- Restart API and verify `GET /recordings/<id>` still returns the audio metadata.
- Assert Temporal workflow reaches `COMPLETED`.

**Verification:**

```bash
bash -n scripts/smoke-postgres-temporal.sh
make smoke-postgres-temporal
cd backend && go test ./...
```

**Suggested commit:**

```txt
chore: smoke test recording audio upload
```

---

## Task E9: Document audio upload and storage workflow

**Objective:** Update local development docs so a developer can understand and manually exercise the upload path.

**Files:**

- Modify: `docs/development.md`
- Possibly modify: `docs/workflows.md`

**Docs should explain:**

- `POST /recordings` remains metadata-only.
- `POST /recordings/upload` accepts multipart audio.
- `STORAGE_PROVIDER=local` and `LOCAL_STORAGE_PATH=var/uploads` are current local defaults.
- Uploaded audio is stored locally in this milestone, while S3-compatible object storage remains a future provider unless implemented in a later task.
- Existing Temporal workflow still only runs activity stubs; no transcription/summarization yet.

**Verification:**

```bash
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
docs: document recording audio upload workflow
```

---

## Future option: S3-compatible/MinIO provider

After the local storage path is working, add a separate milestone or extension task for S3-compatible object storage:

- Add MinIO to local Compose using a pinned current tag such as `minio/minio:RELEASE.2025-09-07T16-13-09Z`.
- Add a bucket bootstrap step or one-shot init container.
- Add a Go S3 client dependency together with the first test/code that imports it.
- Support config such as:

```txt
STORAGE_PROVIDER=s3_compatible
S3_ENDPOINT=http://localhost:9000
S3_REGION=us-east-1
S3_BUCKET=soniq
S3_ACCESS_KEY=soniq_minio_user
S3_SECRET_KEY=soniq_minio_password
S3_FORCE_PATH_STYLE=true
```

Keep this separate unless the user explicitly wants S3/MinIO before local filesystem storage.
