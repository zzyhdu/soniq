# Local Development

This document describes the current local backend workflow for Soniq.

Soniq is currently in the Postgres-backed recording persistence milestone. The commands below intentionally run a small backend foundation:

- the API exposes `GET /healthz`;
- the API exposes recording metadata endpoints: `POST /recordings`, `GET /recordings/{id}`, and `GET /recordings/{id}/status`;
- the production API command uses Soniq Postgres for recording metadata persistence;
- `POST /recordings` invokes an injectable recording processor seam after successful creation; the production API command wires that seam to Temporal and starts `RecordingProcessingWorkflow` asynchronously;
- the worker starts a real Temporal SDK worker, registers the recording processing workflow and activity stubs, and polls the configured task queue;
- object storage, ffmpeg, ASR, LLM providers, authentication, and the web UI are not implemented in this milestone.

## Prerequisites

- Go 1.26 or newer. The backend module pins `toolchain go1.26.4`, which was the latest stable Go toolchain checked for this milestone.
- `make`.
- Optional for local Temporal/Postgres smoke testing: Docker and Docker Compose.

From the repository root, verify the backend toolchain:

```bash
make test
```

## Quality commands

Run these from the repository root.

### Format Go code

```bash
make fmt
```

This delegates to:

```bash
cd backend && go fmt ./...
```

### Run static checks

```bash
make lint
```

This delegates to:

```bash
cd backend && go vet ./...
```

### Run tests

```bash
make test
```

This delegates to:

```bash
cd backend && go test ./...
```

Before committing backend changes, run:

```bash
make fmt
make lint
make test
```

## Run the API skeleton

`make api` now builds the HTTP router with a Postgres-backed recording store and a Temporal-backed recording processor. At startup it opens Soniq Postgres and dials the configured Temporal server, so both services must be reachable before serving requests.

For local development, start the local services first:

```bash
make temporal-up
make temporal-ps
```

The local Temporal frontend listens on `localhost:7233`, and the Temporal Web UI is available at:

```txt
http://localhost:8233
```

Then start the API server:

```bash
make api
```

Default runtime configuration:

- `API_ADDRESS=:8080`
- `POSTGRES_DSN=postgres://soniq_user:***@localhost:5432/soniq?sslmode=disable`
- `TEMPORAL_ADDRESS=localhost:7233`
- `TEMPORAL_NAMESPACE=default`
- `TEMPORAL_TASK_QUEUE=soniq-audio-pipeline`

By default the API listens on `:8080`. If that port is already in use, override the address:

```bash
API_ADDRESS=:18080 make api
```

If Postgres or Temporal is not reachable, `make api` fails during startup. Unit tests do not require running Postgres or Temporal services; command wiring is covered by injected fakes.

Verify the health endpoint in another terminal:

```bash
curl -i http://localhost:8080/healthz
```

If you used a custom port:

```bash
curl -i http://localhost:18080/healthz
```

Expected response:

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{"status":"ok","service":"soniq-api"}
```

## Use the Recording API skeleton

The recording endpoints now persist metadata in Soniq Postgres in the production API path. Records survive API process restarts as long as the local Postgres volume remains intact. This skeleton does not upload audio, write objects to storage, or perform real audio processing.

After a recording is created successfully in Postgres, the API calls an injectable `RecordingProcessor` seam. In the production API command, that seam is wired to a Temporal-backed processor that starts `RecordingProcessingWorkflow` asynchronously with workflow ID `recording-processing-<recording_id>` on `TEMPORAL_TASK_QUEUE`. The HTTP response returns the newly created metadata record; it does not wait for workflow completion.

### Manual local Temporal smoke flow

This is a manual local development smoke flow, not a CI requirement. It assumes Docker is available and uses the local Temporal stack from `compose.temporal.yml`.

Start Temporal and Soniq Postgres and confirm the services are running:

```bash
make temporal-up
make temporal-ps
```

Open the Temporal Web UI:

```txt
http://localhost:8233
```

Start the worker in one terminal using the default task queue:

```bash
TEMPORAL_TASK_QUEUE=soniq-audio-pipeline make worker
```

Start the API on a local test port in another terminal:

```bash
API_ADDRESS=:18080 make api
```

Create a recording metadata record from another terminal:

```bash
curl -i -X POST http://localhost:18080/recordings \
  -H 'Content-Type: application/json' \
  -d '{"title":"Weekly sync","workflow_type":"meeting","language":"en"}'
```

Expected response:

```http
HTTP/1.1 201 Created
Content-Type: application/json
```

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

Save the returned `id`, then inspect the matching workflow execution in the Temporal Web UI:

```txt
recording-processing-<recording_id>
```

Fetch the full recording:

```bash
curl -i http://localhost:18080/recordings/<id>
```

Because the production API path now uses Postgres, this lookup should still work after restarting only the API process, provided the local Postgres service and volume are still running.

Fetch just the recording status:

```bash
curl -i http://localhost:18080/recordings/<id>/status
```

Expected status body:

```json
{"id":"rec_...","status":"uploaded"}
```

The initial supported `workflow_type` values are:

```txt
memo
meeting
lecture
interview
```

`workflow_type` is the processing template selector, not a user tag. Future milestones may add custom templates or more specialized workflow types.

When you finish the manual smoke flow, stop the local Temporal stack:

```bash
make temporal-down
```

## Apply recording migrations locally

The local Soniq Postgres service is separate from Temporal's internal Postgres service. Apply Soniq application migrations to the `soniq` database only.

Start the local services:

```bash
make temporal-up
```

Apply the current recording migration:

```bash
docker compose -f compose.temporal.yml exec -T soniq-postgresql \
  psql -U soniq_user -d soniq -v ON_ERROR_STOP=1 \
  -f - < backend/migrations/0001_create_recordings.up.sql
