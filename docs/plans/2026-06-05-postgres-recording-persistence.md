# Postgres Recording Persistence Implementation Plan

> **For Hermes:** Use test-driven-development and execute this plan task-by-task. Keep each implementation task small: RED → GREEN → verify → report → wait for user commit confirmation. Keep repository docs tool-agnostic; do not describe local task-board tooling as part of Soniq architecture.

**Goal:** Replace the production API's in-memory recording metadata store with durable Postgres-backed persistence while keeping unit tests hermetic and preserving the current API behavior.

**Architecture:** Add a Soniq-owned local Postgres service that is separate from Temporal's internal database. Introduce migrations for the Soniq `recordings` table, load `POSTGRES_DSN` from config, and add a store interface so the HTTP router can use either the existing in-memory store in tests or a Postgres-backed store in production. Wire `cmd/api` to open a Postgres connection and pass the Postgres store into the existing router/processor path.

**Tech Stack:** Go 1.24+, Postgres `18.4-alpine` for Soniq local application data (latest visible Docker Hub tag checked 2026-06-05), `github.com/jackc/pgx/v5 v5.10.0` (latest visible Go module version checked 2026-06-05), existing Temporal Go SDK `v1.44.1`.

---

## Current state

Already implemented:

- `POST /recordings` stores metadata in the in-memory recording store.
- `GET /recordings/{id}` and `GET /recordings/{id}/status` read from the in-memory store.
- Production `cmd/api` starts `RecordingProcessingWorkflow` after successful recording creation.
- Local Temporal smoke verifies API → Temporal → worker → completed workflow skeleton.

Current gap:

```txt
API restart
  ↓
recording metadata disappears
```

Target after this plan:

```txt
POST /recordings
  ↓
insert row into Soniq Postgres recordings table
  ↓
start Temporal workflow
  ↓
GET /recordings/{id} still works after API process restart
```

---

## Database boundary decision

Temporal already has a Postgres container in `compose.temporal.yml`, but that database is Temporal-owned infrastructure state.

Recommended boundary:

- **Temporal Postgres:** stores Temporal namespaces, workflow histories, task queues, visibility metadata, and internal service state. Temporal owns its schema and migrations.
- **Soniq Postgres:** stores Soniq application data such as recordings, artifacts, transcripts, summaries, users, workspaces, and audit logs. Soniq owns its schema and migrations.

For local development, both services can run in the same Docker Compose project and network, but they should be separate services/databases by default.

Why not share the same database service for the first implementation?

1. **Schema ownership:** Temporal manages its own schema and migration lifecycle; Soniq should not couple application migrations to Temporal internals.
2. **Blast radius:** resetting or upgrading Temporal local state should not delete Soniq recording metadata, and vice versa.
3. **Credentials and permissions:** Temporal credentials should not automatically grant access to application tables.
4. **Production parity:** managed Temporal/Temporal Cloud often will not expose Temporal's database at all, so Soniq needs its own database boundary.
5. **Clarity:** developers can reason about `temporal-postgresql` as infrastructure and `soniq-postgresql` as application persistence.

Acceptable alternative for a throwaway prototype:

- Use one Postgres server with two separate databases and two users.

Even then, keep Temporal and Soniq schemas/databases separate. Do not put Soniq tables into Temporal's database.

---

## Important boundaries

Do not add in this milestone:

- Object storage or audio upload.
- Real ffmpeg/ASR/LLM processing.
- WorkflowRun persistence unless explicitly needed by a later task.
- Authentication, workspaces, users, or RBAC.
- CI tests that require Docker or a live Postgres server.
- Reuse of Temporal's internal database for Soniq application tables.

Unit tests must remain hermetic. Integration tests that touch Postgres should be opt-in or use fakes unless the user explicitly approves live Docker-backed tests.

---

## Task C1: Add Soniq Postgres local service

**Objective:** Add a separate local Postgres service for Soniq application data.

**Files:**

- Modify: `compose.temporal.yml` or create a dedicated local compose file if cleaner.
- Modify: `.env.example` if the local DSN changes.
- Modify: `Makefile` if adding convenience targets.

**Implementation notes:**

Prefer a separate service:

