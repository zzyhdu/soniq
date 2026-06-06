# Local Development

This document describes the current local backend workflow for Soniq.

Soniq is currently in the local audio-upload and Postgres-backed recording persistence milestone. The commands below intentionally run a small backend foundation:

- the API exposes `GET /healthz`;
- the API exposes recording endpoints: `POST /recordings`, `POST /recordings/upload`, `GET /recordings/{id}`, and `GET /recordings/{id}/status`;
- the production API command uses Soniq Postgres for recording metadata persistence;
- `POST /recordings` creates metadata-only recordings; `POST /recordings/upload` accepts multipart audio, writes the original audio through an object-store seam, persists audio metadata, and then invokes the same injectable recording processor seam;
- the production API command wires the recording processor seam to Temporal and starts `RecordingProcessingWorkflow` asynchronously;
- the worker starts a real Temporal SDK worker, registers the recording processing workflow and Soniq Postgres-backed recording status activities, and polls the configured task queue;
- local filesystem object storage is implemented for development; S3-compatible storage, ffmpeg, ASR, LLM providers, authentication, and the web UI are not implemented in this milestone.

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

## Full local smoke verification

To avoid opening several terminals manually, run the full smoke target from the repository root:

```bash
make smoke-postgres-temporal
```

This target runs `scripts/smoke-postgres-temporal.sh`. The script starts the Compose infrastructure, applies the recording migrations if needed, starts the API and worker as temporary local background processes, uploads a small audio file through `POST /recordings/upload`, verifies the local object file and Postgres audio metadata, verifies the recording can be read from Postgres before and after an API restart, confirms the Temporal workflow reaches `COMPLETED`, and then stops the API/worker processes it started.

The script intentionally leaves the Compose infrastructure running by default so local Postgres and Temporal state remain available for follow-up debugging. To stop Compose services after the smoke run, set:

```bash
SMOKE_DOWN=1 make smoke-postgres-temporal
```

If an API is already listening on `localhost:8080`, the script refuses to run because it needs to own API startup and restart during the persistence check. Stop the existing API process first, or run the smoke flow on a different local port:

```bash
API_URL=http://localhost:18080 API_ADDRESS=:18080 make smoke-postgres-temporal
```

## Run the API skeleton

`make api` now builds the HTTP router with a Postgres-backed recording store, a local object store, and a Temporal-backed recording processor. At startup it opens Soniq Postgres and dials the configured Temporal server, so both services must be reachable before serving requests.

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
- `STORAGE_PROVIDER=local`
- `LOCAL_STORAGE_PATH=var/uploads`

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

## Use the Recording API

The recording endpoints now persist metadata in Soniq Postgres in the production API path. Records survive API process restarts as long as the local Postgres volume remains intact. Audio-backed recordings also write the original uploaded file under `LOCAL_STORAGE_PATH` through the local object-store provider.

After a recording is created successfully in Postgres, the API calls an injectable `RecordingProcessor` seam. In the production API command, that seam is wired to a Temporal-backed processor that starts `RecordingProcessingWorkflow` asynchronously with workflow ID `recording-processing-<recording_id>` on `TEMPORAL_TASK_QUEUE`. The HTTP response returns the newly created metadata record; it does not wait for workflow completion. The worker consumes that workflow from the same task queue, uses Soniq Postgres-backed activities, and updates the recording status from `uploaded` to `processing` and then to `completed`. If the completion activity fails, the workflow schedules a best-effort `failed` status update before returning the original error.

### Upload an audio-backed recording

Use `POST /recordings/upload` for recordings that include an original audio file:

```bash
printf 'demo audio bytes' > /tmp/soniq-demo.wav

curl -i -X POST http://localhost:8080/recordings/upload \
  -F 'title=Weekly sync' \
  -F 'workflow_type=meeting' \
  -F 'language=en' \
  -F 'audio=@/tmp/soniq-demo.wav;type=audio/wav'
```

If you started the API on a custom port, use that port instead:

```bash
curl -i -X POST http://localhost:18080/recordings/upload \
  -F 'title=Weekly sync' \
  -F 'workflow_type=meeting' \
  -F 'language=en' \
  -F 'audio=@/tmp/soniq-demo.wav;type=audio/wav'
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
  "audio_object_key": "recordings/.../soniq-demo.wav",
  "audio_content_type": "audio/wav",
  "audio_size_bytes": 16,
  "created_at": "...",
  "updated_at": "..."
}
```

For the local provider, the uploaded file is stored at:

```txt
<LOCAL_STORAGE_PATH>/<audio_object_key>
```

With the default configuration, that means files are written under:

