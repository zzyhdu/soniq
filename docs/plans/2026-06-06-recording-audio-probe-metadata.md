# Recording Audio Probe Metadata Implementation Plan

> **For Hermes:** Execute this plan task-by-task. Keep repository docs tool-agnostic and use small commits. Ask before every commit.

**Goal:** Make uploaded recordings run through an ffprobe-backed audio probe step in `RecordingProcessingWorkflow` and persist useful audio metadata in Soniq Postgres before marking the recording completed.

**Architecture:** Keep the existing local-first object storage and Temporal worker boundary. Add a narrow `recording_audio_probes` persistence model/table for original-audio probe results, introduce a testable ffprobe runner seam, then insert a `ProbeRecordingAudioActivity` between `MarkRecordingProcessingActivity` and `CompleteRecordingProcessingActivity`. Unit tests should use fakes; the smoke test should use real local storage, real Postgres, real Temporal, and real `ffprobe` when available.

**Tech Stack:** Go, Temporal Go SDK, Postgres migrations, pgx-backed stores, local filesystem object storage, `ffprobe` from FFmpeg for real smoke verification.

---

## Current-state findings

- `backend/internal/workflows/recording_processing.go` currently schedules:
  1. `ValidateRecordingActivity`
  2. `MarkRecordingProcessingActivity`
  3. `CompleteRecordingProcessingActivity`
- F milestone wired worker activities to Soniq Postgres-backed methods while preserving stable Temporal activity names.
- `backend/internal/activities/recording_processing.go` has a `RecordingProcessingActivities` struct with a `RecordingStore` seam for `Get` and `UpdateStatus`.
- `domain.Recording` already carries original upload metadata:
  - `AudioObjectKey`
  - `AudioContentType`
  - `AudioSizeBytes`
- `backend/internal/storage.LocalStore` writes objects to a local root, but the public `ObjectStore` interface currently only exposes `PutObject`; there is no path resolver or reader seam yet.
- Current migrations are:
  - `0001_create_recordings.*.sql`
  - `0002_add_recording_audio_metadata.*.sql`
- `scripts/smoke-postgres-temporal.sh` already uploads a real audio object, waits for Temporal `COMPLETED`, and verifies `recordings.status=completed`.

## Product decision for this milestone

G is an **audio probe** milestone, not a normalization or transcription milestone.

Do now:

- Add a narrow Postgres table for original-audio probe results.
- Probe the original uploaded audio with ffprobe.
- Persist duration/format/codec/sample-rate/channel metadata plus raw JSON.
- Make the workflow fail and best-effort mark `failed` if probing fails.
- Extend smoke to prove probe metadata exists after workflow completion.

Do not do now:

- No ffmpeg normalize/transcode output.
- No chunking.
- No ASR provider calls.
- No LLM summary generation.
- No generic `recording_artifacts` system yet.
- No S3-compatible object download/presigned-input support yet.

## Target behavior

After a successful audio upload:

```txt
POST /recordings/upload
  -> original audio is stored in local object storage
  -> recordings.status = uploaded
  -> Temporal workflow starts
  -> MarkRecordingProcessingActivity updates status = processing
  -> ProbeRecordingAudioActivity resolves the original local object path
  -> ffprobe reads the original audio file
  -> recording_audio_probes stores probe metadata
  -> CompleteRecordingProcessingActivity updates status = completed
  -> smoke verifies Temporal COMPLETED, recordings.status=completed, and probe metadata exists
```

If ffprobe cannot read the uploaded object, the workflow should return the probe error and use the existing best-effort failed-state path so `recordings.status` can become `failed`.

## Proposed schema

Create `backend/migrations/0003_create_recording_audio_probes.up.sql`:

```sql
CREATE TABLE recording_audio_probes (
  recording_id TEXT PRIMARY KEY REFERENCES recordings(id) ON DELETE CASCADE,
  duration_seconds DOUBLE PRECISION,
  format_name TEXT NOT NULL,
  codec_name TEXT NOT NULL DEFAULT '',
  sample_rate INTEGER NOT NULL DEFAULT 0,
  channels INTEGER NOT NULL DEFAULT 0,
  bit_rate INTEGER NOT NULL DEFAULT 0,
  raw_probe_json JSONB NOT NULL,
  probed_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
```

