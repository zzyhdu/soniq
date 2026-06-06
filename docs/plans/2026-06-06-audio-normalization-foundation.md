# Audio Normalization Foundation Implementation Plan

> **For Hermes:** Execute this plan task-by-task. Keep repository docs tool-agnostic, use small commits, and ask before every commit.

**Goal:** Add a local ffmpeg-backed audio normalization step between original-audio probing and transcription, persist normalized-audio metadata in Soniq Postgres, and make fake transcription read the normalized local artifact instead of the original upload.

**Architecture:** Keep the current local-first object storage and Temporal activity boundary. Introduce a narrow normalized-audio persistence model/table, add a safe derived-object key convention, write an ffmpeg normalization activity with faked unit tests and real smoke verification, then insert `NormalizeRecordingAudioActivity` after `ProbeRecordingAudioActivity` and before `MarkRecordingTranscribingActivity`. Real ASR/LLM providers remain out of scope.

**Tech Stack:** Go, Temporal Go SDK, Postgres migrations, pgx-backed stores, local filesystem object storage, `ffmpeg` and `ffprobe` from FFmpeg for real smoke verification.

---

## Current-state findings

- Current workflow sequence is:

  ```txt
  ValidateRecording
    -> MarkRecordingProcessing
    -> ProbeRecordingAudio
    -> MarkRecordingTranscribing
    -> TranscribeRecordingAudio
    -> MarkRecordingSummarizing
    -> SummarizeRecording
    -> CompleteRecordingProcessing
  ```

- `ProbeRecordingAudio` resolves `recording.AudioObjectKey` through `LocalObjectPathResolver`, runs `FFProbeRunner`, and persists one `recording_audio_probes` row.
- `TranscribeRecordingAudio` currently resolves the original `recording.AudioObjectKey` and passes the original local path to `TranscriptionProvider`.
- Worker currently wires:
  - `storage.NewLocalStore(...)` as object store/path resolver.
  - `activities.FFProbeRunner{Binary: "ffprobe"}` for audio probe.
  - deterministic fake transcription and summary providers.
- Current application persistence tables include:
  - `recordings`
  - `recording_audio_probes`
  - `recording_transcripts`
  - `recording_transcript_segments`
  - `recording_summaries`
- `MemoryStore` has been removed. Do not reintroduce generic in-memory persistence; tests should use focused spies/fakes.
- `scripts/smoke-postgres-temporal.sh` verifies local upload, audio probe, fake transcript, fake summary, Temporal `COMPLETED`, and `recordings.status=completed`.
- J implementation should first check the smoke script's default `POSTGRES_DSN` behavior before running real smoke. If the default was redacted in docs/code, fix it in the relevant implementation task rather than hiding the failure.

## Product decision for this milestone

J is an **audio normalization foundation** milestone.

Do now:

- Add a narrow Postgres table for normalized audio metadata.
- Generate one normalized local WAV artifact per recording using `ffmpeg`.
- Persist normalized object key, content type, size, format target, duration/sample metadata if available, and timestamps.
- Make fake transcription consume the normalized local path when a normalized artifact exists.
- Extend smoke to prove the normalized artifact exists and normalized metadata is persisted.

Do not do now:

- No real ASR provider integration.
- No real LLM provider integration.
- No chunking/splitting.
- No diarization.
- No S3-compatible storage/download/presigned support.
- No generic `recording_artifacts` system yet.
- No UI/API result endpoint.

## Target behavior

After a successful audio upload:

```txt
POST /recordings/upload
  -> original audio stored in local object storage
  -> recordings.status = uploaded
  -> Temporal workflow starts
  -> MarkRecordingProcessingActivity updates status = processing
  -> ProbeRecordingAudioActivity probes original audio
  -> NormalizeRecordingAudioActivity resolves original local path
  -> ffmpeg writes normalized WAV/PCM artifact to local object storage
  -> recording_normalized_audios stores normalized artifact metadata
  -> MarkRecordingTranscribingActivity updates status = transcribing
  -> TranscribeRecordingAudioActivity reads normalized audio path
  -> fake transcript rows are persisted
  -> MarkRecordingSummarizingActivity updates status = summarizing
  -> fake summary row is persisted
  -> CompleteRecordingProcessingActivity updates status = completed
  -> smoke verifies Temporal COMPLETED, DB status, probe row, normalized row/artifact, transcript rows, and summary row
```

