# Temporal Local Development Environment Implementation Plan

> **For Hermes:** Use test-driven-development and execute this plan task-by-task. Keep each task small: RED → GREEN → verify → report → wait for user commit confirmation. During this planning task, do not modify runtime/source files beyond this plan document and task-board metadata.

**Goal:** Add a local Temporal development environment so Soniq developers can run Temporal server, the worker, the API, and manually verify `POST /recordings` starts and completes the recording workflow skeleton.

**Architecture:** Use Docker Compose for local Temporal services and keep Soniq application commands in the root `Makefile`. The API and worker continue to read `TEMPORAL_ADDRESS`, `TEMPORAL_NAMESPACE`, and `TEMPORAL_TASK_QUEUE`; local services provide Temporal at `localhost:7233` and the Temporal Web UI at `localhost:8233`. This milestone adds local infrastructure and manual smoke verification only; it does not add production deployment config.

**Tech Stack:** Docker 29.5.2, Docker Compose v5.1.4, Temporal server image `temporalio/auto-setup:1.29.6.1` (latest visible Docker Hub tag checked 2026-06-05), Temporal UI image `temporalio/ui:2.51.0` (latest visible version tag checked 2026-06-05), Go 1.24+, Temporal Go SDK `v1.44.1`.

---

## Current state

Already implemented:

- `cmd/api` dials Temporal at startup and injects `TemporalRecordingProcessor`.
- `POST /recordings` starts `RecordingProcessingWorkflow` asynchronously.
- `cmd/worker` starts a real Temporal SDK worker and registers the workflow/activity stubs.
- Unit tests do not require a Temporal server.

Current gap:

```txt
make api
make worker
```

now require a reachable Temporal server, but the repository does not yet provide a local Temporal server setup.

Target after this plan:

```bash
make temporal-up
make worker
API_ADDRESS=:18080 make api
curl -X POST http://localhost:18080/recordings ...
make temporal-down
```

Developers should be able to inspect workflow executions in Temporal Web UI:

```txt
http://localhost:8233
```

---

## Important boundaries

Do not add in this milestone:

- Production Temporal deployment configuration.
- Postgres-backed Soniq persistence.
- MinIO/S3 audio upload.
- ffmpeg, ASR, LLM providers, provider webhooks.
- Automated CI smoke tests that require Docker or Temporal to be running.
- Hermes/Kanban references in repository docs.

Unit tests must remain hermetic and must pass with:

```bash
cd backend && go test ./...
```

without Docker or Temporal running.

---

## Version notes

Version discovery performed during planning:

```bash
docker --version
# Docker version 29.5.2

docker compose version
# Docker Compose version v5.1.4
```

Docker Hub latest visible tags checked by API on 2026-06-05:

- `temporalio/auto-setup`: latest visible version tag `1.29.6.1`
- `temporalio/ui`: latest visible version tag `2.51.0`

Use pinned image tags rather than `latest` so local dev is reproducible.

---

## Task B1: Add local Temporal Docker Compose config

**Objective:** Add a minimal local Temporal server and Temporal Web UI configuration.

**Files:**

- Create: `compose.temporal.yml`

**Expected service shape:**

Use Temporal auto-setup with embedded SQLite or a simple local development DB mode if supported by the selected image. Prefer the smallest setup that exposes:

- Temporal frontend: `localhost:7233`
- Temporal Web UI: `localhost:8233`

Candidate service structure:

```yaml
services:
  temporal:
    image: temporalio/auto-setup:1.29.6.1
    ports:
      - "7233:7233"
    environment:
      - DB=sqlite
      - TEMPORAL_ADDRESS=temporal:7233

  temporal-ui:
    image: temporalio/ui:2.51.0
    depends_on:
      - temporal
    ports:
      - "8233:8080"
    environment:
      - TEMPORAL_ADDRESS=temporal:7233
      - TEMPORAL_CORS_ORIGINS=http://localhost:3000
```

If `temporalio/auto-setup` does not support the intended embedded DB mode, adjust to the smallest documented local dev setup and explain the reason in the task report.

**Verification:**

Run:

```bash
docker compose -f compose.temporal.yml config
```

Expected:

- command exits successfully;
- rendered config includes `temporal` and `temporal-ui` services;
- no application source files are modified.

