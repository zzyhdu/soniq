# Transcription and Summarization Foundation Implementation Plan

> **For Hermes:** Execute this plan task-by-task. Keep repository docs tool-agnostic, use small commits, and ask before every commit.

**Goal:** Extend the current upload → ffprobe workflow into the first provider-neutral transcription and summarization pipeline, persisting raw transcript text/segments and generated summaries without coupling the implementation to a real external ASR or LLM provider yet.

**Architecture:** Build the 1F milestone in two layers. First add Soniq-owned persistence tables and store methods for transcription results and summaries. Then add provider-neutral Go seams and fake deterministic providers so `RecordingProcessingWorkflow` can transition through `transcribing` and `summarizing`, persist results, and complete locally without network credentials. Real providers such as faster-whisper HTTP and OpenAI-compatible LLMs remain follow-up work after the local deterministic pipeline is proven.

**Tech Stack:** Go, Temporal Go SDK, Postgres migrations, pgx-backed stores, local filesystem object storage, existing `recordings` package stores, existing `activities.RecordingProcessingActivities`, deterministic fake ASR/LLM providers for tests and smoke.

---

## Current-state findings

- The repository now has a working local smoke path:
  `POST /recordings/upload` → local object store → Soniq Postgres → Temporal workflow → worker → `ffprobe` → `recording_audio_probes` → `recordings.status=completed`.
- `RecordingProcessingWorkflow` currently executes stable Temporal activity names:
  1. `ValidateRecordingActivity`
  2. `MarkRecordingProcessingActivity`
  3. `ProbeRecordingAudioActivity`
  4. `CompleteRecordingProcessingActivity`
- Worker registration already maps those names to store-backed `RecordingProcessingActivities` methods.
- `recordings.status` already supports the next status values in migration `0001_create_recordings.up.sql`:
  - `transcribing`
  - `summarizing`
- `docs/data-model.md` already identifies first-class future concepts:
  - `TranscriptSegment`
  - `Summary`
  - raw/clean transcript artifacts
  - summary artifacts
- `docs/providers.md` defines target provider categories:
  - `TranscriptionProvider` with synchronous/asynchronous providers
  - `LLMProvider` with OpenAI-compatible and Ollama initial implementations
- `docs/roadmap.md` says Phase 1F is planned and should cover:
  - audio normalization/format policy
  - one transcription provider
  - one LLM provider
  - persisted transcript and summary

## Product decision for this milestone

H is a **provider-neutral transcription/summarization foundation** milestone, not the real provider integration milestone.

Do now:

- Add minimal transcript and summary persistence.
- Add activity/status flow for `transcribing` and `summarizing`.
- Add deterministic fake transcription and summarization providers that require no credentials.
- Persist transcript and summary rows from workflow activities.
- Extend smoke to prove transcript and summary rows exist after workflow completion.
- Keep real provider interfaces shaped so faster-whisper/OpenAI-compatible integrations can be added later.

Do not do now:

- Do not introduce real external ASR/LLM API calls.
- Do not add provider credentials or config secrets.
- Do not implement async webhook/signal providers.
- Do not implement chunking/map-reduce for long audio.
- Do not introduce a generic artifact/versioning system yet.
- Do not add MinIO/S3 object read support in this milestone.
- Do not implement a web UI.

## Target behavior

After a successful audio upload:

```txt
POST /recordings/upload
  -> original audio is stored in local object storage
  -> recordings.status = uploaded
  -> Temporal workflow starts
  -> MarkRecordingProcessingActivity updates status = processing
  -> ProbeRecordingAudioActivity stores ffprobe metadata
  -> MarkRecordingTranscribingActivity updates status = transcribing
  -> TranscribeRecordingAudioActivity persists transcript rows
  -> MarkRecordingSummarizingActivity updates status = summarizing
  -> SummarizeRecordingActivity persists summary row
  -> CompleteRecordingProcessingActivity updates status = completed
  -> smoke verifies Temporal COMPLETED, recordings.status=completed, probe metadata exists, transcript exists, and summary exists
```

If transcription or summarization fails, the workflow should return the original error and use the existing best-effort failure path so `recordings.status` can become `failed`.

## Proposed schema

Create `backend/migrations/0004_create_recording_transcripts_and_summaries.up.sql`:

