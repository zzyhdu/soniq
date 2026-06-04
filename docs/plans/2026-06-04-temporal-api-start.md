# Temporal API Start Implementation Plan

> **For Hermes:** Use test-driven-development and execute this plan task-by-task. Keep each task small: RED → GREEN → verify → report → wait for user commit confirmation.

**Goal:** Replace the API's default no-op recording processor with an optional real Temporal-backed processor that starts `RecordingProcessingWorkflow` after `POST /recordings` succeeds.

**Architecture:** Keep the HTTP API decoupled from Temporal through the existing `api.RecordingProcessor` seam. Add a Temporal implementation in a separate package, then wire `cmd/api` to create a Temporal client and inject that processor. Tests must not require a running Temporal server; use a small fake/stub client around the minimal workflow-start interface.

**Tech Stack:** Go 1.24+, `go.temporal.io/sdk v1.44.1`, standard-library HTTP tests, strict TDD.

---

## Current state

Already implemented:

- `api.RecordingProcessor` interface:
  - `Enqueue(recording domain.Recording) error`
- API constructor seam:
  - `api.NewRouterWithProcessor(store, processor)`
- Temporal workflow skeleton:
  - `workflows.RecordingProcessingWorkflow`
  - `workflows.RecordingProcessingInput`
- Worker registration:
  - worker registers workflow and activity stubs.

Current gap:

```txt
POST /recordings
  ↓
api.RecordingProcessor.Enqueue(recording)
  ↓
noopRecordingProcessor
```

Target after this plan:

```txt
POST /recordings
  ↓
api.RecordingProcessor.Enqueue(recording)
  ↓
TemporalRecordingProcessor.ExecuteWorkflow(...)
  ↓
RecordingProcessingWorkflow
```

---

## Important boundaries

Do not add in this milestone:

- Docker Compose or local Temporal server setup.
- Postgres persistence.
- Audio upload/storage.
- ffmpeg, ASR, LLM, webhooks.
- Production smoke tests requiring Temporal to be running.

Unit tests must stay hermetic and must pass with:

```bash
cd backend && go test ./...
```

without a Temporal server.

---

## Task A1: Add Temporal recording processor tests

**Objective:** Define the Temporal processor behavior without touching API wiring yet.

**Files:**

- Create: `backend/internal/processing/temporal_recording_processor_test.go`
- Later implementation file: `backend/internal/processing/temporal_recording_processor.go`

**RED test behavior:**

Create tests for a new processor API similar to:

```go
processor := NewTemporalRecordingProcessor(fakeStarter, TemporalRecordingProcessorConfig{
    TaskQueue: "soniq-audio-pipeline",
})
err := processor.Enqueue(recording)
```

Expected behavior:

1. Starts `workflows.RecordingProcessingWorkflow`.
2. Uses workflow ID:
   ```txt
   recording-processing-<recording.ID>
   ```
3. Uses configured task queue.
4. Passes workflow input:
   ```go
   workflows.RecordingProcessingInput{
       RecordingID: recording.ID,
       WorkflowType: recording.WorkflowType,
       Language: recording.Language,
   }
   ```
5. Returns start errors from the workflow starter.
6. Rejects empty task queue in constructor or enqueue path.

**Expected RED:** missing `NewTemporalRecordingProcessor`, config type, and implementation.

**Verification command:**

```bash
cd backend && go test ./internal/processing -v
```

**Suggested commit:**

```txt
test: add temporal recording processor red tests
```

---

## Task A2: Implement Temporal recording processor

**Objective:** Implement the processor and keep it independent from HTTP handlers.

**Files:**

- Create: `backend/internal/processing/temporal_recording_processor.go`
- Delete if present: `backend/internal/processing/.gitkeep`

**Implementation shape:**

Use a minimal interface so tests can fake workflow start without a live Temporal server:

```go
type WorkflowStarter interface {
    ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error)
}
```

Processor shape:

```go
type TemporalRecordingProcessorConfig struct {
    TaskQueue string
}

type TemporalRecordingProcessor struct {
    starter WorkflowStarter
    config  TemporalRecordingProcessorConfig
}
```

`Enqueue(recording domain.Recording) error` should call:

```go
starter.ExecuteWorkflow(
    context.Background(),
    client.StartWorkflowOptions{
        ID:        "recording-processing-" + recording.ID,
        TaskQueue: config.TaskQueue,
    },
    workflows.RecordingProcessingWorkflow,
    workflows.RecordingProcessingInput{
        RecordingID:  recording.ID,
        WorkflowType: recording.WorkflowType,
        Language:     recording.Language,
    },
)
```

Do not wait for workflow completion.

**GREEN verification:**

```bash
cd backend && go test ./internal/processing -v
cd backend && go test ./...
```

**Suggested commit:**

```txt
feat: add temporal recording processor
```

---

## Task A3: Wire `cmd/api` to Temporal processor

**Objective:** Make the production API command inject the Temporal processor instead of the no-op default.

**Files:**

- Modify: `backend/cmd/api/main.go`
- Test: `backend/cmd/api/main_test.go`

**Design:**

Extract API construction into a testable function, for example:

```go
func buildHandler(ctx context.Context, cfg config.Config) (http.Handler, func(), error)
```

or split smaller seams:

```go
func newTemporalClient(ctx context.Context, cfg config.Config) (client.Client, error)
func newAPIHandlerWithProcessor(processor api.RecordingProcessor) http.Handler
```

Use Temporal config already available:

- `cfg.TemporalAddress`
- `cfg.TemporalNamespace`
- `cfg.TemporalTaskQueue`

Production flow:

```txt
config.LoadFromEnv
  ↓
client.DialContext
  ↓
processing.NewTemporalRecordingProcessor(temporalClient, task queue)
  ↓
api.NewRouterWithProcessor(recordings.NewMemoryStore(), processor)
  ↓
http.ListenAndServe
```

**Testing requirements:**

- Unit tests must not dial a real Temporal server.
- Prefer extracting a builder that accepts a fake/stub Temporal client factory.
- Verify that a processor is injected using configured task queue.
- Verify client close function is called by the cleanup path if practical.

**Verification:**

```bash
cd backend && go test ./cmd/api -v
cd backend && go test ./...
```

**Suggested commit:**

```txt
feat: wire api to temporal recording processor
```

---

## Task A4: Document API-to-Temporal local flow

**Objective:** Update docs to explain that the API now attempts to start Temporal workflows.

**Files:**

- Modify: `docs/development.md`
- Modify: `docs/workflows.md` if needed

**Docs should clarify:**

- `make api` now requires Temporal to be reachable if using the real Temporal processor.
- `POST /recordings` starts `RecordingProcessingWorkflow` asynchronously.
- Unit tests still do not require Temporal.
- The workflow still uses stub activities and does not process real audio.
- Local end-to-end testing requires both `make worker` and `make api` with the same task queue.

**Verification:**

```bash
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
docs: document api temporal workflow start
```

---

## Review checklist

Before considering the milestone complete:

- [ ] `POST /recordings` starts `RecordingProcessingWorkflow` through the processor seam.
- [ ] API handlers still do not import Temporal directly.
- [ ] Tests do not require a running Temporal server.
- [ ] Workflow ID is deterministic from recording ID.
- [ ] Task queue comes from config.
- [ ] `cd backend && go test ./...` passes.
- [ ] Docs accurately distinguish implemented skeleton behavior from future audio processing.