```yaml
soniq-postgresql:
  image: postgres:18.4-alpine
  environment:
    POSTGRES_USER: soniq
    POSTGRES_PASSWORD: soniq
    POSTGRES_DB: soniq
  ports:
    - "5432:5432"
  healthcheck:
    test: ["CMD-SHELL", "pg_isready -U soniq"]
    interval: 5s
    timeout: 5s
    retries: 10
  volumes:
    - soniq-postgres-data:/var/lib/postgresql/data
```

Use `POSTGRES_DSN=postgres://soniq:soniq@localhost:5432/soniq?sslmode=disable` for local development.

If port `5432` is already taken, use a project-specific host port such as `55432` and update `.env.example`/docs accordingly.

**Verification:**

Run:

```bash
docker compose -f compose.temporal.yml config
make temporal-ps
```

If services are running and the user approves live checks:

```bash
make temporal-up
docker compose -f compose.temporal.yml exec -T soniq-postgresql pg_isready -U soniq
```

**Suggested commit:**

```txt
chore: add local soniq postgres service
```

---

## Task C2: Add recording migrations

**Objective:** Add SQL migrations for the `recordings` table that match the current API/domain model subset.

**Files:**

- Create: `backend/migrations/0001_create_recordings.up.sql`
- Create: `backend/migrations/0001_create_recordings.down.sql`

**Schema:**

Start with only fields currently used by the API/domain skeleton:

```sql
CREATE TABLE recordings (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  workflow_type TEXT NOT NULL,
  language TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
```

Add useful constraints:

```sql
ALTER TABLE recordings
  ADD CONSTRAINT recordings_status_check
  CHECK (status IN ('uploaded', 'processing', 'transcribing', 'summarizing', 'completed', 'failed', 'cancelled'));

ALTER TABLE recordings
  ADD CONSTRAINT recordings_workflow_type_check
  CHECK (workflow_type IN ('memo', 'meeting', 'lecture', 'interview'));
```

Do not add workspace/user/artifact columns until those features are implemented.

**Verification:**

Run static SQL review and, if a local Postgres service is running, apply manually with `psql` or a migration tool. Keep automated unit tests independent of Postgres.

**Suggested commit:**

```txt
chore: add recordings table migration
```

---

## Task C3: Add Postgres DSN config

**Objective:** Load and validate `POSTGRES_DSN` so production API wiring can connect to Soniq Postgres.

**Files:**

- Modify: `backend/internal/config/config.go`
- Modify: `backend/internal/config/config_test.go`
- Modify: `.env.example` if needed

**TDD steps:**

1. Add config tests asserting:
   - default `PostgresDSN` is `postgres://soniq:soniq@localhost:5432/soniq?sslmode=disable` or the chosen local DSN;
   - env override works;
   - startup validation rejects empty `POSTGRES_DSN` once production API depends on it.
2. Run targeted tests and watch RED.
3. Add `PostgresDSN string` to `Config` and load it from `POSTGRES_DSN`.
4. Update `ValidateForStartup` only if the API path will require it immediately; avoid breaking worker-only startup if worker does not yet need Postgres.

**Verification:**

```bash
cd backend && go test ./internal/config -v
cd backend && go test ./...
```

**Suggested commit:**

```txt
feat: add postgres dsn config
```

---

## Task C4: Introduce recording store interface for API wiring

**Objective:** Define the small store behavior the API needs so in-memory and Postgres stores can be swapped without changing handlers.

**Files:**

- Modify: `backend/internal/api/router.go`
- Modify: `backend/internal/api/recordings_test.go` if needed
- Possibly modify: `backend/internal/recordings/store.go`

**Interface shape:**

```go
type RecordingStore interface {
  Create(recordings.CreateRecordingInput) (domain.Recording, error)
  Get(id string) (domain.Recording, bool)
}
```

Keep `NewRouter()` and `NewRouterWithStore(...)` compatibility where possible. Existing tests should keep using `recordings.NewMemoryStore()`.

**Verification:**

```bash
cd backend && go test ./internal/api -v
cd backend && go test ./...
```

**Suggested commit:**

```txt
refactor: add recording store interface
```

---

## Task C5: Add Postgres recording store tests RED

