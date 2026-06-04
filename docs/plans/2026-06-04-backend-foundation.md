# Backend Foundation Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task when delegating execution.

**Goal:** Turn Soniq from a documentation skeleton into a runnable Go backend foundation with repeatable local quality commands.

**Architecture:** Create a minimal Go backend under `backend/` with separate API and worker entrypoints. Add configuration loading, health checks, a placeholder worker process, and project-level Makefile commands. Keep the implementation intentionally small; Temporal, database, and storage integrations are introduced in later plans.

**Tech Stack:** Go, standard library HTTP server initially, future-compatible internal package layout, Makefile.

---

## Non-goals

This plan does not implement:

- Postgres schema or migrations;
- Temporal workflow logic;
- MinIO/S3 upload;
- ffmpeg processing;
- real ASR or LLM providers;
- frontend UI;
- authentication/RBAC.

Those belong to later milestones.

## Acceptance Criteria

By the end of this plan:

- [ ] `backend/go.mod` exists.
- [ ] `go test ./...` passes inside `backend/`.
- [ ] `go fmt ./...` produces no changes.
- [ ] API server starts with `go run ./cmd/api`.
- [ ] API exposes `GET /healthz` returning `200 OK` and JSON status.
- [ ] Worker starts with `go run ./cmd/worker` and validates config.
- [ ] Root `Makefile` exposes `fmt`, `lint`, `test`, `api`, and `worker` targets.
- [ ] `.env.example` remains aligned with config keys.
- [ ] README or docs mention local backend commands.

## Proposed File Layout

```txt
backend/
├── go.mod
├── cmd/
│   ├── api/
│   │   └── main.go
│   └── worker/
│       └── main.go
└── internal/
    ├── api/
    │   ├── router.go
    │   └── router_test.go
    └── config/
        ├── config.go
        └── config_test.go
Makefile
```

## Task 1: Initialize Go module

**Objective:** Create the backend Go module with the intended module path.

**Files:**

- Create: `backend/go.mod`

**Steps:**

1. From `backend/`, run:

   ```bash
   go mod init github.com/zzyhdu/soniq/backend
   ```

2. Run:

   ```bash
   go mod tidy
   ```

3. Verify:

   ```bash
   cd backend && go test ./...
   ```

   Expected: succeeds, possibly with no packages yet.

## Task 2: Add config loader tests

**Objective:** Define expected configuration behavior before implementation.

**Files:**

- Create: `backend/internal/config/config_test.go`
- Create: `backend/internal/config/config.go`

**Test cases:**

- default config uses development-safe defaults;
- environment variables override defaults;
- missing required LLM key is allowed in development but detectable via a validation method for real provider usage;
- `PRIVACY_ALLOW_EXTERNAL_MODEL_PROVIDERS=false` is parsed as false.

**RED command:**

```bash
cd backend && go test ./internal/config -v
```

Expected: fails before implementation.

## Task 3: Implement minimal config package

**Objective:** Implement the smallest config loader that satisfies tests.

**Files:**

- Modify: `backend/internal/config/config.go`

**Implementation guidance:**

Create a `Config` struct with at least:

```go
type Config struct {
    AppEnv string
    PublicURL string
    TemporalAddress string
    TemporalNamespace string
    TemporalTaskQueue string
    StorageProvider string
    TranscriptionProvider string
    LLMProvider string
    LLMBaseURL string
    LLMAPIKey string
    LLMModel string
    PrivacyAllowExternalModelProviders bool
    PrivacyDeleteOriginalAudioAfterTranscription bool
}
```

Expose:

```go
func LoadFromEnv() Config
func (c Config) ValidateForStartup() error
```

Keep validation minimal for this milestone: required Temporal task queue, provider names, and parseable booleans with defaults.

**GREEN command:**

```bash
cd backend && go test ./internal/config -v
```

Expected: pass.

## Task 4: Add API router tests

**Objective:** Define the health endpoint behavior before implementation.

**Files:**

- Create: `backend/internal/api/router_test.go`
- Create: `backend/internal/api/router.go`

**Expected behavior:**

`GET /healthz` returns:

```json
{
  "status": "ok",
  "service": "soniq-api"
}
```

with HTTP status `200` and `Content-Type: application/json`.

**RED command:**

```bash
cd backend && go test ./internal/api -v
```

Expected: fails before implementation.

## Task 5: Implement minimal API router

**Objective:** Implement the health endpoint using the Go standard library.

**Files:**

- Modify: `backend/internal/api/router.go`

**Implementation guidance:**

Expose:

```go
func NewRouter() http.Handler
```

Use `http.NewServeMux()` for now. Avoid adding chi until routing complexity justifies the dependency.

**GREEN command:**

```bash
cd backend && go test ./internal/api -v
```

Expected: pass.

## Task 6: Add API entrypoint

**Objective:** Start a local HTTP API server.

**Files:**

- Create: `backend/cmd/api/main.go`

**Implementation guidance:**

- Load config.
- Build router with `api.NewRouter()`.
- Listen on `:8080` by default.
- Log startup and fatal server errors.

**Verification:**

Run in one terminal:

```bash
cd backend && go run ./cmd/api
```

In another terminal:

```bash
curl -i http://localhost:8080/healthz
```

Expected: `200 OK` and JSON body.

## Task 7: Add worker entrypoint skeleton

**Objective:** Provide a runnable worker command that validates configuration but does not start Temporal yet.

**Files:**

- Create: `backend/cmd/worker/main.go`

**Implementation guidance:**

- Load config.
- Validate startup config.
- Log the configured Temporal address, namespace, and task queue.
- Exit cleanly after printing a clear message such as `worker skeleton ready`.

**Verification:**

```bash
cd backend && go run ./cmd/worker
```

Expected: exits successfully and prints config summary without secrets.

## Task 8: Add root Makefile quality commands

**Objective:** Standardize local commands.

**Files:**

- Create: `Makefile`

**Targets:**

```makefile
fmt:
	cd backend && go fmt ./...

lint:
	cd backend && go vet ./...

test:
	cd backend && go test ./...

api:
	cd backend && go run ./cmd/api

worker:
	cd backend && go run ./cmd/worker
```

**Verification:**

```bash
make fmt
make lint
make test
```

Expected: all pass.

## Task 9: Document local backend commands

**Objective:** Make the new developer workflow discoverable.

**Files:**

- Modify: `README.md` or create `docs/development.md`

**Content:**

Document:

```bash
make fmt
make lint
make test
make api
make worker
```

Also mention that the first backend milestone intentionally uses mock/skeleton behavior only.

## Task 10: Final verification

**Objective:** Confirm the milestone is complete and clean.

**Commands:**

```bash
git status --short
make fmt
make lint
make test
```

Manual API smoke test:

```bash
make api
curl -i http://localhost:8080/healthz
```

Expected:

- formatting/lint/tests pass;
- health endpoint returns `200 OK`;
- no unrelated files are modified.

## Commit Guidance

Suggested commits, subject to user confirmation before each commit:

```txt
chore: initialize backend go module
feat: add backend config loader
feat: add api health endpoint
chore: add backend quality commands
```

Do not commit without explicit user approval.