If normalization fails, the workflow should return the normalization error and use the existing best-effort failed-state path so `recordings.status` can become `failed`.

## Proposed normalized audio target

For the first local foundation, normalize to:

```txt
container: wav
codec: pcm_s16le
sample_rate: 16000
channels: 1
content_type: audio/wav
```

Rationale:

- Stable, simple target for downstream ASR providers.
- Easy to verify with `ffprobe`.
- Avoids provider-specific preprocessing decisions.
- Keeps current milestone local and deterministic.

Future milestones can add provider-specific targets or keep multiple normalized variants.

## Proposed derived object key convention

For an original object key such as:

```txt
recordings/20260606T150747.170276465Z/weekly.wav
```

write normalized audio to a deterministic derived key:

```txt
recordings/20260606T150747.170276465Z/normalized.wav
```

Rationale:

- Stable retry behavior: the same recording overwrites/reuses the same normalized key.
- Easy local smoke inspection.
- Avoids generic artifact versioning before it is needed.

If collision or versioning becomes important later, move to an explicit artifact table/version key strategy.

## Proposed schema

Create `backend/migrations/0005_create_recording_normalized_audios.up.sql`:

```sql
CREATE TABLE recording_normalized_audios (
  recording_id TEXT PRIMARY KEY REFERENCES recordings(id) ON DELETE CASCADE,
  object_key TEXT NOT NULL,
  content_type TEXT NOT NULL,
  size_bytes BIGINT NOT NULL DEFAULT 0,
  format_name TEXT NOT NULL DEFAULT 'wav',
  codec_name TEXT NOT NULL DEFAULT 'pcm_s16le',
  sample_rate INTEGER NOT NULL DEFAULT 16000,
  channels INTEGER NOT NULL DEFAULT 1,
  duration_seconds DOUBLE PRECISION,
  normalized_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
```

Create matching down migration:

```sql
DROP TABLE IF EXISTS recording_normalized_audios;
```

Rationale:

- Keep normalized metadata out of the `recordings` main table.
- Use one row per recording for this first normalized-audio milestone.
- Keep this narrower than a generic artifact system until multiple derived artifacts exist.

---

## Task J1: Add audio normalization foundation plan

**Objective:** Save this plan without modifying implementation files.

**Files:**

- Create: `docs/plans/2026-06-06-audio-normalization-foundation.md`

**Plan-only guardrail:**

- Capture `git status --short` before and after.
- Only this plan file may change.
- Do not edit Go source, tests, migrations, scripts, or runtime docs in this task.
- Do not run dependency-changing commands.

**Verification:**

```bash
git status --short
```

Expected: only the plan file is untracked/modified.

**Suggested commit:**

```txt
docs: add audio normalization foundation plan
```

---

## Task J2: Add normalized audio persistence RED tests

**Objective:** Define the normalized audio persistence contract before implementation.

**Files:**

- Modify: `backend/internal/recordings/postgres_store_test.go`
- Modify: `backend/internal/recordings/store.go` only if needed for type compile anchors after RED test edits

**TDD expectation:**

Add failing tests for:

1. `PostgresStore.UpsertNormalizedAudio` issues an upsert into `recording_normalized_audios` and scans all fields.
2. Upsert preserves `CreatedAt` and advances `UpdatedAt` through returned Postgres rows.
3. `PostgresStore.GetNormalizedAudio(recordingID)` returns existing normalized metadata.
4. `PostgresStore.GetNormalizedAudio(recordingID)` returns `ok=false` for missing rows.
5. Validation rejects empty recording ID, empty object key, empty content type, non-positive size, zero timestamp, and unsupported target metadata if validation is added.

**Expected RED failure:**

```txt
undefined: RecordingNormalizedAudio
undefined: UpsertNormalizedAudioInput
store.UpsertNormalizedAudio undefined
store.GetNormalizedAudio undefined
```

**Verification command:**

```bash
cd backend && go test ./internal/recordings -run 'NormalizedAudio' -v
```

**Suggested commit:**

Commit together with J3 after GREEN unless the user explicitly asks for separate RED-only commits.

---

## Task J3: Implement normalized audio persistence and migration

**Objective:** Add the normalized audio model, Postgres store support, and migration `0005`.

