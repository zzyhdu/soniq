# Postgres + Temporal Smoke Verification Implementation Plan

> **For Hermes:** Execute this plan task-by-task. Keep repository docs tool-agnostic and use small commits. Ask before every commit.

**Goal:** Make the local Postgres + Temporal + API + worker path easy to verify after the production API was wired to Postgres.

**Architecture:** Keep infrastructure services (`soniq-postgresql`, `temporal-postgresql`, `temporal`, `temporal-ui`) in Docker Compose. For now, keep API and worker runnable via `make api` / `make worker`, then add a smoke script that orchestrates migration, process startup, `POST /recordings`, persistence verification across API restart, and Temporal workflow completion. Optionally add API/worker Compose services later behind a development profile once the project has a backend Dockerfile.

**Tech Stack:** Docker Compose, Go 1.26.4 toolchain, Postgres 18.4, Temporal 1.29.6.1, Temporal UI 2.51.0.

---

## Answer: can these services live in one Compose file?

Yes. In fact, the local infrastructure already lives in one Compose file:

```txt
compose.temporal.yml
  soniq-postgresql
  temporal-postgresql
  temporal
  temporal-ui
```

We can also add `api` and `worker` services to the same Compose project, but that requires one of these approaches:

1. **Add a backend Dockerfile** and run API/worker as containers.
2. **Use Docker Compose `develop` / bind mounts** for local source code.
3. **Keep API/worker as local `make` commands** and use a script to start/stop them for smoke verification.

Recommendation for this D milestone:

- Keep infrastructure in Compose.
- Do not add backend containerization yet.
- Add a smoke verification script that starts infrastructure, applies migration, starts API/worker as local background processes, runs the smoke flow, verifies persistence after API restart, verifies Temporal completion, then cleans up local app processes.

Reason: this gives us the verification loop immediately without prematurely designing container packaging for the backend. Once frontend/storage enters the picture, we can add a dedicated full-stack compose profile.

---

## Task D1: Add Postgres + Temporal smoke plan

**Objective:** Save this plan and create parked work items for the smoke verification milestone.

**Files:**

- Create: `docs/plans/2026-06-06-postgres-temporal-smoke.md`

**Verification:**

```bash
git status --short
```

Only the plan file and task-board metadata should change.

**Suggested commit:**

```txt
docs: add postgres temporal smoke plan
```

---

## Task D2: Add smoke helper script for full local verification

**Objective:** Add one command that verifies production-path persistence and Temporal workflow start/completion.

**Files:**

- Create: `scripts/smoke-postgres-temporal.sh`
- Optionally modify: `Makefile`

**Script responsibilities:**

1. Start infra:

   ```bash
   make temporal-up
   ```

2. Wait for Soniq Postgres:

   ```bash
   docker compose -f compose.temporal.yml exec -T soniq-postgresql pg_isready -U soniq_user
   ```

3. Apply migration:

   ```bash
   docker compose -f compose.temporal.yml exec -T soniq-postgresql \
     psql -U soniq_user -d soniq -v ON_ERROR_STOP=1 \
     -f - < backend/migrations/0001_create_recordings.up.sql
   ```

4. Start worker and API as local background processes using temporary log files.
5. Wait for API `/healthz`.
6. Run `scripts/smoke-recording-workflow.sh` against `http://localhost:8080`.
7. Extract `recording_id`.
8. Verify `GET /recordings/<id>` returns the row.
9. Restart only the API process.
10. Verify `GET /recordings/<id>` still works after API restart.
11. Verify Temporal workflow completed using Temporal CLI inside the `temporal` container:

    ```bash
    docker compose -f compose.temporal.yml exec -T temporal \
      temporal --address temporal:7233 workflow describe \
      --namespace default \
      --workflow-id recording-processing-<recording_id>
    ```

12. Stop only locally started API/worker processes; leave infra running unless `SMOKE_DOWN=1` is set.

**Important behavior:**

- Script should fail fast with clear messages.
- Script should not delete local Postgres data by default.
- Script should not run in CI by default.
- Script should print log file paths for API/worker failures.

**Verification:**

```bash
bash -n scripts/smoke-postgres-temporal.sh
cd backend && go test ./...
```

If local Docker is available, run:

```bash
scripts/smoke-postgres-temporal.sh
```

**Suggested commit:**

```txt
chore: add postgres temporal smoke script
```

---

## Task D3: Add Makefile target for full smoke

**Objective:** Add a convenient Makefile target that runs the full smoke script.

**Files:**

- Modify: `Makefile`

**Target:**

```make
.PHONY: smoke-postgres-temporal

smoke-postgres-temporal:
	./scripts/smoke-postgres-temporal.sh
```

**Verification:**

```bash
make smoke-postgres-temporal
```

**Suggested commit:**

```txt
chore: add postgres temporal smoke make target
```

---

## Task D4: Document full local smoke workflow

**Objective:** Update local development docs so the user can run one command instead of manually opening several services.

**Files:**

- Modify: `docs/development.md`

**Docs should explain:**

- `make temporal-up` starts infrastructure services in one Compose project.
- `make smoke-postgres-temporal` verifies the full backend path.
- The script starts API/worker locally for the duration of the smoke test.
- Full containerized API/worker can be added later when a backend Dockerfile/profile is introduced.

**Verification:**

```bash
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
docs: document postgres temporal smoke workflow
```

---

## Future option: one Compose file for everything

When we are ready to run API and worker inside Compose, add:

- `backend/Dockerfile`
- `api` service
- `worker` service
- optional `migrate` one-shot service
- Compose `profiles: [app]` or a separate `compose.local.yml`

A future command could become:

```bash
docker compose -f compose.temporal.yml --profile app up
```

For now, the smoke script is the smaller and safer step.
