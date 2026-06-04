# Recording API Skeleton Implementation Plan

> **For Hermes:** Use test-driven-development skill and implement this plan task-by-task. Keep the first milestone in-memory only; do not introduce Postgres, Temporal, storage, upload handling, auth, or frontend work.

**Goal:** Add a minimal Recording API skeleton that can create an in-memory recording and read its details/status through HTTP endpoints.

**Architecture:** Keep the API small and testable by introducing a domain model, an in-memory recording store, and HTTP handlers wired through the existing `api.NewRouter()`. Use dependency injection so tests can create isolated stores. Persistence and workflow triggering are intentionally deferred to later milestones.

**Tech Stack:** Go standard library `net/http`, `httptest`, `encoding/json`, in-memory store with `sync.RWMutex`.

---

## Non-goals

This plan does not implement:

- audio file upload or multipart handling;
- object storage / MinIO / S3 integration;
- Temporal workflow start;
- Postgres schema or migrations;
- authentication, workspace membership, or RBAC;
- frontend UI;
- pagination or search.

## API Scope

Add three endpoints:

```txt
POST /recordings
GET /recordings/{id}
GET /recordings/{id}/status
```

For this skeleton, `POST /recordings` accepts JSON metadata only:

```json
{
  "title": "Weekly sync",
  "workflow_type": "meeting",
  "language": "en"
}
```

Expected create response:

```json
{
  "id": "rec_...",
  "title": "Weekly sync",
  "status": "uploaded",
  "workflow_type": "meeting",
  "language": "en",
  "created_at": "...",
  "updated_at": "..."
}
```

## Acceptance Criteria

By the end of this plan:

- [ ] `POST /recordings` returns `201 Created` with a generated recording id.
- [ ] `GET /recordings/{id}` returns `200 OK` for an existing recording.
- [ ] `GET /recordings/{id}/status` returns `200 OK` with id/status fields.
- [ ] Unknown recording ids return `404 Not Found`.
- [ ] Invalid JSON or invalid workflow type returns `400 Bad Request`.
- [ ] `make fmt`, `make lint`, and `make test` pass.
- [ ] `docs/development.md` mentions the recording API skeleton and curl examples.

## Proposed File Layout

```txt
backend/internal/domain/
├── recording.go
└── recording_test.go
backend/internal/recordings/
├── store.go
└── store_test.go
backend/internal/api/
├── router.go
├── router_test.go
└── recordings_test.go
```

Remove `.gitkeep` from directories after adding real files.

## Task 1: Add recording domain model

**Objective:** Define the minimal recording domain type and validation helpers.

**Files:**

- Create: `backend/internal/domain/recording_test.go`
- Create: `backend/internal/domain/recording.go`
- Delete: `backend/internal/domain/.gitkeep`

**TDD steps:**

1. Write tests for valid workflow types: `memo`, `meeting`, `lecture`, `interview`.
2. Write tests that invalid workflow types are rejected.
3. Run:

   ```bash
   cd backend && go test ./internal/domain -v
   ```

   Expected RED: missing domain functions/types.

4. Implement:

   ```go
   type RecordingStatus string
   type WorkflowType string
   type Recording struct { ... }
   func IsValidWorkflowType(value string) bool
   ```

5. Run:

   ```bash
   cd backend && go test ./internal/domain -v
   ```

   Expected GREEN.

## Task 2: Add in-memory recording store

**Objective:** Provide a thread-safe in-memory store for skeleton API tests and local demos.

**Files:**

- Create: `backend/internal/recordings/store_test.go`
- Create: `backend/internal/recordings/store.go`

**TDD steps:**

1. Write tests that creating a recording assigns:
   - non-empty id with `rec_` prefix;
   - status `uploaded`;
   - non-zero `created_at` and `updated_at`.
2. Write tests that `Get(id)` returns the same recording.
3. Write tests that unknown ids return a not-found signal.
4. Run:

   ```bash
   cd backend && go test ./internal/recordings -v
   ```

   Expected RED.