**Files:**

- Create: `backend/migrations/0005_create_recording_normalized_audios.up.sql`
- Create: `backend/migrations/0005_create_recording_normalized_audios.down.sql`
- Modify: `backend/internal/recordings/store.go`
- Modify: `backend/internal/recordings/postgres_store.go`
- Modify: `backend/internal/recordings/postgres_store_test.go`

**Implementation notes:**

Add types:

```go
type RecordingNormalizedAudio struct {
    RecordingID      string
    ObjectKey        string
    ContentType      string
    SizeBytes        int64
    FormatName       string
    CodecName        string
    SampleRate       int
    Channels         int
    DurationSeconds  float64
    NormalizedAt     time.Time
    CreatedAt        time.Time
    UpdatedAt        time.Time
}

type UpsertNormalizedAudioInput struct {
    RecordingID      string
    ObjectKey        string
    ContentType      string
    SizeBytes        int64
    FormatName       string
    CodecName        string
    SampleRate       int
    Channels         int
    DurationSeconds  float64
    NormalizedAt     time.Time
}
```

Add methods:

```go
func (s *PostgresStore) UpsertNormalizedAudio(input UpsertNormalizedAudioInput) (RecordingNormalizedAudio, error)
func (s *PostgresStore) GetNormalizedAudio(recordingID string) (RecordingNormalizedAudio, bool)
```

Use `INSERT ... ON CONFLICT (recording_id) DO UPDATE`.

**Verification commands:**

```bash
cd backend && go test ./internal/recordings -run 'NormalizedAudio' -v
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
feat: add normalized audio persistence
```

---

## Task J4: Add normalized object key helper RED tests

**Objective:** Define deterministic local derived-object key behavior before implementation.

**Files:**

- Modify: `backend/internal/storage/local_test.go` or create a narrow test near the chosen helper.

**TDD expectation:**

Add failing tests for a helper such as:

```go
func NormalizedAudioObjectKey(originalKey string) (string, error)
```

Test cases:

1. `recordings/<timestamp>/weekly.wav` becomes `recordings/<timestamp>/normalized.wav`.
2. Nested original names still produce a safe sibling/derived key.
3. Empty key is rejected.
4. Absolute path is rejected.
5. `../` traversal is rejected.
6. Backslash input is normalized consistently with local object key handling.

**Verification command:**

```bash
cd backend && go test ./internal/storage -run 'NormalizedAudioObjectKey' -v
```

**Suggested commit:**

Commit together with J5 after GREEN unless the user explicitly asks for separate RED-only commits.

---

## Task J5: Implement normalized object key helper

**Objective:** Provide a deterministic derived key for normalized audio artifacts.

**Files:**

- Modify: `backend/internal/storage/local.go` or create `backend/internal/storage/keys.go`
- Modify: `backend/internal/storage/local_test.go` or matching test file

**Implementation notes:**

- Reuse the same safety semantics as `cleanObjectKey`.
- Do not export `cleanObjectKey` unless necessary.
- Keep derived key local-provider friendly but not tied to absolute filesystem paths.
- Prefer a helper that returns object key only; path resolution should stay with `LocalPathForObject`.

**Verification commands:**

```bash
cd backend && go test ./internal/storage -run 'NormalizedAudioObjectKey|LocalPath' -v
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
feat: add normalized audio object key helper
```

---

## Task J6: Add ffmpeg normalize runner RED tests

**Objective:** Define a testable ffmpeg normalization runner seam before implementation.

**Files:**

- Create or modify: `backend/internal/activities/audio_normalize_test.go`
- Modify: `backend/internal/activities/recording_processing.go` only if compile anchors are needed after RED tests

**TDD expectation:**

Introduce a runner seam such as:

```go
type AudioNormalizeRunner interface {
    Normalize(ctx context.Context, input AudioNormalizeRequest) (AudioNormalizeResult, error)
}

type AudioNormalizeRequest struct {
    InputPath  string
    OutputPath string
}
```

Tests should cover:

1. Missing input path returns an error.
2. Missing output path returns an error.
3. Runner invokes configured ffmpeg binary with target options:
   - `-ac 1`
   - `-ar 16000`
   - `-c:a pcm_s16le`
   - output WAV path
4. ffmpeg failure includes stderr context.
5. Successful runner returns normalized metadata or at least output path/target fields.