Create matching down migration:

```sql
DROP TABLE IF EXISTS recording_audio_probes;
```

Rationale:

- Keep probe data out of the `recordings` main table.
- Use one row per recording for this first original-audio probe milestone.
- Keep `raw_probe_json` so early field choices do not lose data.
- Defer generic artifact/version tables until normalized audio or multiple derived artifacts exist.

---

## Task G1: Add audio probe metadata plan and work items

**Objective:** Save this plan and create parked work items for the audio probe milestone without modifying implementation files.

**Files:**

- Create: `docs/plans/2026-06-06-recording-audio-probe-metadata.md`

**Plan-only guardrail:**

- Capture `git status --short` before and after.
- Only this plan file and task-board metadata may change.
- Do not edit Go source, tests, migrations, scripts, or runtime docs in this task.

**Verification:**

```bash
git status --short
```

Expected: only the plan file is modified/created in git.

**Suggested commit:**

```txt
docs: add recording audio probe metadata plan
```

---

## Task G2: Add audio probe domain and store RED tests

**Objective:** Define the persistence contract for original-audio probe results before implementation.

**Files:**

- Modify: `backend/internal/recordings/store_test.go`
- Modify: `backend/internal/recordings/postgres_store_test.go`

**TDD expectation:**

Add failing tests for:

1. Memory store can upsert and get a `RecordingAudioProbe` by recording ID.
2. Upsert replaces probe fields and advances `UpdatedAt` while preserving `CreatedAt`.
3. Get returns `ok=false` for missing recording IDs.
4. Postgres store issues an upsert into `recording_audio_probes` and scans all fields.
5. Postgres get handles `pgx.ErrNoRows` as `ok=false`.

**Expected RED failure:**

```txt
undefined: RecordingAudioProbe
store.UpsertAudioProbe undefined
store.GetAudioProbe undefined
```

**Verification command:**

```bash
cd backend && go test ./internal/recordings -run 'AudioProbe' -v
```

**Suggested commit:**

Commit together with G3 after GREEN unless the user explicitly asks for separate RED-only commits.

---

## Task G3: Implement audio probe persistence and migration

**Objective:** Add the storage model, in-memory implementation, Postgres implementation, and migration for probe metadata.

**Files:**

- Create: `backend/migrations/0003_create_recording_audio_probes.up.sql`
- Create: `backend/migrations/0003_create_recording_audio_probes.down.sql`
- Modify: `backend/internal/recordings/store.go`
- Modify: `backend/internal/recordings/postgres_store.go`
- Modify: `backend/internal/recordings/store_test.go`
- Modify: `backend/internal/recordings/postgres_store_test.go`

**Implementation notes:**

- Add `RecordingAudioProbe` with fields matching the migration.
- Add store methods:
  - `UpsertAudioProbe(input UpsertAudioProbeInput) (RecordingAudioProbe, error)`
  - `GetAudioProbe(recordingID string) (RecordingAudioProbe, bool)`
- Postgres upsert should use `INSERT ... ON CONFLICT (recording_id) DO UPDATE`.
- Keep raw JSON as `[]byte` or `json.RawMessage` in Go; use whichever keeps tests clean and SQL args explicit.
- Use `time.Now().UTC()` consistently with existing store timestamp patterns.

**Verification commands:**

```bash
cd backend && go test ./internal/recordings -v
cd backend && go test ./...
git diff --check
git status --short
```

**Suggested commit:**

```txt
feat: add recording audio probe persistence
```

---

## Task G4: Add local object path resolver tests

**Objective:** Define a safe way for worker activities to resolve local object keys to filesystem paths without hand-building paths inside activities.

**Files:**

- Modify: `backend/internal/storage/local_test.go`
- Modify: `backend/internal/storage/local.go`

**TDD expectation:**

Add failing tests for a method like:

```go
func (s *LocalStore) LocalPathForObject(key string) (string, error)
```

Test cases:

1. Valid object key resolves under the local root.
2. Backslash input is normalized consistently with `PutObject`.
3. Empty key is rejected.
4. Absolute path is rejected.
5. `../` traversal is rejected.

