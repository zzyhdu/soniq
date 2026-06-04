# Temporal Workflow Skeleton Implementation Plan

> **For Hermes:** Use test-driven-development skill and implement this plan task-by-task. Use the real Temporal Go SDK and Temporal test suite, but keep the first workflow skeleton mock-only: no ffmpeg, ASR, LLM, Postgres, object storage, or production Temporal server requirement for tests.

**Goal:** Introduce a real Temporal Go SDK workflow skeleton for recording processing, with unit tests, activity stubs, worker registration, and documentation.

**Architecture:** Keep deterministic workflow code in `backend/internal/workflows` and all external or side-effecting work in activities under `backend/internal/activities`, matching ADR-0001. Use Temporal's Go SDK test suite for workflow unit tests so local verification does not require a running Temporal server. Keep API enqueue integration optional and config-gated until a local Temporal server is available.

**Tech Stack:** Go, `go.temporal.io/sdk`, Temporal Go SDK `testsuite`, Go standard library, existing in-memory recording API skeleton.

---

## Source context

- `docs/adr/0001-use-temporal.md` says Soniq uses Temporal for durable workflow orchestration.
- ADR-0001 also requires external I/O to live in activities, not workflow code.
- `docs/workflows.md` defines `RecordingProcessingWorkflow` and the full future step sequence.
- This milestone implements only the first real Temporal skeleton, not the complete audio pipeline.

## Non-goals

This plan does not implement:

- a production Temporal server stack;
- Docker Compose for Temporal;
- real audio probing, normalization, chunking, or ffmpeg calls;
- real ASR or LLM provider calls;
- Postgres persistence;
- object storage writes;
- webhook/signal pattern;
- retry failed step, cancellation, or reprocessing UI;
- frontend changes.

## Skeleton workflow scope

The first Temporal skeleton should prove the workflow shape:

```txt
ValidateRecording
  ↓
MarkRecordingProcessing
  ↓
CompleteRecordingProcessing
```

These are mock/stub activities. They do not perform real external I/O yet, but they establish:

- workflow input shape;
- workflow result shape;
- activity boundary;
- deterministic workflow orchestration;
- Temporal testsuite verification;
- worker registration.

## Acceptance Criteria

By the end of this plan:

- [ ] `go.temporal.io/sdk` is added to `backend/go.mod` and `backend/go.sum`.
- [ ] `RecordingProcessingWorkflow` exists in `backend/internal/workflows`.
- [ ] Temporal workflow unit tests pass with the SDK testsuite.
- [ ] Activity stubs exist in `backend/internal/activities` and are registered by the worker.
- [ ] `make fmt`, `make lint`, and `make test` pass.
- [ ] `docs/development.md` documents the Temporal skeleton boundary.
- [ ] No secrets, API keys, tokens, passwords, or connection strings are committed.

## Proposed File Layout

```txt
backend/internal/workflows/
├── recording_processing.go
└── recording_processing_test.go
backend/internal/activities/
├── recording_processing.go
└── recording_processing_test.go
backend/cmd/worker/main.go
backend/go.mod
backend/go.sum
```

Remove `.gitkeep` from directories after adding real files.

## Task W1: Add Temporal Go SDK dependency

**Objective:** Add the Temporal Go SDK in a small isolated dependency commit.

**Files:**

- Modify: `backend/go.mod`
- Create/Modify: `backend/go.sum`

**Steps:**

1. Run:

   ```bash
   cd backend && go get go.temporal.io/sdk@latest
   ```

2. Verify dependency resolution:

   ```bash
   cd backend && go mod tidy && go test ./...
   ```

3. Confirm `go.mod` includes `go.temporal.io/sdk` and no unrelated dependencies are manually added.

**Acceptance Criteria:**

- `cd backend && go test ./...` passes.
- `backend/go.mod` and `backend/go.sum` are the only changed files.

**Suggested commit message:**

```txt
chore: add temporal go sdk
```

## Task W2: Add workflow input/result types and workflow test RED

**Objective:** Define the desired workflow API through a failing Temporal testsuite test.

**Files:**

- Create: `backend/internal/workflows/recording_processing_test.go`
- Later create: `backend/internal/workflows/recording_processing.go`
- Delete: `backend/internal/workflows/.gitkeep` after real files exist.

**TDD steps:**

1. Write a Temporal testsuite test for `RecordingProcessingWorkflow` with input:

   ```go
   RecordingProcessingInput{
       RecordingID: "rec_test",
       WorkflowType: "meeting",
       Language: "en",
   }
   ```