Use a fake command runner seam if direct `exec.CommandContext` is hard to unit-test.

**Expected RED failure:**

```txt
undefined: AudioNormalizeRunner
undefined: AudioNormalizeRequest
undefined: FFmpegNormalizeRunner
```

**Verification command:**

```bash
cd backend && go test ./internal/activities -run 'NormalizeRunner|FFmpeg' -v
```

**Suggested commit:**

Commit together with J7 after GREEN unless the user explicitly asks for separate RED-only commits.

---

## Task J7: Implement ffmpeg normalize runner

**Objective:** Add a production runner that writes normalized WAV/PCM output with ffmpeg.

**Files:**

- Modify: `backend/internal/activities/recording_processing.go` or create `backend/internal/activities/audio_normalize.go`
- Modify: `backend/internal/activities/audio_normalize_test.go`

**Implementation notes:**

The production command should be equivalent to:

```bash
ffmpeg -y -hide_banner -loglevel error \
  -i <input_path> \
  -ac 1 -ar 16000 -c:a pcm_s16le \
  <output_path>
```

Consider returning:

```go
type AudioNormalizeResult struct {
    OutputPath      string
    ContentType     string
    FormatName      string
    CodecName       string
    SampleRate      int
    Channels        int
    NormalizedAt    time.Time
}
```

The size can be obtained by the activity after the file is written.

**Verification commands:**

```bash
cd backend && go test ./internal/activities -run 'NormalizeRunner|FFmpeg' -v
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
feat: add ffmpeg audio normalize runner
```

---

## Task J8: Add normalize recording activity RED tests

**Objective:** Define the activity behavior that resolves original audio, writes normalized audio, and persists normalized metadata.

**Files:**

- Modify: `backend/internal/activities/recording_processing_test.go`
- Create or modify: `backend/internal/activities/audio_normalize_test.go`

**TDD expectation:**

Add failing tests for `NormalizeRecordingAudio`:

1. Missing recording ID returns an error.
2. Missing store/path resolver/normalize runner returns an error.
3. Missing recording returns an error.
4. Recording with empty `AudioObjectKey` returns an error.
5. Success path:
   - loads recording;
   - resolves original object path;
   - computes normalized object key;
   - resolves normalized output path;
   - invokes normalize runner;
   - persists normalized metadata through `UpsertNormalizedAudio`.
6. Runner error does not persist normalized metadata.

**Expected RED failure:**

```txt
activities.NormalizeRecordingAudioActivityName undefined
activitySet.NormalizeRecordingAudio undefined
store.UpsertNormalizedAudio undefined in focused test spy
```

**Verification command:**

```bash
cd backend && go test ./internal/activities -run 'NormalizeRecordingAudio' -v
```

**Suggested commit:**

Commit together with J9 after GREEN unless the user explicitly asks for separate RED-only commits.

---

## Task J9: Implement normalize recording activity

**Objective:** Add `NormalizeRecordingAudio` and the persistence seam it needs.

**Files:**

- Modify: `backend/internal/activities/recording_processing.go`
- Modify: `backend/internal/activities/recording_processing_test.go`
- Modify: `backend/internal/activities/audio_normalize_test.go`

**Implementation notes:**

Add constant:

```go
NormalizeRecordingAudioActivityName = "NormalizeRecordingAudioActivity"
```

Add focused seam:

```go
type NormalizedAudioStore interface {
    UpsertNormalizedAudio(input recordings.UpsertNormalizedAudioInput) (recordings.RecordingNormalizedAudio, error)
    GetNormalizedAudio(recordingID string) (recordings.RecordingNormalizedAudio, bool)
}
```

Update `PipelineStore` to include normalized audio support if transcription will choose normalized audio through the same activity set.

Add `NormalizeRecordingAudio(ctx, recordingID string) error`.

Implementation should avoid reintroducing any generic in-memory store.

**Verification commands:**

```bash
cd backend && go test ./internal/activities -run 'NormalizeRecordingAudio|Transcribe|Summarize|ProbeRecordingAudio' -v
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
feat: add normalized audio activity
```

---

## Task J10: Add workflow RED tests for normalization sequence

**Objective:** Make the workflow test require normalization between probe and transcription.

**Files:**

- Modify: `backend/internal/workflows/recording_processing_test.go`