**Suggested commit:**

```txt
chore: add local temporal compose config
```

---

## Task B2: Add Makefile targets for Temporal local dev

**Objective:** Add project-level commands for starting, stopping, and inspecting the local Temporal stack.

**Files:**

- Modify: `Makefile`

**Targets to add:**

```make
.PHONY: temporal-up temporal-down temporal-logs temporal-ps

temporal-up:
	docker compose -f compose.temporal.yml up -d

temporal-down:
	docker compose -f compose.temporal.yml down

temporal-logs:
	docker compose -f compose.temporal.yml logs -f temporal temporal-ui

temporal-ps:
	docker compose -f compose.temporal.yml ps
```

Keep existing targets unchanged.

**Verification:**

Run:

```bash
make temporal-ps
make test
```

Expected:

- `make temporal-ps` can run even if services are not running;
- `make test` still passes and does not require Docker services.

**Suggested commit:**

```txt
chore: add temporal local dev make targets
```

---

## Task B3: Document local Temporal end-to-end skeleton flow

**Objective:** Update local development docs so developers know how to run Temporal, worker, API, and manual workflow start verification.

**Files:**

- Modify: `docs/development.md`

**Docs should include:**

1. Docker and Docker Compose as optional prerequisites for local Temporal smoke testing.
2. Start Temporal:

   ```bash
   make temporal-up
   make temporal-ps
   ```

3. Open Temporal Web UI:

   ```txt
   http://localhost:8233
   ```

4. Start worker in one terminal:

   ```bash
   make worker
   ```

5. Start API in another terminal:

   ```bash
   API_ADDRESS=:18080 make api
   ```

6. Create recording:

   ```bash
   curl -i -X POST http://localhost:18080/recordings \
     -H 'Content-Type: application/json' \
     -d '{"title":"Weekly sync","workflow_type":"meeting","language":"en"}'
   ```

7. Inspect workflow ID in Temporal UI:

   ```txt
   recording-processing-<recording_id>
   ```

8. Stop services:

   ```bash
   make temporal-down
   ```

Clarify that this is a manual local smoke flow, not a CI requirement.

**Verification:**

Run:

```bash
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
docs: document local temporal smoke flow
```

---

## Task B4: Add optional manual smoke helper script

**Objective:** Add a small script that performs the HTTP `POST /recordings` request and prints the expected Temporal workflow ID for manual verification.

**Files:**

- Create: `scripts/smoke-recording-workflow.sh`
- Modify: `Makefile` only if adding a convenience target such as `smoke-recording-workflow`

**Script behavior:**

```bash
#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://localhost:18080}"
response="$(curl -fsS -X POST "$API_URL/recordings" \
  -H 'Content-Type: application/json' \
  -d '{"title":"Weekly sync","workflow_type":"meeting","language":"en"}')"

printf '%s\n' "$response"
recording_id="$(printf '%s\n' "$response" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
printf 'Expected Temporal workflow ID: recording-processing-%s\n' "$recording_id"
```

Do not make this script part of automated tests. It is a manual helper that assumes API and Temporal are already running.

**Verification:**

Run static checks only unless the user explicitly wants a live smoke test:

```bash
bash -n scripts/smoke-recording-workflow.sh
cd backend && go test ./...
```

If live services are running and the user asks for a manual smoke test, run:

```bash
API_URL=http://localhost:18080 scripts/smoke-recording-workflow.sh
```

**Suggested commit:**

```txt
chore: add recording workflow smoke helper
```

---

## Final verification checklist

Before considering the milestone complete:

- [ ] `docker compose -f compose.temporal.yml config` succeeds.
- [ ] `make temporal-up` starts Temporal server and UI.
- [ ] `make temporal-ps` shows services.
- [ ] `make worker` connects to Temporal and polls `soniq-audio-pipeline`.
- [ ] `API_ADDRESS=:18080 make api` connects to Temporal and serves HTTP.
- [ ] `POST /recordings` returns `201 Created`.
- [ ] Temporal UI shows workflow ID `recording-processing-<recording_id>`.
- [ ] Workflow reaches completed status with current activity stubs.
- [ ] `cd backend && go test ./...` passes without Temporal running.
- [ ] Docs clearly separate manual local smoke tests from CI/unit tests.