```sql
CREATE TABLE recording_transcripts (
  recording_id TEXT PRIMARY KEY REFERENCES recordings(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  model TEXT NOT NULL DEFAULT '',
  language TEXT NOT NULL DEFAULT '',
  text TEXT NOT NULL,
  raw_result_json JSONB NOT NULL,
  transcribed_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE recording_transcript_segments (
  id TEXT PRIMARY KEY,
  recording_id TEXT NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
  segment_index INTEGER NOT NULL,
  start_ms INTEGER NOT NULL DEFAULT 0,
  end_ms INTEGER NOT NULL DEFAULT 0,
  speaker_label TEXT NOT NULL DEFAULT '',
  text TEXT NOT NULL,
  confidence DOUBLE PRECISION,
  created_at TIMESTAMPTZ NOT NULL,
  UNIQUE (recording_id, segment_index)
);

CREATE TABLE recording_summaries (
  recording_id TEXT PRIMARY KEY REFERENCES recordings(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  model TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  overview TEXT NOT NULL,
  content_markdown TEXT NOT NULL,
  raw_result_json JSONB NOT NULL,
  summarized_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
```

Create matching down migration:

```sql
DROP TABLE IF EXISTS recording_summaries;
DROP TABLE IF EXISTS recording_transcript_segments;
DROP TABLE IF EXISTS recording_transcripts;
```

Rationale:

- Keep transcript and summary state outside the main `recordings` table.
- Use one latest transcript and one latest summary per recording for the first milestone.
- Allow multiple transcript segments for future diarization/chunking without requiring it now.
- Keep `raw_result_json` so provider-specific details are not lost.
- Defer generic artifact/version tables until normalized audio, exports, or reprocessing require them.

## Provider seam shape

Add provider-neutral types in `backend/internal/activities` for the first milestone:

```go
type TranscriptionProvider interface {
    Name() string
    Transcribe(ctx context.Context, input TranscriptionInput) (TranscriptionResult, error)
}

type TranscriptionInput struct {
    RecordingID string
    AudioPath   string
    Language    string
}

type TranscriptionResult struct {
    Provider      string
    Model         string
    Language      string
    Text          string
    Segments      []TranscriptSegmentResult
    RawResultJSON []byte
    TranscribedAt time.Time
}

type SummaryProvider interface {
    Name() string
    Summarize(ctx context.Context, input SummaryInput) (SummaryResult, error)
}

type SummaryInput struct {
    RecordingID   string
    WorkflowType  domain.WorkflowType
    Language      string
    TranscriptText string
}

type SummaryResult struct {
    Provider      string
    Model         string
    Type          domain.WorkflowType
    Title         string
    Overview      string
    ContentMarkdown string
    RawResultJSON []byte
    SummarizedAt  time.Time
}
```

The exact field names may be adjusted during implementation, but the acceptance criteria are:

- provider interfaces accept context;
- fake providers are deterministic;
- activities can be tested without network calls;
- store methods receive normalized persistence inputs, not provider-specific structs.

---

## Task H1: Add this implementation plan

**Objective:** Save the 1F plan without modifying implementation files.

**Files:**

- Create: `docs/plans/2026-06-06-transcription-summarization-foundation.md`

**Plan-only guardrail:**

- Capture `git status --short` before and after.
- Only this plan file may change.
- Do not edit Go source, tests, migrations, scripts, runtime docs, or dependency files in this task.

**Verification:**

```bash
git status --short
```

Expected: only the plan file is modified/created in git.

**Suggested commit:**

```txt
docs: add transcription summarization foundation plan
```

---

## Task H2: Add transcript/summary store RED tests

**Objective:** Define the persistence contract for transcripts, transcript segments, and summaries before implementation.

**Files:**

- Modify: `backend/internal/recordings/store_test.go`
- Modify: `backend/internal/recordings/postgres_store_test.go`

**TDD expectation:**

Add failing tests for:

1. Memory store can upsert and get a transcript by recording ID.
2. Memory store can upsert and replace transcript segments by recording ID.
3. Memory store can upsert and get a summary by recording ID.
4. Upserts preserve `CreatedAt` and advance `UpdatedAt` for latest transcript/summary rows.
5. Missing transcript/summary getters return `ok=false`.
6. Postgres store inserts/upserts transcript, segments, and summary using deterministic queries.
7. Postgres get handles `sql.ErrNoRows` as `ok=false`.