**TDD expectation:**

Update success-path order to:

```txt
ValidateRecording
MarkRecordingProcessing
ProbeRecordingAudio
NormalizeRecordingAudio
MarkRecordingTranscribing
TranscribeRecordingAudio
MarkRecordingSummarizing
SummarizeRecording
CompleteRecordingProcessing
```

Add failure-path test:

```txt
NormalizeRecordingAudio failure -> FailRecordingProcessingActivity -> workflow returns original normalize error
```

**Expected RED failure:**

```txt
unexpected activity call / missing NormalizeRecordingAudioActivityName wiring
```

**Verification command:**

```bash
cd backend && go test ./internal/workflows -run RecordingProcessingWorkflow -v
```

**Suggested commit:**

Commit together with J11 after GREEN unless the user explicitly asks for separate RED-only commits.

---

## Task J11: Wire workflow normalization step

**Objective:** Insert normalization into `RecordingProcessingWorkflow` after probe and before transcribing.

**Files:**

- Modify: `backend/internal/workflows/recording_processing.go`
- Modify: `backend/internal/workflows/recording_processing_test.go`

**Implementation notes:**

Add:

```go
if err := workflow.ExecuteActivity(ctx, activities.NormalizeRecordingAudioActivityName, input.RecordingID).Get(ctx, nil); err != nil {
    _ = workflow.ExecuteActivity(ctx, activities.FailRecordingProcessingActivityName, input.RecordingID).Get(ctx, nil)
    return RecordingProcessingResult{}, err
}
```

Keep workflow deterministic: no filesystem, time, process, HTTP, or randomness in workflow code.

**Verification commands:**

```bash
cd backend && go test ./internal/workflows -run RecordingProcessingWorkflow -v
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
feat: normalize audio in recording workflow
```

---

## Task J12: Make transcription prefer normalized audio

**Objective:** Ensure `TranscribeRecordingAudio` reads normalized audio when available.

**Files:**

- Modify: `backend/internal/activities/transcription_summary_test.go`
- Modify: `backend/internal/activities/recording_processing.go`

**TDD expectation:**

Add tests for:

1. If `GetNormalizedAudio(recordingID)` returns a row, transcription resolves and passes the normalized `ObjectKey` path to the transcription provider.
2. If no normalized row exists, transcription either returns a clear error or falls back to original audio, depending on the chosen policy.

Recommended policy for this milestone:

```txt
After J workflow wiring, transcription should require normalized audio.
```

Rationale: if normalization is now part of the required pipeline, falling back silently could hide normalization failures.

**Implementation notes:**

- Add `GetNormalizedAudio` to the relevant activity seam.
- Keep the fake transcription provider unchanged; only its input `AudioPath` should switch to normalized output.

**Verification commands:**

```bash
cd backend && go test ./internal/activities -run 'TranscribeRecordingAudio' -v
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
feat: transcribe normalized recording audio
```

---

## Task J13: Wire worker registration and dependencies

**Objective:** Register the new normalization activity and inject ffmpeg runner dependencies in the worker.

**Files:**

- Modify: `backend/cmd/worker/main.go`
- Modify: `backend/cmd/worker/main_test.go`

**Implementation notes:**

- Add `FFmpegNormalizeRunner{Binary: "ffmpeg"}` or equivalent.
- Ensure `storage.LocalStore` is used for both original path resolution and normalized output path resolution.
- Register:

```go
activitySet.NormalizeRecordingAudio
```

with:

```go
activities.NormalizeRecordingAudioActivityName
```

- Update worker registration test to expect the new activity name.

**Verification commands:**

```bash
cd backend && go test ./cmd/worker -v
cd backend && go test ./internal/workflows ./internal/activities ./cmd/worker -v
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
feat: register normalized audio activity
```

---

## Task J14: Extend smoke for normalized audio

**Objective:** Prove the full local workflow writes and persists normalized audio.

**Files:**

- Modify: `scripts/smoke-postgres-temporal.sh`

**Implementation notes:**

- Apply migration `0005` if `recording_normalized_audios` is missing.
- After workflow completion, query `recording_normalized_audios` and verify:
  - row exists;
  - object key is non-empty and preferably ends with `/normalized.wav`;
  - content type is `audio/wav`;
  - size is positive;
  - sample rate is `16000`;
  - channels is `1`;
  - codec is `pcm_s16le`;
  - normalized timestamp is set.