**Expected RED failure:**

```txt
store.LocalPathForObject undefined
```

**Verification command:**

```bash
cd backend && go test ./internal/storage -run 'LocalPath' -v
```

**Suggested commit:**

Commit together with G5 after GREEN unless the user explicitly asks for separate RED-only commits.

---

## Task G5: Implement local object path resolver

**Objective:** Let local worker activities obtain a safe local filesystem path for an existing object key.

**Files:**

- Modify: `backend/internal/storage/local.go`
- Modify: `backend/internal/storage/local_test.go`

**Implementation notes:**

- Reuse existing `cleanObjectKey` so `PutObject` and path resolution enforce the same key rules.
- Do not check file existence in the resolver; let ffprobe return the file-read error. This keeps the resolver focused on key safety.
- Return `filepath.Join(root, filepath.FromSlash(cleanedKey))`.
- If root is empty, preserve current behavior and resolve relative to `.`.

**Verification commands:**

```bash
cd backend && go test ./internal/storage -v
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
feat: add local object path resolver
```

---

## Task G6: Add ffprobe runner and probe activity RED tests

**Objective:** Define how recording processing activities probe original audio without making unit tests depend on real ffprobe.

**Files:**

- Create or modify: `backend/internal/activities/audio_probe_test.go`
- Modify: `backend/internal/activities/recording_processing.go`

**TDD expectation:**

Add failing tests for a store-backed activity method like:

```go
func (a *RecordingProcessingActivities) ProbeRecordingAudio(ctx context.Context, recordingID string) error
```

Use fakes for:

- recording store with `Get`, `UpdateStatus`, `UpsertAudioProbe`
- local object path resolver
- ffprobe runner

Test cases:

1. Missing recording ID returns an error.
2. Missing store returns an error.
3. Missing recording returns an error.
4. Recording with empty `AudioObjectKey` returns an error.
5. Resolver receives the recording's `AudioObjectKey`.
6. ffprobe runner receives the resolved local path.
7. Parsed metadata is persisted through the store.
8. ffprobe failure returns an error and does not write probe metadata.

**Expected RED failure:**

```txt
ProbeRecordingAudio undefined
AudioProbeRunner undefined
```

**Verification command:**

```bash
cd backend && go test ./internal/activities -run 'ProbeRecordingAudio|FFProbe' -v
```

**Suggested commit:**

Commit together with G7 after GREEN unless the user explicitly asks for separate RED-only commits.

---

## Task G7: Implement ffprobe runner and probe activity

**Objective:** Add a production ffprobe-backed runner and a store-backed Temporal activity method that persists probe metadata.

**Files:**

- Create: `backend/internal/activities/audio_probe.go`
- Modify: `backend/internal/activities/recording_processing.go`
- Modify: `backend/internal/activities/recording_processing_test.go` or `audio_probe_test.go`

**Implementation notes:**

- Add a focused runner seam, for example:

```go
type AudioProbeRunner interface {
    Probe(ctx context.Context, path string) (AudioProbeResult, error)
}
```

- Production runner should execute:

```bash
ffprobe -v error -print_format json -show_format -show_streams <path>
```

- Parse the first audio stream for codec/sample-rate/channels.
- Parse format duration/format name/bit rate when present.
- Preserve raw ffprobe JSON.
- Use context-aware `exec.CommandContext`.
- Keep unit tests fake-runner based.

**Verification commands:**

```bash
cd backend && go test ./internal/activities -v
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
feat: add recording audio probe activity
```

---

## Task G8: Wire workflow and worker to probe activity

**Objective:** Insert the probe step into the workflow and register the store-backed method under a stable Temporal activity name.

**Files:**

- Modify: `backend/internal/workflows/recording_processing.go`
- Modify: `backend/internal/workflows/recording_processing_test.go`
- Modify: `backend/cmd/worker/main.go`
- Modify: `backend/cmd/worker/main_test.go`

**TDD expectation:**

Update workflow tests to expect:

```txt
ValidateRecordingActivity
MarkRecordingProcessingActivity
ProbeRecordingAudioActivity
CompleteRecordingProcessingActivity
```