**Expected RED failure:**

```txt
undefined: RecordingTranscript
undefined: RecordingTranscriptSegment
undefined: RecordingSummary
store.UpsertTranscript undefined
store.GetTranscript undefined
store.UpsertSummary undefined
store.GetSummary undefined
```

**Verification command:**

```bash
cd backend && go test ./internal/recordings -run 'Transcript|Summary' -v
```

**Suggested commit:**

Commit together with H3 after GREEN unless the user explicitly asks for separate RED-only commits.

---

## Task H3: Implement transcript/summary persistence and migration

**Objective:** Add persistence models, in-memory store support, Postgres store support, and migration `0004`.

**Files:**

- Create: `backend/migrations/0004_create_recording_transcripts_and_summaries.up.sql`
- Create: `backend/migrations/0004_create_recording_transcripts_and_summaries.down.sql`
- Modify: `backend/internal/recordings/store.go`
- Modify: `backend/internal/recordings/postgres_store.go`
- Modify: `backend/internal/recordings/store_test.go`
- Modify: `backend/internal/recordings/postgres_store_test.go`

**Implementation notes:**

- Add structs:
  - `RecordingTranscript`
  - `RecordingTranscriptSegment`
  - `RecordingSummary`
  - `UpsertTranscriptInput`
  - `UpsertSummaryInput`
- Add store methods:
  - `UpsertTranscript(input UpsertTranscriptInput) (RecordingTranscript, error)`
  - `GetTranscript(recordingID string) (RecordingTranscript, bool)`
  - `ListTranscriptSegments(recordingID string) []RecordingTranscriptSegment`
  - `UpsertSummary(input UpsertSummaryInput) (RecordingSummary, error)`
  - `GetSummary(recordingID string) (RecordingSummary, bool)`
- For transcript segment IDs, use deterministic IDs such as:

```txt
<recording_id>-seg-000001
```

- Store raw provider JSON as copied `[]byte` to avoid caller mutation.

**Verification commands:**

```bash
cd backend && go test ./internal/recordings -run 'Transcript|Summary' -v
cd backend && go test ./internal/recordings -v
```

**Suggested commit:**

```txt
feat: add transcript and summary persistence
```

---

## Task H4: Add activity provider seams and fake provider RED tests

**Objective:** Define deterministic transcription/summarization provider seams and activity-level tests before implementation.

**Files:**

- Modify: `backend/internal/activities/recording_processing.go`
- Modify: `backend/internal/activities/recording_processing_test.go`
- Create or modify: `backend/internal/activities/transcription_summary_test.go`

**TDD expectation:**

Add failing tests for:

1. `TranscribeRecordingAudio` rejects missing recording ID.
2. `TranscribeRecordingAudio` requires store, local object path resolver, and transcription provider.
3. `TranscribeRecordingAudio` loads the recording, resolves `AudioObjectKey`, calls provider with local audio path/language, and persists transcript + segments.
4. `SummarizeRecording` rejects missing recording ID.
5. `SummarizeRecording` requires store and summary provider.
6. `SummarizeRecording` loads transcript text and persists summary.
7. Provider errors are returned without persisting partial results.

**Expected RED failure:**

```txt
activities.TranscribeRecordingAudio undefined
activities.SummarizeRecording undefined
TranscriptionProvider undefined
SummaryProvider undefined
RecordingStore missing transcript/summary methods
```

**Verification command:**

```bash
cd backend && go test ./internal/activities -run 'Transcribe|Summarize' -v
```

**Suggested commit:**

Commit together with H5 after GREEN unless the user explicitly asks for separate RED-only commits.

---

## Task H5: Implement transcription/summarization activities with deterministic fake providers

**Objective:** Make activity tests pass with no network calls.

**Files:**

- Modify: `backend/internal/activities/recording_processing.go`
- Modify: `backend/internal/activities/recording_processing_test.go`
- Create or modify: `backend/internal/activities/transcription_summary_test.go`

**Implementation notes:**

- Extend `RecordingStore` interface with transcript/summary methods from H3.
- Extend `RecordingProcessingActivities` dependencies:

```go
transcriptionProvider TranscriptionProvider
summaryProvider       SummaryProvider
```

- Add constructor variant if needed:

```go
NewRecordingProcessingActivitiesWithPipeline(store, resolver, probeRunner, transcriptionProvider, summaryProvider)
```

- Implement deterministic fake providers for tests and local worker default:
  - fake transcription text can include recording ID and file basename;
  - fake summary can use first sentence/first N characters of transcript text;
  - raw JSON should be valid JSON object.

**Verification commands:**

```bash
cd backend && go test ./internal/activities -run 'Transcribe|Summarize' -v
cd backend && go test ./internal/activities -v
```

**Suggested commit:**

```txt
feat: add deterministic transcription summary activities
```

---

## Task H6: Add workflow RED tests for transcribing/summarizing sequence

**Objective:** Prove the workflow should call transcription and summarization activities in the correct order and failure paths before implementation.

**Files:**

- Modify: `backend/internal/workflows/recording_processing_test.go`

**TDD expectation:**

Add failing tests for:

1. Successful workflow calls:
   - validate
   - mark processing
   - probe audio
   - mark transcribing
   - transcribe audio
   - mark summarizing
   - summarize recording
   - complete
2. If transcription fails, workflow calls `FailRecordingProcessingActivity` and returns transcription error.
3. If summarization fails, workflow calls `FailRecordingProcessingActivity` and returns summary error.

**Expected RED failure:**

```txt
expected activity MarkRecordingTranscribingActivity was not called
expected activity TranscribeRecordingAudioActivity was not called
expected activity MarkRecordingSummarizingActivity was not called
expected activity SummarizeRecordingActivity was not called
```

**Verification command:**

```bash
cd backend && go test ./internal/workflows -run RecordingProcessingWorkflow -v
```

**Suggested commit:**

Commit together with H7 after GREEN unless the user explicitly asks for separate RED-only commits.

---

## Task H7: Wire workflow activity names and status transitions

**Objective:** Add stable Temporal activity names and update workflow order/status transitions.

**Files:**

- Modify: `backend/internal/activities/recording_processing.go`
- Modify: `backend/internal/workflows/recording_processing.go`
- Modify: `backend/internal/workflows/recording_processing_test.go`
- Modify: `backend/internal/activities/recording_processing_test.go`

**Implementation notes:**

Add stable names:

```go
MarkRecordingTranscribingActivityName = "MarkRecordingTranscribingActivity"
TranscribeRecordingAudioActivityName  = "TranscribeRecordingAudioActivity"
MarkRecordingSummarizingActivityName  = "MarkRecordingSummarizingActivity"
SummarizeRecordingActivityName        = "SummarizeRecordingActivity"
```

Add methods:

```go
func (a *RecordingProcessingActivities) MarkRecordingTranscribing(ctx context.Context, recordingID string) error
func (a *RecordingProcessingActivities) MarkRecordingSummarizing(ctx context.Context, recordingID string) error
```

These should reuse `updateStatus` with existing domain statuses:

- `domain.RecordingStatusTranscribing`
- `domain.RecordingStatusSummarizing`

Workflow order should become:

```txt
ValidateRecording
MarkRecordingProcessing
ProbeRecordingAudio
MarkRecordingTranscribing
TranscribeRecordingAudio
MarkRecordingSummarizing
SummarizeRecording
CompleteRecordingProcessing
```

**Verification commands:**

```bash
cd backend && go test ./internal/workflows -run RecordingProcessingWorkflow -v
cd backend && go test ./internal/activities -run 'MarkRecordingTranscribing|MarkRecordingSummarizing|Transcribe|Summarize' -v
```

**Suggested commit:**

```txt
feat: extend recording workflow through transcription summary
```

---

## Task H8: Wire worker registration with fake providers

**Objective:** Make the production worker register the new activity names against store-backed methods using deterministic local fake providers.

**Files:**

- Modify: `backend/cmd/worker/main.go`
- Modify: `backend/cmd/worker/main_test.go`

**Implementation notes:**

- Register the new methods under stable activity names.
- Keep default providers local and deterministic for now.
- Do not add env vars for external providers yet.
- `main_test.go` should assert the worker registers all expected activity names.

**Verification commands:**

```bash
cd backend && go test ./cmd/worker -v
cd backend && go test ./internal/workflows ./internal/activities ./cmd/worker -v
```

**Suggested commit:**

```txt
feat: register transcription summary activities
```

---