- Verify local file exists under:

```txt
<LOCAL_STORAGE_PATH>/<normalized_object_key>
```

- Optionally run `ffprobe` on normalized file in smoke to prove sample rate/channels if the DB values come from activity output rather than a post-normalization probe.

**Important prerequisite:** Before changing smoke assertions, verify the script's default `POSTGRES_DSN` still uses `POSTGRES_PASSWORD` and does not contain a literal redacted placeholder.

**Verification commands:**

```bash
bash -n scripts/smoke-postgres-temporal.sh
cd backend && go test ./cmd/worker ./internal/workflows ./internal/activities ./internal/recordings
API_URL=http://localhost:18080 API_ADDRESS=:18080 bash scripts/smoke-postgres-temporal.sh
git diff --check
```

If stale local API/worker processes own the port or task queue, stop them before smoke and report the exact cleanup.

**Suggested commit:**

```txt
test: verify normalized audio smoke workflow
```

---

## Task J15: Update docs for normalized audio boundary

**Objective:** Document the implemented normalization boundary without claiming real ASR/LLM integration.

**Files:**

- Modify: `docs/development.md`
- Modify: `docs/workflows.md`
- Modify: `docs/roadmap.md`
- Modify: `docs/data-model.md`
- Optionally modify: `docs/providers.md`

**Docs should say:**

- Current local workflow includes original probe + normalized WAV/PCM artifact + fake transcription + fake summary.
- Normalization target is WAV, PCM signed 16-bit, mono, 16 kHz.
- Real ASR/LLM providers remain future scope.
- S3-compatible object storage remains future scope.
- Smoke verifies normalized DB row and local artifact.

**Verification commands:**

```bash
! grep -RIn "normalization.*planned only\|does not.*normalize" docs/development.md docs/workflows.md docs/roadmap.md
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
docs: document normalized audio workflow boundary
```

---

## Task J16: Final cleanup/review pass

**Objective:** Review the J milestone for accidental temporary seams, stale wording, or over-coupling before moving to real providers.

**Files:**

- No required file changes unless review finds issues.

**Checklist:**

- No `MemoryStore` or generic in-memory persistence was reintroduced.
- Workflow code remains deterministic.
- Activity name constants are stable and used by workflow, tests, and worker registration.
- Normalization failure path best-effort marks recording failed.
- Transcription requires or clearly documents normalized audio policy.
- Smoke proves normalized artifact and DB metadata exist.
- No real provider credentials/config were added.
- Local runtime artifacts remain under ignored `var/` paths.

**Verification commands:**

```bash
grep -RIn "NewMemoryStore\|MemoryStore" backend --include='*.go' || true
grep -RIn "time\.Now\|os\.\|exec\.\|http\.\|rand\.\|uuid" backend/internal/workflows --include='*.go' || true
cd backend && go test ./...
git diff --check
git status --short
```

**Suggested commit:**

If changes are needed:

```txt
refactor: clean normalized audio workflow foundation
```

If no changes are needed, report clean state and do not create an empty commit.

---

## Acceptance criteria

J is complete when:

- `recording_normalized_audios` migration exists and is used by local smoke.
- Worker writes a deterministic normalized WAV artifact for uploaded audio.
- Workflow sequence includes normalization after original probe and before transcription.
- `TranscribeRecordingAudio` reads normalized audio, not original upload, after J is wired.
- Local smoke verifies:
  - original upload object exists;
  - `recording_audio_probes` exists;
  - normalized local artifact exists;
  - `recording_normalized_audios` row exists;
  - fake transcript/segment rows exist;
  - fake summary row exists;
  - Temporal workflow is `COMPLETED`;
  - `recordings.status=completed`.
- Full backend tests pass.
- Repository docs describe the implemented boundary accurately.
- No real provider credentials or external provider calls are required.

## Follow-up milestone after J

After normalized audio is in place, choose exactly one real provider integration path:

1. Local `faster-whisper` style ASR worker using normalized WAV input.
2. OpenAI-compatible transcription provider where available.
3. Another explicitly selected ASR provider.

Do not add multiple real providers at once. Keep the first provider milestone narrow: one provider, one config path, one smoke/demo path, and no provider fallback chain yet.