```

For local reset/testing, the matching down migration is:

```bash
docker compose -f compose.temporal.yml exec -T soniq-postgresql \
  psql -U soniq_user -d soniq -v ON_ERROR_STOP=1 \
  -f - < backend/migrations/0001_create_recordings.down.sql
```

Do not apply Soniq migrations to `temporal-postgresql`; Temporal owns that database and its schema.

## Run the Temporal worker skeleton

`make worker` starts a Temporal SDK worker and blocks while polling the configured task queue. It requires a reachable Temporal server at runtime.

For local development, start Temporal first, then run:

```bash
make worker
```

Default runtime configuration:

- `TEMPORAL_ADDRESS=localhost:7233`
- `TEMPORAL_NAMESPACE=default`
- `TEMPORAL_TASK_QUEUE=soniq-audio-pipeline`

Expected behavior:

- load environment configuration;
- validate minimal startup configuration;
- connect to Temporal;
- register `RecordingProcessingWorkflow`;
- register the recording processing activity stubs;
- poll the configured task queue until interrupted;
- do not print secrets such as API keys.

Example startup output:

```txt
starting temporal worker
temporal_address=localhost:7233
temporal_namespace=default
temporal_task_queue=soniq-audio-pipeline
```

If Temporal is not reachable, `make worker` fails during startup. Unit tests do not require a running Temporal server; worker registration is covered by an in-process registry spy.

## Temporal workflow skeleton boundaries

The current Temporal implementation is intentionally a skeleton:

- The workflow is implemented with the real Temporal Go SDK and covered by the Temporal SDK testsuite.
- Activity implementations are stubs that validate input and model recording status transitions only.
- The API calls an injectable recording processor seam after `POST /recordings`; the production API command wires that seam to a Temporal client and starts `RecordingProcessingWorkflow` asynchronously.
- Worker startup is the boundary where the code leaves in-process tests and requires a real Temporal server.

The skeleton does not yet perform audio processing, storage writes, ASR, LLM summarization, Postgres persistence, provider webhooks, or production Temporal smoke testing. Those integrations should be added as separate milestones with explicit local service configuration.

## Configuration

Start from the example environment file:

```bash
cp .env.example .env
```

The current backend reads environment variables directly. Important local settings include:

| Variable | Default | Purpose |
|---|---:|---|
| `APP_ENV` | `development` | Runtime environment name. |
| `APP_PUBLIC_URL` | `http://localhost:8080` | Public API URL used by clients and links. |
| `API_ADDRESS` | `:8080` | Local HTTP listen address for `make api`. |
| `POSTGRES_DSN` | `postgres://soniq_user:***@localhost:5432/soniq?sslmode=disable` | Soniq application database used by `make api` for recording metadata persistence. |
| `TEMPORAL_ADDRESS` | `localhost:7233` | Temporal server address used by `make api` and `make worker`. |
| `TEMPORAL_NAMESPACE` | `default` | Temporal namespace used by `make api` and `make worker`. |
| `TEMPORAL_TASK_QUEUE` | `soniq-audio-pipeline` | Task queue used when the API starts workflows and the worker polls work. |
| `STORAGE_PROVIDER` | `s3_compatible` | Future storage provider selector. |
| `TRANSCRIPTION_PROVIDER` | `faster_whisper` | Future transcription provider selector. |
| `LLM_PROVIDER` | `openai_compatible` | Future LLM provider selector. |
| `LLM_BASE_URL` | `https://api.openai.com/v1` | Future OpenAI-compatible endpoint. |
| `LLM_MODEL` | `gpt-4o-mini` | Future default LLM model name. |

Do not commit real secrets. Keep real API keys and credentials in local environment files only.

## Current milestone boundaries

The current backend foundation is intentionally small. It provides:

- a Go module under `backend/`;
- config loading and validation;
- a standard-library HTTP router;
- `GET /healthz`;
- Postgres-backed recording metadata endpoints;
- SQL migrations for the `recordings` table;
- API and Temporal worker command entrypoints;
- a Temporal-backed recording processor that starts `RecordingProcessingWorkflow` after successful `POST /recordings` requests;
- a Temporal SDK recording processing workflow skeleton;
- activity stubs for validation and recording status transitions;
- root `Makefile` quality commands.

It does not yet provide:

- durable recording persistence is currently limited to recording metadata;
- real recording audio upload handling;
- production Temporal smoke-test configuration;
- MinIO/S3 storage integration;
- ffmpeg audio processing;
- ASR or LLM provider calls;
- authentication/RBAC;
- frontend UI.
