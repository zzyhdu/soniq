# Local Development

This document describes the current local backend workflow for Soniq.

Soniq is currently in the Temporal workflow skeleton milestone. The commands below intentionally run a small backend foundation:

- the API exposes `GET /healthz`;
- the API exposes in-memory recording metadata endpoints: `POST /recordings`, `GET /recordings/{id}`, and `GET /recordings/{id}/status`;
- `POST /recordings` invokes an injectable recording processor seam after successful creation; the default processor is a no-op and does not start a real Temporal workflow yet;
- the worker starts a real Temporal SDK worker, registers the recording processing workflow and activity stubs, and polls the configured task queue;
- Postgres, object storage, ffmpeg, ASR, LLM providers, authentication, and the web UI are not implemented in this milestone.

## Prerequisites

- Go 1.24 or newer.
- `make`.

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

Start the API server:

```bash
make api
```

By default the API listens on `:8080`. If that port is already in use, override the address:

```bash
API_ADDRESS=:18080 make api
```

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

The recording endpoints currently store metadata in memory only. Records disappear when the API process exits or restarts. This skeleton does not upload audio, persist to Postgres, write objects to storage, or start Temporal workflows.

After a recording is created successfully, the API calls an injectable `RecordingProcessor` seam. The default local processor is a no-op, so the HTTP response remains fast and no Temporal workflow is started by the API yet. A later milestone can replace the no-op with a Temporal client implementation that starts `RecordingProcessingWorkflow`.

Start the API on a local test port:

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

Save the returned `id`, then fetch the full recording:

```bash
curl -i http://localhost:18080/recordings/<id>
```

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
- The API calls an injectable recording processor seam after `POST /recordings`; the default local processor is a no-op and does not start a production workflow.
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
| `TEMPORAL_ADDRESS` | `localhost:7233` | Future Temporal server address. |
| `TEMPORAL_NAMESPACE` | `default` | Future Temporal namespace. |
| `TEMPORAL_TASK_QUEUE` | `soniq-audio-pipeline` | Future worker task queue. |
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
- in-memory recording metadata endpoints;
- API and Temporal worker command entrypoints;
- a Temporal SDK recording processing workflow skeleton;
- activity stubs for validation and recording status transitions;
- root `Makefile` quality commands.

It does not yet provide:

- durable recording persistence;
- real recording audio upload handling;
- production Temporal smoke-test configuration;
- Postgres schema or migrations;
- MinIO/S3 storage integration;
- ffmpeg audio processing;
- ASR or LLM provider calls;
- authentication/RBAC;
- frontend UI.