## Task H9: Extend smoke to verify transcript and summary rows

**Objective:** Prove the full local path reaches probe, transcript, summary, and completion using real API/worker/Temporal/Postgres, but fake ASR/LLM providers.

**Files:**

- Modify: `scripts/smoke-postgres-temporal.sh`

**Implementation notes:**

- Apply migration `0004` if transcript/summary tables do not exist.
- After Temporal `COMPLETED` and `recordings.status=completed`, assert:
  - `recording_audio_probes` row exists;
  - `recording_transcripts` row exists with non-empty `text` and JSON object `raw_result_json`;
  - `recording_transcript_segments` has at least one row;
  - `recording_summaries` row exists with non-empty `overview` or `content_markdown` and JSON object `raw_result_json`.

**Verification commands:**

```bash
bash -n scripts/smoke-postgres-temporal.sh
API_URL=http://localhost:18080 API_ADDRESS=:18080 make smoke-postgres-temporal
```

**Suggested commit:**

```txt
test: verify transcript summary smoke workflow
```

---

## Task H10: Update docs for the 1F implemented boundary

**Objective:** Update runtime docs after implementation is verified.

**Files:**

- Modify: `docs/development.md`
- Modify: `docs/workflows.md`
- Modify: `docs/roadmap.md`
- Optionally modify: `docs/data-model.md`
- Optionally modify: `docs/providers.md`

**Documentation updates:**

- Current implementation status should mention fake deterministic providers, not real external provider support.
- Workflow sequence should include transcription and summarization activities before completion.
- Roadmap 1F should move to complete only after smoke passes.
- Provider docs should distinguish:
  - current fake local providers for development verification;
  - future real `faster_whisper` and OpenAI-compatible providers.

**Stale wording scan:**

```bash
grep -RIn \
  "does not yet.*ASR\|does not yet.*LLM\|1F — First transcription and summarization activities.*Status: planned" \
  docs/development.md docs/workflows.md docs/roadmap.md docs/providers.md || true
```

**Verification commands:**

```bash
git diff --check
cd backend && go test ./...
```

**Suggested commit:**

```txt
docs: document transcription summary workflow boundary
```

---

## Task H11: Optional cleanup/review pass

**Objective:** Review the H milestone for accidental temporary seams, stale wording, or over-coupling before moving to real providers.

**Files:**

- No expected file changes unless review finds issues.

**Review checklist:**

- No package-level temporary activity functions were introduced.
- Activity names are constants and worker registration uses those constants.
- Workflow remains deterministic: no direct file IO, network calls, time.Now, or random IDs inside workflow code.
- Activities own side effects.
- Fake providers are named as fake/deterministic and cannot be mistaken for production ASR/LLM quality.
- Store methods copy JSON byte slices where needed.
- Smoke proves rows exist using real Postgres/Temporal.
- No real provider credentials/config were added.

**Verification commands:**

```bash
cd backend && go test ./...
git diff --check
git status --short
```

**Suggested commit:**

If changes are needed:

```txt
refactor: clean transcription summary foundation
```

If no changes are needed, report clean review and move to the next milestone.

---

## Acceptance criteria for H milestone

The H milestone is complete when:

- `recording_transcripts`, `recording_transcript_segments`, and `recording_summaries` persistence is implemented and tested.
- Workflow status transitions include `transcribing` and `summarizing` before `completed`.
- Transcription and summarization activities are provider-neutral and locally deterministic.
- Worker registers all activities under stable Temporal activity name constants.
- Full local smoke verifies:
  - upload stored original audio;
  - probe metadata persisted;
  - transcript persisted;
  - at least one transcript segment persisted;
  - summary persisted;
  - Temporal workflow completed;
  - `recordings.status=completed`.
- No real external ASR/LLM provider credentials are required.
- `cd backend && go test ./...` passes.
- `git diff --check` passes.

## Follow-up milestone after H

After H is complete, the next milestone should choose exactly one real provider integration path:

1. **Local faster-whisper HTTP provider** for ASR, because it aligns with offline/self-hosted goals.
2. **OpenAI-compatible LLM provider** for summarization, because it can support DeepSeek/Qwen/Kimi/vLLM-style endpoints behind one adapter.

That follow-up should verify latest stable dependencies before adding modules, and each new dependency should be introduced together with the first test/code import that uses it.