Add failure-path test:

```txt
ProbeRecordingAudioActivity fails
  -> FailRecordingProcessingActivity is called
  -> workflow returns original probe error
```

Update worker registration tests to require `ProbeRecordingAudioActivity`.

**Implementation notes:**

- Preserve stable activity name with `RegisterActivityWithOptions`.
- If the package-level compatibility function is still needed for workflow tests, add a stateless `ProbeRecordingAudioActivity` compatibility function temporarily and document that production worker registers the method implementation under the same name.
- Existing best-effort failed-state handling should cover probe failure, not only completion failure.

**Verification commands:**

```bash
cd backend && go test ./internal/workflows -v
cd backend && go test ./cmd/worker -v
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
feat: probe recording audio in workflow
```

---

## Task G9: Extend smoke verification for audio probe metadata

**Objective:** Prove the full local path persists probe metadata after Temporal workflow completion.

**Files:**

- Modify: `scripts/smoke-postgres-temporal.sh`
- Possibly modify: `Makefile` only if a separate target is needed; prefer reusing existing target.

**Implementation notes:**

- Check `command -v ffprobe` near the smoke prerequisite checks.
- If `ffprobe` is missing, fail with a clear message explaining how to install FFmpeg locally.
- After `recordings.status=completed`, query `recording_audio_probes` by recording ID.
- Assert at least:
  - row exists
  - `format_name` is non-empty
  - `raw_probe_json` is non-empty
  - one of `duration_seconds`, `codec_name`, `sample_rate`, or `channels` proves real parsing occurred
- Keep runtime artifacts under the existing temp/local storage cleanup behavior and report log path.

**Verification commands:**

```bash
bash -n scripts/smoke-postgres-temporal.sh
API_URL=http://localhost:18080 API_ADDRESS=:18080 make smoke-postgres-temporal
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
test: verify audio probe metadata in smoke workflow
```

---

## Task G10: Document audio probe workflow

**Objective:** Update docs to describe the new ffprobe metadata boundary and current non-goals.

**Files:**

- Modify: `docs/development.md`
- Modify: `docs/workflows.md`
- Modify: `docs/roadmap.md`

**Documentation points:**

- The workflow now probes uploaded original audio before marking the recording completed.
- Local smoke requires `ffprobe` from FFmpeg.
- Probe metadata is stored in `recording_audio_probes`.
- Current probe support is local-object-storage-first; S3-compatible storage will need download-to-temp or another input strategy later.
- Normalize/transcode, ASR, LLM, and generic artifact tracking remain future milestones.

**Verification commands:**

```bash
grep -R "ffprobe\|recording_audio_probes\|audio probe" docs/*.md
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
docs: document recording audio probe workflow
```

---

## Milestone acceptance criteria

G is complete when:

- `recording_audio_probes` exists in migrations with a matching down migration.
- Memory and Postgres stores can upsert/get probe metadata.
- Local object storage exposes a safe path resolver for local-only worker probing.
- `RecordingProcessingWorkflow` schedules a probe activity before completion.
- Worker registers a store-backed probe activity under a stable Temporal activity name.
- Unit tests pass for recordings, storage, activities, workflows, worker, and full backend.
- Smoke proves:
  - upload succeeds;
  - Temporal reaches `COMPLETED`;
  - `recordings.status=completed`;
  - `recording_audio_probes` has metadata for the uploaded recording.
- Docs clearly state that normalize/ASR/LLM/artifact versioning are not implemented yet.

## Risks and rollback

- **ffprobe availability:** Keep real ffprobe usage in smoke/production path; unit tests use fake runners. If missing locally, smoke should fail with a clear install message.
- **Temporal activity naming:** Preserve existing stable names and add one new stable name, `ProbeRecordingAudioActivity`, so workflow history remains readable.
- **Local-only storage coupling:** Keep the resolver seam explicit and document that S3-compatible storage needs a later download-to-temp strategy.
- **Schema overreach:** Do not introduce generic artifact tables until normalized audio or multiple derived outputs require them.
- **Workflow failure behavior:** Probe failure should use the F milestone best-effort failed-state path and return the original error.