**Objective:** Specify Postgres-backed recording store behavior before implementation.

**Files:**

- Create: `backend/internal/recordings/postgres_store_test.go`

**Test strategy:**

Prefer hermetic tests with a fake `pgx`-style executor if the store is designed around a small DB interface, or use opt-in live integration tests guarded by an environment variable such as `SONIQ_POSTGRES_TEST_DSN`.

Minimum behaviors to cover:

- `Create` inserts a recording with generated id, `uploaded` status, workflow type, language, timestamps.
- `Get` returns a persisted recording by id.
- `Get` returns false for missing id.
- invalid workflow type is rejected before insert.

Expected RED examples:

```txt
undefined: NewPostgresStore
undefined: PostgresStore
```

**Verification:**

```bash
cd backend && go test ./internal/recordings -run Postgres -v
```

Expected: fails for missing implementation, not syntax errors.

**Suggested commit:**

```txt
test: add postgres recording store red tests
```

---

## Task C6: Implement Postgres recording store

**Objective:** Add a Postgres-backed implementation of recording create/get behavior.

**Files:**

- Create: `backend/internal/recordings/postgres_store.go`
- Modify: `backend/go.mod`, `backend/go.sum` as driven by imports

**Implementation notes:**

Use `github.com/jackc/pgx/v5` / `pgxpool` through a small interface if that keeps tests simple:

```go
type DB interface {
  QueryRow(ctx context.Context, sql string, args ...any) Row
}
```

or use `pgxpool.Pool` directly if tests use opt-in integration.

Do not add a standalone dependency-only commit. Let the first implementation/test import drive `go mod tidy`.

**Verification:**

```bash
cd backend && go test ./internal/recordings -v
cd backend && go test ./...
```

Explain any new `go.mod`/`go.sum` indirect dependencies in the task report.

**Suggested commit:**

```txt
feat: add postgres recording store
```

---

## Task C7: Wire production API to Postgres store

**Objective:** Use the Postgres recording store in `cmd/api` while preserving hermetic command tests via injectable factories.

**Files:**

- Modify: `backend/cmd/api/main.go`
- Modify: `backend/cmd/api/main_test.go`

**TDD steps:**

1. Add a test that `buildHandler(...)` opens/closes a fake Postgres store or connection factory and still injects a Temporal-backed processor.
2. Verify RED for missing DB factory seam.
3. Add a `postgresStoreFactory`/connection seam.
4. Wire production API to open Postgres using `cfg.PostgresDSN` and pass `recordings.NewPostgresStore(...)` to `api.NewRouterWithProcessor(...)`.
5. Ensure cleanup closes both Temporal client and Postgres pool.

**Verification:**

```bash
cd backend && go test ./cmd/api -v
cd backend && go test ./...
```

If local services are running and migrations applied, optionally run a manual API smoke.

**Suggested commit:**

```txt
feat: wire api to postgres recording store
```

---

## Task C8: Document Postgres persistence and local migration flow

**Objective:** Update docs to describe local Soniq Postgres, migration application, and persistence boundaries.

**Files:**

- Modify: `docs/development.md`
- Modify: `docs/infrastructure.md`
- Possibly modify: `docs/roadmap.md`

**Docs should cover:**

- Temporal Postgres and Soniq Postgres are separate by ownership.
- `POSTGRES_DSN` points to Soniq application data.
- How to start local services.
- How to apply migrations manually.
- Unit tests do not require Postgres.
- API restart should preserve recording metadata once using Postgres.

**Verification:**

```bash
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
docs: document postgres recording persistence
```

---

## Final verification checklist

Before considering the milestone complete:

- [ ] Local Soniq Postgres service starts independently from Temporal internals.
- [ ] Migrations create and drop the `recordings` table.
- [ ] `POSTGRES_DSN` is documented and loaded by config.
- [ ] API tests remain hermetic.
- [ ] Postgres store behavior is tested.
- [ ] Production `cmd/api` uses Postgres store.
- [ ] `POST /recordings` persists metadata durably.
- [ ] `GET /recordings/{id}` and `/status` work after API restart.
- [ ] API still starts Temporal workflow after successful Postgres insert.
- [ ] `cd backend && go test ./...` passes without live Postgres.