```txt
var/uploads/recordings/...
```

The `var/` directory is ignored by git because it contains local runtime artifacts.

### Create a metadata-only recording

Use `POST /recordings` when you want to exercise the metadata and Temporal enqueue path without uploading an audio file:

### Manual local Temporal smoke flow

This is the manual version of the full local smoke verification above. Use it when you want to inspect each process yourself in separate terminals. For routine checks, prefer `make smoke-postgres-temporal`.

It assumes Docker is available and uses the local Temporal/Postgres stack from `compose.temporal.yml`.

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

Expected status body before the worker has completed the workflow:

```json
{"id":"rec_...","status":"uploaded"}
```

After the worker has processed the workflow successfully, the same endpoint should return:

```json
{"id":"rec_...","status":"completed"}
```

The current workflow status path is:

```txt
uploaded -> processing -> completed
```

If completion fails, the workflow attempts a best-effort transition to:

```txt
failed
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

Apply the current recording migrations:

```bash
docker compose -f compose.temporal.yml exec -T soniq-postgresql \
  psql -U soniq_user -d soniq -v ON_ERROR_STOP=1 \
  -f - < backend/migrations/0001_create_recordings.up.sql

docker compose -f compose.temporal.yml exec -T soniq-postgresql \
  psql -U soniq_user -d soniq -v ON_ERROR_STOP=1 \
  -f - < backend/migrations/0002_add_recording_audio_metadata.up.sql
```

For local reset/testing, apply the matching down migrations in reverse order:

```bash
docker compose -f compose.temporal.yml exec -T soniq-postgresql \
  psql -U soniq_user -d soniq -v ON_ERROR_STOP=1 \
  -f - < backend/migrations/0002_add_recording_audio_metadata.down.sql

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
- connect to Soniq application Postgres with `POSTGRES_DSN`;
- register `RecordingProcessingWorkflow`;
- register store-backed recording processing activities under the stable Temporal activity names used by the workflow;
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

## Temporal workflow boundaries

The current Temporal implementation is intentionally narrow but no longer stateless:

- The workflow is implemented with the real Temporal Go SDK and covered by the Temporal SDK testsuite.
- Workflow code stays deterministic and delegates Soniq Postgres writes to activities.
- Worker-registered activities validate that the recording exists, persist `processing`, persist `completed`, and can persist `failed` on the completion-failure path.
- The API calls an injectable recording processor seam after `POST /recordings` and `POST /recordings/upload`; the production API command wires that seam to a Temporal client and starts `RecordingProcessingWorkflow` asynchronously.
- Worker startup is the boundary where the code leaves in-process tests and requires a real Temporal server plus Soniq application Postgres.

The workflow does not yet perform audio processing, ASR, LLM summarization, provider webhooks, or S3-compatible object storage. Those integrations should be added as separate milestones with explicit local service configuration.

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
| `STORAGE_PROVIDER` | `local` | Object storage provider selector. The implemented local development provider is `local`; S3-compatible storage is future-facing. |
| `LOCAL_STORAGE_PATH` | `var/uploads` | Local object storage root used when `STORAGE_PROVIDER=local`. |
| `TRANSCRIPTION_PROVIDER` | `faster_whisper` | Future transcription provider selector. |
| `LLM_PROVIDER` | `openai_compatible` | Future LLM provider selector. |
| `LLM_BASE_URL` | `https://api.openai.com/v1` | Future OpenAI-compatible endpoint. |
| `LLM_MODEL` | `gpt-4o-mini` | Future default LLM model name. |

Do not commit real secrets. Keep real API keys and credentials in local environment files only.

## Current milestone boundaries

The current backend foundation provides:

- a Go module under `backend/`;
- config loading and validation;
- a standard-library HTTP router;
- `GET /healthz`;
- Postgres-backed recording endpoints for metadata-only creation, audio upload, full-recording lookup, and status lookup;
- SQL migrations for the `recordings` table, including audio object metadata columns;
- a local filesystem object-store provider selected with `STORAGE_PROVIDER=local` and rooted at `LOCAL_STORAGE_PATH`;
- API and Temporal worker command entrypoints;
- a Temporal-backed recording processor that starts `RecordingProcessingWorkflow` after successful recording creation or upload requests;
- a Temporal SDK recording processing workflow skeleton;
- Soniq Postgres-backed activities for recording validation and durable status transitions;
- root `Makefile` quality and smoke commands.

It does not yet provide:

- MinIO/S3 storage integration;
- ffmpeg audio processing;
- ASR or LLM provider calls;
- authentication/RBAC;
- frontend UI.