2. Mock activity calls for:

   ```txt
   ValidateRecordingActivity
   MarkRecordingProcessingActivity
   CompleteRecordingProcessingActivity
   ```

3. Expect result:

   ```go
   RecordingProcessingResult{
       RecordingID: "rec_test",
       Status: "completed",
   }
   ```

4. Run:

   ```bash
   cd backend && go test ./internal/workflows -v
   ```

   Expected RED: missing workflow/types/activities.

## Task W3: Implement RecordingProcessingWorkflow GREEN

**Objective:** Implement the minimal deterministic workflow that satisfies W2.

**Files:**

- Create: `backend/internal/workflows/recording_processing.go`
- Modify: `backend/internal/workflows/recording_processing_test.go` if needed only for compile corrections.
- Delete: `backend/internal/workflows/.gitkeep`

**Implementation notes:**

- Use `workflow.ExecuteActivity` for each activity.
- Do not call `time.Now`, random, network, filesystem, DB, or logs outside Temporal workflow APIs.
- Set activity options inside workflow code, e.g. `StartToCloseTimeout`.
- Return `RecordingProcessingResult` with completed status after the final activity.

**Verification:**

```bash
cd backend && go test ./internal/workflows -v && go test ./...
```

## Task W4: Add activity stubs with tests

**Objective:** Add real activity functions that currently return skeleton success values.

**Files:**

- Create: `backend/internal/activities/recording_processing_test.go`
- Create: `backend/internal/activities/recording_processing.go`
- Delete: `backend/internal/activities/.gitkeep`

**TDD steps:**

1. Write tests for:

   ```txt
   ValidateRecordingActivity
   MarkRecordingProcessingActivity
   CompleteRecordingProcessingActivity
   ```

2. Verify each activity accepts the workflow input or recording id and returns nil/skeleton output.
3. Run focused tests for RED.
4. Implement minimal stubs.
5. Run focused tests and full backend tests for GREEN.

**Important:** These activities may later do external I/O, but this task should not add DB, storage, ffmpeg, ASR, or LLM calls.

## Task W5: Register workflow and activities in worker

**Objective:** Make `cmd/worker` register the Temporal workflow and activity stubs with a real Temporal worker object.

**Files:**

- Modify: `backend/cmd/worker/main.go`

**Steps:**

1. Refactor worker startup so registration can be tested without requiring a running Temporal server if practical.
2. Register:

   ```txt
   workflows.RecordingProcessingWorkflow
   activities.ValidateRecordingActivity
   activities.MarkRecordingProcessingActivity
   activities.CompleteRecordingProcessingActivity
   ```

3. Keep secret redaction behavior: do not print API keys or credentials.
4. Do not require a production Temporal server for `make test`.

**Verification:**

```bash
cd backend && go test ./...
```

If `make worker` now attempts to connect to Temporal, document that local Temporal must be running before using it. If we keep a dry-run/register-only mode, document that behavior.

## Task W6: Add optional API enqueue seam

**Objective:** Prepare `POST /recordings` to enqueue processing later without requiring Temporal in local tests.

**Files:**

- Modify or create small API/service files as needed.
- Modify tests in `backend/internal/api`.

**Recommended design:**

Introduce a small interface near the API layer:

```go
type RecordingProcessor interface {
    StartRecordingProcessing(ctx context.Context, recording domain.Recording) error
}
```

Default local implementation is no-op. Future implementation can call Temporal client.

**Acceptance Criteria:**

- Existing `POST /recordings` behavior stays the same.
- Tests prove the processor is invoked after successful recording creation.
- Invalid JSON and invalid workflow type must not invoke the processor.
- No Temporal server required in tests.

## Task W7: Document and verify Temporal workflow skeleton

**Objective:** Document how the skeleton works and run final quality checks.

**Files:**

- Modify: `docs/development.md`
- Optionally modify: `docs/workflows.md` if it needs an implementation-status note.

**Steps:**

1. Add a section explaining:
   - Temporal Go SDK is present;
   - workflow tests use SDK testsuite and do not require a local Temporal server;
   - real activities are still stubs;
   - production worker connection requires Temporal server configuration.
2. Run:

   ```bash
   make fmt
   make lint
   make test
   ```

3. If a local Temporal server is not configured, do not fake a successful production worker run. Clearly document what was and was not smoke-tested.

## Commit Guidance

Suggested commits, subject to user confirmation before each commit:

```txt
chore: add temporal go sdk
feat: add recording processing workflow skeleton
feat: add recording processing activity stubs
feat: register temporal worker skeleton
feat: add recording processor enqueue seam
feat: document temporal workflow skeleton
```

Do not commit without explicit user approval.