5. Implement a `MemoryStore` with `sync.RWMutex` and methods:

   ```go
   func NewMemoryStore() *MemoryStore
   func (s *MemoryStore) Create(input CreateRecordingInput) (domain.Recording, error)
   func (s *MemoryStore) Get(id string) (domain.Recording, bool)
   ```

6. Run the package tests and then all backend tests.

## Task 3: Refactor API router for dependency injection

**Objective:** Let tests and command entrypoints provide a recording store while preserving simple router construction.

**Files:**

- Modify: `backend/internal/api/router.go`
- Modify: `backend/internal/api/router_test.go`
- Modify: `backend/cmd/api/main.go`

**TDD steps:**

1. Update existing router tests to use `NewRouter()` unchanged.
2. Add or adjust code so `NewRouter()` creates a default in-memory store, and `NewRouterWithStore(store)` is available for tests.
3. Run:

   ```bash
   cd backend && go test ./internal/api -v
   ```

4. Implement the smallest router options needed.
5. Run:

   ```bash
   cd backend && go test ./...
   ```

## Task 4: Add POST /recordings tests and handler

**Objective:** Allow clients to create a recording metadata skeleton.

**Files:**

- Create/Modify: `backend/internal/api/recordings_test.go`
- Modify: `backend/internal/api/router.go`

**TDD steps:**

1. Write tests for successful `POST /recordings` with JSON metadata.
2. Assert status `201 Created`, `Content-Type: application/json`, generated `id`, status `uploaded`, and echoed fields.
3. Write tests for invalid JSON returning `400 Bad Request`.
4. Write tests for unsupported workflow type returning `400 Bad Request`.
5. Run:

   ```bash
   cd backend && go test ./internal/api -v
   ```

   Expected RED.

6. Implement handler using the store.
7. Run API tests and full backend tests.

## Task 5: Add GET /recordings/{id} tests and handler

**Objective:** Fetch an existing in-memory recording by id.

**Files:**

- Modify: `backend/internal/api/recordings_test.go`
- Modify: `backend/internal/api/router.go`

**TDD steps:**

1. Write a test that creates a recording through the store, then calls `GET /recordings/{id}`.
2. Assert `200 OK` and JSON recording body.
3. Write a test for unknown id returning `404 Not Found`.
4. Run API tests for RED.
5. Implement route parsing for `/recordings/{id}`.
6. Run API tests and full backend tests.

## Task 6: Add GET /recordings/{id}/status tests and handler

**Objective:** Fetch a lightweight status response for an existing recording.

**Files:**

- Modify: `backend/internal/api/recordings_test.go`
- Modify: `backend/internal/api/router.go`

**TDD steps:**

1. Write a test that `GET /recordings/{id}/status` returns:

   ```json
   {"id":"...","status":"uploaded"}
   ```

2. Write a test for unknown id returning `404 Not Found`.
3. Run API tests for RED.
4. Implement the status handler.
5. Run API tests and full backend tests.

## Task 7: Document and verify Recording API skeleton

**Objective:** Make the new skeleton API discoverable and verify final quality gates.

**Files:**

- Modify: `docs/development.md`

**Steps:**

1. Add curl examples:

   ```bash
   curl -i -X POST http://localhost:18080/recordings \
     -H 'Content-Type: application/json' \
     -d '{"title":"Weekly sync","workflow_type":"meeting","language":"en"}'
   curl -i http://localhost:18080/recordings/<id>
   curl -i http://localhost:18080/recordings/<id>/status
   ```

2. State clearly that recordings are currently in-memory and disappear when the API process exits.
3. Run:

   ```bash
   make fmt
   make lint
   make test
   ```

4. Run a smoke test with:

   ```bash
   API_ADDRESS=:18080 make api
   ```

   Then verify create/get/status with curl.

## Commit Guidance

Suggested commits, subject to user confirmation before each commit:

```txt
feat: add recording domain model
feat: add in-memory recording store
feat: add recording api skeleton
feat: document recording api skeleton
```

Do not commit without explicit user approval.
