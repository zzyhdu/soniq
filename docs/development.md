# Local Development

This document describes the current local backend workflow for Soniq.

Soniq is still in the backend foundation milestone. The commands below intentionally run a small skeleton backend:

- the API exposes `GET /healthz` only;
- the worker validates configuration and exits cleanly;
- Temporal, Postgres, object storage, ffmpeg, ASR, LLM providers, authentication, and the web UI are not implemented in this milestone.

## Prerequisites

- Go 1.23 or newer.
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

## Run the worker skeleton

Run:

```bash
make worker
```

Expected behavior:

- load environment configuration;
- validate minimal startup configuration;
- print a skeleton-ready message;
- print Temporal address, namespace, and task queue;
- exit successfully;
- do not print secrets such as API keys.

Example output:

```txt
worker skeleton ready
temporal_address=localhost:7233
temporal_namespace=default
temporal_task_queue=soniq-audio-pipeline
```

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
- API and worker command entrypoints;
- root `Makefile` quality commands.

It does not yet provide:

- recording upload APIs;
- Temporal workflows or activities;
- Postgres schema or migrations;
- MinIO/S3 storage integration;
- ffmpeg audio processing;
- ASR or LLM provider calls;
- authentication/RBAC;
- frontend UI.
