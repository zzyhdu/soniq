# Local Development

This document describes the current local backend and Web UI workflow for Soniq.

Soniq is currently in the password-session identity, workspace-scoped recording, Postgres-backed recording persistence, original-audio probe, normalized-audio artifact, fake transcription/summarization/mind-map generation, and failed-recording retry milestone. The commands below intentionally run a small backend foundation:

- the API exposes `GET /healthz` for process liveness and `GET /readyz` for dependency readiness;
- the API exposes identity endpoints: `GET /me` and `GET /workspaces`;
- the API exposes `POST /auth/signup`, `POST /auth/signin`, and `POST /auth/signout` for email/password accounts backed by an httpOnly session cookie;
- the API exposes workspace-scoped recording endpoints: `GET /workspaces/{workspace_id}/recordings`, `POST /workspaces/{workspace_id}/recordings`, `POST /workspaces/{workspace_id}/recordings/upload`, `GET /workspaces/{workspace_id}/recordings/trash`, `GET /workspaces/{workspace_id}/recordings/{recording_id}`, `GET /workspaces/{workspace_id}/recordings/{recording_id}/status`, `GET /workspaces/{workspace_id}/recordings/{recording_id}/details`, `POST /workspaces/{workspace_id}/recordings/{recording_id}/retry`, `DELETE /workspaces/{workspace_id}/recordings/{recording_id}`, `POST /workspaces/{workspace_id}/recordings/{recording_id}/restore`, and `DELETE /workspaces/{workspace_id}/recordings/{recording_id}/purge`;
- the production API command uses Soniq Postgres for user, workspace, membership, and recording metadata persistence;
- `POST /workspaces/{workspace_id}/recordings` creates metadata-only recordings without starting processing; `POST /workspaces/{workspace_id}/recordings/upload` accepts multipart audio, writes the original audio through an object-store seam, persists audio metadata, and then invokes the same injectable recording processor seam;
- the production API command wires the recording processor seam to Temporal and starts `RecordingProcessingWorkflow` asynchronously; failed audio-backed recordings can be reset and re-enqueued through the retry endpoint;
- the worker starts a real Temporal SDK worker, registers the recording processing workflow and Soniq Postgres-backed recording status/audio-preparation/transcript/summary/mind-map activities, polls the configured task queue, and runs a lightweight retry loop for pending/failed recording purge artifact cleanup rows;
- object storage uses the S3-compatible provider; the worker stages audio objects to temporary local files when needed, runs `ffprobe` and `ffmpeg`, and writes the deterministic normalized WAV/PCM artifact back through object storage;
- deterministic fake transcription, summarization, and mind map providers are wired for local development verification; opt-in external ASR/LLM providers are available for manual runs; transcription reads the normalized audio artifact and persists transcript, transcript segment, summary, and mind map rows;
- the product Web UI in `apps/web` loads the current user, shows sign in/sign up forms when the API returns `401`, selects a workspace, lists recording history, uploads audio through the Go API, exposes bookmarkable recording hash routes, polls processing status, displays failure reasons with retry, and displays completed transcript/summary/mind-map results;
- provider webhooks, multi-user account management, invitations, password reset, and production RBAC are not implemented in this milestone.

## Prerequisites

- Go 1.26 or newer. The backend module pins `toolchain go1.26.4`, which was the latest stable Go toolchain checked for this milestone.
- `make`.
- Required for audio-probe smoke testing: `ffmpeg` and `ffprobe` available on `PATH`.
- Optional for local Temporal/Postgres smoke testing: Docker and Docker Compose.
- Required for Web UI development: Node.js with `pnpm` available. Use the root
  pnpm workspace commands; do not use npm or yarn for this repository.

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

The root `Makefile` automatically reads `.env` when the file exists and exports
those values only to runtime targets such as `make api`, `make worker`,
`make env-check`, and smoke targets. Quality targets such as `make test`,
`make lint`, and `make fmt` stay environment-neutral. Use `make env-check` to
confirm which non-secret runtime selectors will be used. To use a different env
file for one command, set `ENV_FILE`:

```bash
ENV_FILE=.env.local make env-check
ENV_FILE=.env.local make worker
```

## Build production backend images

The root `Dockerfile` builds three backend runtime targets from the same source
and embedded migrations:

```bash
make docker-build-api
make docker-build-worker
make docker-build-migrate
```

Or build all three:

```bash
make docker-build
```

Default image tags are:

- `soniq-api:dev`
- `soniq-worker:dev`
- `soniq-migrate:dev`

Set `APP_VERSION` to tag images and inject build metadata into the binaries:

```bash
APP_VERSION=0.1.0 make docker-build
```

Each image runs as a non-root user and excludes local `.env` files, dependency
directories, local uploads, local service volumes, and test artifacts via
`.dockerignore`. The API and migration images use a distroless runtime. The
worker image uses a slim Debian runtime because audio processing requires
`ffmpeg` and `ffprobe`.

Verify the built binaries without connecting to Postgres or Temporal:

```bash
docker run --rm soniq-api:dev --version
docker run --rm soniq-worker:dev --version
docker run --rm soniq-migrate:dev --version
```

To run the API container against local dependencies, start the local services
and migrations first, then connect the container to the host network:

```bash
make temporal-up
make migrate
docker run --rm --network host \
  -e 'POSTGRES_DSN=postgres://soniq_user:soniq_password@localhost:5432/soniq?sslmode=disable' \
  -e TEMPORAL_ADDRESS=localhost:7233 \
  -e API_ADDRESS=:8080 \
  soniq-api:dev
```

Then verify:

```bash
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```

The current container path expects S3-compatible object storage. The local
Compose stack provides MinIO for image verification and local development.

## Full local smoke verification

To avoid opening several terminals manually, run the full smoke target from the repository root:

```bash
make smoke-postgres-temporal
```

This target runs `scripts/smoke-postgres-temporal.sh`. The script starts the Compose infrastructure, applies missing Soniq application migrations through `make migrate`, starts the API and worker as temporary local background processes, signs up or signs in a smoke user, uploads a small valid WAV file through that user's workspace, verifies the workspace-scoped object and Postgres audio metadata, verifies the recording can be read from Postgres before and after an API restart, confirms the Temporal workflow reaches `COMPLETED`, verifies `recordings.status=completed`, verifies a `recording_audio_probes` row was persisted from real `ffprobe` output, verifies a `recording_normalized_audios` row and normalized audio artifact were persisted from real `ffmpeg` normalization, verifies deterministic fake transcription/summary/mind-map rows were persisted, and then stops the API/worker processes it started.

By default this smoke target forces `TRANSCRIPTION_PROVIDER=fake_transcription` and `LLM_PROVIDER=fake_llm`, even if `.env` selects real external providers. This keeps the baseline smoke deterministic and independent of provider credentials, network availability, quota, and model behavior.

To run the same smoke on a non-default port and stop Compose services after the
run:

```bash
API_URL=http://localhost:18080 \
API_ADDRESS=:18080 \
SMOKE_DOWN=1 \
make smoke-postgres-temporal
```

The script verifies uploaded and normalized audio objects with MinIO `mc stat`.

Real ASR providers receive a presigned normalized-audio URL when
`STORAGE_PROVIDER=s3_compatible` is enabled. For external providers such as
DashScope, make sure `S3_ENDPOINT` resolves to an object-storage endpoint the
provider can reach; `http://localhost:9000` is only suitable for local MinIO
development and fake-provider verification.

The script intentionally leaves the Compose infrastructure running by default so local Postgres and Temporal state remain available for follow-up debugging. To stop Compose services after the smoke run, set:

```bash
SMOKE_DOWN=1 make smoke-postgres-temporal
```

If an API is already listening on `localhost:8080`, the script refuses to run because it needs to own API startup and restart during the persistence check. Stop the existing API process first, or run the smoke flow on a different local port:

```bash
API_URL=http://localhost:18080 API_ADDRESS=:18080 make smoke-postgres-temporal
```

To intentionally run the smoke flow against real external providers, opt in explicitly and use an audio file with real speech:

```bash
SMOKE_EXTERNAL_PROVIDERS=1 \
TRANSCRIPTION_PROVIDER=dashscope_asr \
LLM_PROVIDER=openai_compatible \
SMOKE_AUDIO_FILE=/path/to/speech.wav \
SMOKE_AUDIO_CONTENT_TYPE=audio/wav \
make smoke-postgres-temporal
```

## Run the API skeleton

`make api` now builds the HTTP router with a Postgres-backed recording store, S3-compatible object storage, and a Temporal-backed recording processor. At startup it opens Soniq Postgres and dials the configured Temporal server, so both services must be reachable before serving requests.

For local development, start the local services first:

```bash
make temporal-up
make migrate
make temporal-ps
```

The local Temporal frontend listens on `localhost:7233`, and the Temporal Web UI is available at:

```txt
http://localhost:8233
```

The same Compose stack also starts a local MinIO service for S3-compatible
storage verification. MinIO's S3 API listens on `localhost:9000`, the console
listens on `localhost:9001`, and a one-shot `minio-init` service creates the
`soniq` bucket. Local credentials are:

- access key: `soniq_minio_user`
- secret key: `soniq_minio_password`

The backend defaults to `STORAGE_PROVIDER=s3_compatible`, so API upload, worker
processing, and purge cleanup use MinIO/S3-compatible object storage.

Then start the API server:

```bash
make api
```

Default runtime configuration:

- `API_ADDRESS=:8080`
- `LOG_FORMAT=text`
- `LOG_LEVEL=info`
- `POSTGRES_DSN=postgres://soniq_user:***@localhost:5432/soniq?sslmode=disable`
- `TEMPORAL_ADDRESS=localhost:7233`
- `TEMPORAL_NAMESPACE=default`
- `TEMPORAL_TASK_QUEUE=soniq-audio-pipeline`
- `WORKER_METRICS_ADDRESS=:9091`
- `PURGE_ARTIFACT_CLEANUP_INTERVAL_SECONDS=300`
- `PURGE_ARTIFACT_CLEANUP_BATCH_SIZE=25`
- `STORAGE_PROVIDER=s3_compatible`
- `S3_ENDPOINT=http://localhost:9000`
- `S3_REGION=us-east-1`
- `S3_BUCKET=soniq`
- `S3_FORCE_PATH_STYLE=true`

By default the API listens on `:8080`. If that port is already in use, override the address:

```bash
API_ADDRESS=:18080 make api
```

If Postgres or Temporal is not reachable, `make api` fails during startup. `make api` does not apply database migrations; run `make migrate` after starting `soniq-postgresql` whenever the schema may be stale. Unit tests do not require running Postgres or Temporal services; command wiring is covered by injected fakes.

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

`/healthz` is intentionally lightweight: it only means the API process is alive.
Use `/readyz` to check whether the API can serve real traffic:

```bash
curl -i http://localhost:8080/readyz
```

`/readyz` checks Postgres, the current Soniq application migration version,
Temporal health, and S3-compatible bucket reachability. It returns `200` with
`status: "ready"` when all checks pass, and `503` with `status: "not_ready"`
plus short check errors when a dependency is unavailable. It does not return
DSNs, secrets, or storage credentials.

The API exposes Prometheus metrics at `/metrics`:

```bash
curl -i http://localhost:8080/metrics
```

Current API metrics include HTTP request count and request duration with
low-cardinality `route`, `method`, and `status` labels. Route labels use chi
route templates such as `/workspaces/{workspace_id}/recordings/{recording_id}`,
not concrete workspace or recording IDs.

The worker exposes Prometheus metrics on a separate HTTP endpoint:

```bash
curl -i http://localhost:9091/metrics
```

Current worker metrics include Temporal activity counts and durations,
recording terminal status update counts, and purge artifact cleanup
claimed/deleted/failed counters and run duration. Worker metrics use fixed
labels such as `activity`, `result`, and `status`; they do not include
recording, workspace, artifact, object key, or user identifiers.

To run the optional local observability stack:

```bash
make observability-up
```

This starts Prometheus, Grafana, OpenTelemetry Collector, and Jaeger through
`compose.observability.yml`. It is intentionally separate from
`make temporal-up` so the default local development path stays lightweight.

Local URLs:

- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3001` with local credentials
  `admin` / `soniq_admin`
- Jaeger: `http://localhost:16686`
- OpenTelemetry OTLP gRPC: `localhost:4317`
- OpenTelemetry OTLP HTTP: `http://localhost:4318`

Prometheus scrapes the local API at `host.docker.internal:8080/metrics` and the
local worker at `host.docker.internal:9091/metrics`. Run `make api` and
`make worker` on their default ports before expecting the `soniq-api` and
`soniq-worker` Prometheus targets to be up. Grafana is provisioned with
Prometheus and Jaeger datasources and a minimal `Soniq Overview` dashboard for
API request rate/error/latency, worker activity rate, recording terminal status
updates, and purge cleanup health. The OpenTelemetry Collector is available
for future OTLP metrics and traces; the current API and worker metrics still
use the direct Prometheus scrape path.

Useful commands:

```bash
make observability-ps
make observability-logs
make observability-smoke
make observability-down
```

Every API response includes `X-Request-ID`. If the client sends that header, the
API returns the same value; otherwise the API generates one. API and worker logs
use structured `slog` output. Keep `LOG_FORMAT=text` for local readability, or
set `LOG_FORMAT=json` when logs will be collected by a log backend. `LOG_LEVEL`
accepts `debug`, `info`, `warn`, or `error`.

The API and worker handle `SIGINT` and `SIGTERM` for local and Kubernetes
shutdowns. `make api` stops accepting new HTTP requests and waits briefly for
in-flight requests before exiting. `make worker` stops the Temporal worker and
cancels the purge artifact cleanup loop before exiting.

## Use the Recording API

The identity, workspace, and recording endpoints now persist metadata in Soniq Postgres in the production API path. `POST /auth/signup` creates a new user, default workspace, owner membership, and login session; subsequent requests authenticate through the `soniq_session` httpOnly cookie backed by the `user_sessions` table. Unsafe authenticated requests also require `X-CSRF-Token` copied from the readable `soniq_csrf` cookie. Records survive API process restarts as long as the local Postgres volume remains intact. Audio-backed recordings write the original uploaded file through S3-compatible object storage; the worker later writes a sibling normalized artifact named `normalized.wav`.

`POST /workspaces/{workspace_id}/recordings` creates a metadata-only recording row and returns that metadata record without enqueueing processing. After an audio-backed `POST /workspaces/{workspace_id}/recordings/upload` succeeds in Postgres, the API calls an injectable `RecordingProcessor` seam. In the production API command, that seam is wired to a Temporal-backed processor that starts `RecordingProcessingWorkflow` asynchronously with workflow ID `recording-processing-<recording_id>` on `TEMPORAL_TASK_QUEUE`. The upload HTTP response returns an explicit `{recording, processing_enqueued}` envelope; it does not wait for workflow completion. The worker consumes that workflow from the same task queue, uses Soniq Postgres-backed activities, and updates the recording status from `uploaded` through `processing`, `transcribing`, `summarizing`, and `completed`. For audio-backed recordings, the worker downloads the original object to a temporary local file, runs `ffprobe`, persists original-audio probe metadata in `recording_audio_probes`, runs `ffmpeg` normalization to create a temporary WAV/PCM artifact, uploads the normalized artifact, persists normalized metadata in `recording_normalized_audios`, calls the configured transcription provider with a presigned normalized-audio URL, calls deterministic fake summary and mind map providers, persists `recording_transcripts`, `recording_transcript_segments`, `recording_summaries`, and `recording_mind_maps`, and then marks the recording `completed` with `completed_at`. If probe, normalization, transcription, summarization, mind map generation, or completion fails, the workflow schedules a best-effort `failed` status update with `failure_reason` and `failed_at` before returning the original error.

### Upload an audio-backed recording

Use `POST /workspaces/{workspace_id}/recordings/upload` for recordings that include an original audio file. Sign up once, or use `/auth/signin` with the same cookie jar for an existing account:

```bash
curl -i -c /tmp/soniq-cookies.txt \
  -H 'Content-Type: application/json' \
  -X POST http://localhost:8080/auth/signup \
  -d '{"email":"owner@local.soniq","display_name":"Owner","password":"correct horse"}'

WORKSPACE_ID="$(curl -fsS -b /tmp/soniq-cookies.txt http://localhost:8080/workspaces \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["workspaces"][0]["id"])')"
CSRF_TOKEN="$(awk '$0 !~ /^#/ && $6 == "soniq_csrf" { token = $7 } END { print token }' /tmp/soniq-cookies.txt)"

ffmpeg -hide_banner -loglevel error \
  -f lavfi -i sine=frequency=1000:duration=1 \
  -ac 1 -ar 16000 -c:a pcm_s16le /tmp/soniq-demo.wav

curl -i -b /tmp/soniq-cookies.txt \
  -X POST "http://localhost:8080/workspaces/${WORKSPACE_ID}/recordings/upload" \
  -H "X-CSRF-Token: ${CSRF_TOKEN}" \
  -F 'title=Weekly sync' \
  -F 'workflow_type=meeting' \
  -F 'language=en' \
  -F 'audio=@/tmp/soniq-demo.wav;type=audio/wav'
```

If you started the API on a custom port, use that port instead:

```bash
curl -i -b /tmp/soniq-cookies.txt \
  -X POST "http://localhost:18080/workspaces/${WORKSPACE_ID}/recordings/upload" \
  -H "X-CSRF-Token: ${CSRF_TOKEN}" \
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
  "recording": {
    "id": "rec_...",
    "workspace_id": "wsp_...",
    "title": "Weekly sync",
    "status": "uploaded",
    "workflow_type": "meeting",
    "language": "en",
    "audio_object_key": "workspaces/wsp_.../recordings/.../soniq-demo.wav",
    "audio_content_type": "audio/wav",
    "audio_size_bytes": 32078,
    "created_at": "...",
    "updated_at": "..."
  },
  "processing_enqueued": true
}
```

With the default configuration, uploaded and normalized audio objects are stored
in the local MinIO `soniq` bucket under workspace-scoped object keys.

After the Temporal worker processes the upload, it probes the original audio with `ffprobe`, stores one probe row in `recording_audio_probes`, normalizes the audio with `ffmpeg` to a WAV/PCM target (`pcm_s16le`, 16 kHz, mono), stores one row in `recording_normalized_audios`, runs deterministic fake transcription/summarization/mind-map providers, and stores transcript, segment, summary, and mind map rows. For local inspection:

```bash
docker compose -f compose.temporal.yml exec -T soniq-postgresql \
  psql -U soniq_user -d soniq -x \
  -c "SELECT recording_id, duration_seconds, format_name, codec_name, sample_rate, channels, bit_rate, probed_at FROM recording_audio_probes WHERE recording_id = '<id>'"
```

The probe and normalization steps download object keys to temporary local files
before invoking `ffprobe` and `ffmpeg`, upload the normalized artifact back
through object storage, and remove temporary files on best effort cleanup.
Transcription providers receive the normalized audio as a presigned object URL;
fake transcription uses the same request contract without calling an external
provider.

### Create a metadata-only recording

Use `POST /workspaces/{workspace_id}/recordings` when you only want to create a recording metadata row. This endpoint does not upload audio and does not enqueue `RecordingProcessingWorkflow`:

```bash
curl -i -b /tmp/soniq-cookies.txt \
  -X POST "http://localhost:18080/workspaces/${WORKSPACE_ID}/recordings" \
  -H "X-CSRF-Token: ${CSRF_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d '{"title":"Weekly sync","workflow_type":"meeting","language":"en"}'
```

Use `POST /workspaces/{workspace_id}/recordings/upload` for the Temporal processing path.

### Manual local Temporal smoke flow

This is the manual version of the full local smoke verification above. Use it when you want to inspect each process yourself in separate terminals. For routine checks, prefer `make smoke-postgres-temporal`.

It assumes Docker is available and uses the local Temporal/Postgres stack from `compose.temporal.yml`.

Start Temporal and Soniq Postgres and confirm the services are running:

```bash
make temporal-up
make migrate
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

Create or sign in a local account, then upload a local audio file from another terminal. This is the path that starts `RecordingProcessingWorkflow`:

```bash
curl -i -c /tmp/soniq-cookies.txt \
  -H 'Content-Type: application/json' \
  -X POST http://localhost:18080/auth/signup \
  -d '{"email":"owner@local.soniq","display_name":"Owner","password":"correct horse"}'

WORKSPACE_ID="$(curl -fsS -b /tmp/soniq-cookies.txt http://localhost:18080/workspaces \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["workspaces"][0]["id"])')"
CSRF_TOKEN="$(awk '$0 !~ /^#/ && $6 == "soniq_csrf" { token = $7 } END { print token }' /tmp/soniq-cookies.txt)"

curl -i -b /tmp/soniq-cookies.txt \
  -X POST "http://localhost:18080/workspaces/${WORKSPACE_ID}/recordings/upload" \
  -H "X-CSRF-Token: ${CSRF_TOKEN}" \
  -F title='Weekly sync' \
  -F workflow_type=meeting \
  -F language=en \
  -F audio=@/path/to/local/audio.wav
```

Expected response:

```http
HTTP/1.1 201 Created
Content-Type: application/json
```

```json
{
  "recording": {
    "id": "rec_...",
    "workspace_id": "wsp_...",
    "title": "Weekly sync",
    "status": "uploaded",
    "workflow_type": "meeting",
    "language": "en",
    "audio_object_key": "workspaces/wsp_.../recordings/.../audio.wav",
    "audio_content_type": "audio/wav",
    "audio_size_bytes": 12345,
    "created_at": "...",
    "updated_at": "..."
  },
  "processing_enqueued": true
}
```

Save the returned `recording.id`, then inspect the matching workflow execution in the Temporal Web UI:

```txt
recording-processing-<recording_id>
```

Fetch the full recording:

```bash
curl -i -b /tmp/soniq-cookies.txt \
  "http://localhost:18080/workspaces/${WORKSPACE_ID}/recordings/<id>"
```

Because the production API path now uses Postgres, this lookup should still work after restarting only the API process, provided the local Postgres service and volume are still running.

Fetch just the recording status:

```bash
curl -i -b /tmp/soniq-cookies.txt \
  "http://localhost:18080/workspaces/${WORKSPACE_ID}/recordings/<id>/status"
```

Expected status body before the worker has completed the workflow:

```json
{"id":"rec_...","workspace_id":"wsp_...","status":"uploaded"}
```

After the worker has processed the workflow successfully, the same endpoint should return:

```json
{"id":"rec_...","workspace_id":"wsp_...","status":"completed","completed_at":"..."}
```

If processing fails, the status response includes a reason:

```json
{"id":"rec_...","workspace_id":"wsp_...","status":"failed","failure_reason":"transcribe audio: ...","failed_at":"..."}
```

Retry a failed audio-backed recording:

```bash
curl -i -b /tmp/soniq-cookies.txt \
  -H "X-CSRF-Token: ${CSRF_TOKEN}" \
  -X POST "http://localhost:18080/workspaces/${WORKSPACE_ID}/recordings/<id>/retry"
```

Expected retry response:

```json
{
  "recording": {
    "id": "rec_...",
    "workspace_id": "wsp_...",
    "status": "uploaded"
  },
  "processing_enqueued": true
}
```

The current workflow status path is:

```txt
uploaded -> processing -> transcribing -> summarizing -> completed
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

## Apply application migrations locally

The local Soniq Postgres service is separate from Temporal's internal Postgres service. Apply Soniq application migrations to the `soniq` database only. Do not apply Soniq migrations to `temporal-postgresql`; Temporal owns that database and its schema.

Start the local services:

```bash
make temporal-up
```

Apply missing local application migrations:

```bash
make migrate
```

`make migrate` runs the Go migration command:

```bash
cd backend && go run ./cmd/migrate
```

The command reads `POSTGRES_DSN`, creates `schema_migrations` when needed, applies embedded `backend/migrations/*.up.sql` files in version order, and records each applied version. This is the same migration path intended for future container/Kubernetes jobs; a production image can run the compiled `soniq-migrate` binary without Docker Compose.

The current baseline application schema is represented by `backend/migrations/0001_baseline.up.sql` and recorded as version `1`. Later migrations add recording failure metadata, password sessions, mind maps, soft delete, and purge artifact cleanup rows. The current required application schema version is `6`; `/readyz` reports `503` when the database has not reached that version.

If a local database was created before the current migration system and does not have `schema_migrations`, reset that local application database before running migrations again. The project has not shipped yet, so we do not keep backward-compatibility migration paths for pre-release local schemas.

Baseline migration `0001` seeds legacy local fixture identity data:

- user: `usr_dev`
- workspace: `wsp_default`
- owner membership: `usr_dev` in `wsp_default`

After the baseline is present, the migration command records `version='1'` in `schema_migrations`, then applies later migration versions in order. Future schema changes should be added as later migration versions instead of extending baseline version `1`.

For local reset/testing, use the matching down migration only when you intentionally want to destroy local application schema/data. The normal local workflow should use `make migrate`.

### Debug purge artifact cleanup

Permanent recording purge removes database rows first and records object-storage
cleanup work in `recording_purge_artifacts`. The API attempts immediate object
cleanup, and the worker retries pending or failed rows.

To inspect cleanup state without writing SQL manually:

```bash
make debug-purge-artifacts
```

The script reads `POSTGRES_DSN` from the shell or `.env` through the Makefile.
When local `psql` is not installed, it falls back to `docker compose exec` using
the same Postgres service settings as `make migrate`. It prints status counts,
failed rows, and rows stuck in `deleting` for more than 10 minutes. It does not
print `object_key` by default because object keys can contain user-provided
filenames. Useful overrides:

```bash
LIMIT=50 make debug-purge-artifacts
STUCK_AFTER_MINUTES=30 make debug-purge-artifacts
```

## Run the Temporal worker skeleton

`make worker` starts a Temporal SDK worker and blocks while polling the configured task queue. It requires a reachable Temporal server at runtime.

`make worker` loads provider settings from `.env` through the root Makefile. If
you expect a real ASR/LLM provider, run `make env-check` first and verify that
`transcription_provider` and `llm_provider` are not the fake defaults.

For local development, start Temporal first, then run:

```bash
make worker
```

Default runtime configuration:

- `TEMPORAL_ADDRESS=localhost:7233`
- `TEMPORAL_NAMESPACE=default`
- `TEMPORAL_TASK_QUEUE=soniq-audio-pipeline`
- `LOG_FORMAT=text`
- `LOG_LEVEL=info`

Expected behavior:

- load environment configuration;
- validate minimal startup configuration;
- connect to Temporal;
- connect to Soniq application Postgres with `POSTGRES_DSN`;
- register `RecordingProcessingWorkflow`;
- register store-backed recording processing activities, including fake transcription, summarization, and mind map activities, under the stable Temporal activity names used by the workflow;
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

## Run the product Web UI

The product Web UI lives in `apps/web` and uses the root pnpm workspace. Shared typed API calls live in `packages/api-client` and are consumed by the Web app as `@soniq/api-client`.

The Web UI uses React, Vite, TypeScript, Tailwind CSS, React Query, and shadcn/ui-style primitives for workspace selection, recording history, upload, processing status, and transcript/summary/mind-map result display. It currently supports loading the local dev user, selecting a workspace, browsing the current workspace recording list, uploading audio through the Go API, polling processing status, and rendering transcript/summary/mind-map results after completion.

Run each long-lived process from the repository root in a separate terminal. Start the local dependencies first:

```bash
make temporal-up
make migrate
```

Then start the Go API:

```bash
make api
```

Start the Temporal worker:

```bash
make worker
```

Start the Web dev server with pnpm:

```bash
pnpm web:dev
```

Open the Vite URL, usually:

```txt
http://localhost:5173
```

The Vite dev server proxies `/healthz`, `/readyz`, `/auth/*`, `/me`, `/workspaces/*`, and legacy `/recordings/*` to `http://localhost:8080`, so keep `make api` running while using the browser UI. Keep `make worker` running if you want Temporal processing to continue after upload.

Manual browser verification:

1. Open the Vite local URL.
2. Confirm the default workspace is selected.
3. Select `testdata/asr/mimo-tts/mp3/zh-four-speaker-standup.mp3`.
4. Choose workflow type `meeting`.
5. Set language to `zh`.
6. Upload the recording.
7. Confirm upload succeeds, the recording appears in the history list, and the processing status progresses to `completed`.
8. Click historical recordings and confirm the transcript segments, summary, and mind map render after completion.

Useful Web UI checks:

```bash
git diff --check
pnpm test
pnpm typecheck
pnpm web:build
```

## Temporal workflow boundaries

The current Temporal implementation is intentionally narrow but no longer stateless:

- The workflow is implemented with the real Temporal Go SDK and covered by the Temporal SDK testsuite.
- Workflow code stays deterministic and delegates Soniq Postgres writes to activities.
- Worker-registered activities validate that the recording exists, persist `processing`, prepare audio by staging the original object once, probe original audio with `ffprobe`, persist one `recording_audio_probes` row, normalize audio with `ffmpeg`, persist one `recording_normalized_audios` row, persist `transcribing`, persist configured transcription provider transcript and segment rows from the normalized audio artifact, optionally delete the original uploaded audio object when `PRIVACY_DELETE_ORIGINAL_AUDIO_AFTER_TRANSCRIPTION=true`, persist `summarizing`, persist configured summary and mind map rows, persist `completed`, and can persist `failed` on audio-preparation/transcription/original-audio-deletion/summarization/mind-map generation/completion failure paths.
- The API calls an injectable recording processor seam after `POST /workspaces/{workspace_id}/recordings/upload`; the production API command wires that seam to a Temporal client and starts `RecordingProcessingWorkflow` asynchronously. `POST /workspaces/{workspace_id}/recordings` only creates metadata and does not enqueue workflow processing.
- Worker startup is the boundary where the code leaves in-process tests and requires a real Temporal server plus Soniq application Postgres.

The default transcription and LLM providers remain local-safe
`fake_transcription` and `fake_llm`. `openai_compatible_asr`, `dashscope_asr`,
and `openai_compatible` LLM can be enabled manually for real external-provider
runs. Provider webhooks remain future milestones.

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
| `LOG_FORMAT` | `text` | Process log format: `text` for local readability or `json` for collection. |
| `LOG_LEVEL` | `info` | Minimum log level: `debug`, `info`, `warn`, or `error`. |
| `AUTH_SESSION_TTL_HOURS` | `720` | Session lifetime for email/password login cookies. |
| `AUTH_COOKIE_SECURE` | `false` | Whether the `soniq_session` cookie is marked `Secure`. Keep `false` for local HTTP; use `true` behind HTTPS. |
| `API_ADDRESS` | `:8080` | Local HTTP listen address for `make api`. |
| `POSTGRES_DSN` | `postgres://soniq_user:***@localhost:5432/soniq?sslmode=disable` | Soniq application database used by `make api` for recording metadata persistence. |
| `TEMPORAL_ADDRESS` | `localhost:7233` | Temporal server address used by `make api` and `make worker`. |
| `TEMPORAL_NAMESPACE` | `default` | Temporal namespace used by `make api` and `make worker`. |
| `TEMPORAL_TASK_QUEUE` | `soniq-audio-pipeline` | Task queue used when the API starts workflows and the worker polls work. |
| `WORKER_METRICS_ADDRESS` | `:9091` | Worker Prometheus metrics listen address. Set empty to disable the worker metrics endpoint. |
| `WORKER_MAX_CONCURRENT_WORKFLOW_TASKS` | `20` | Maximum concurrent Temporal workflow task executions per worker process. Must be greater than 1. |
| `WORKER_MAX_CONCURRENT_ACTIVITIES` | `4` | Maximum concurrent Temporal activity executions per worker process. This bounds audio processing and external model calls per worker. |
| `WORKER_MAX_CONCURRENT_LOCAL_ACTIVITIES` | `4` | Maximum concurrent Temporal local activity executions per worker process. |
| `WORKER_TASK_QUEUE_ACTIVITIES_PER_SECOND` | `0` | Optional server-side activity rate limit for the whole Temporal task queue. `0` leaves the SDK default effectively unlimited. |
| `STORAGE_PROVIDER` | `s3_compatible` | Object storage provider selector. The supported value is `s3_compatible`. |
| `S3_ENDPOINT` | `http://localhost:9000` | S3-compatible endpoint used when `STORAGE_PROVIDER=s3_compatible`; points at local MinIO in the Compose stack. |
| `S3_REGION` | `us-east-1` | S3-compatible region value. |
| `S3_BUCKET` | `soniq` | S3-compatible bucket; created by the local `minio-init` service. |
| `S3_ACCESS_KEY` | `soniq_minio_user` | S3-compatible access key for local MinIO. |
| `S3_SECRET_KEY` | `soniq_minio_password` | S3-compatible secret key for local MinIO. |
| `S3_FORCE_PATH_STYLE` | `true` | S3-compatible path-style setting required by local MinIO. |
| `TRANSCRIPTION_PROVIDER` | `fake_transcription` | Transcription provider selector. Use `openai_compatible_asr` for Xiaomi MiMo/OpenAI-compatible ASR or `dashscope_asr` for native DashScope ASR. |
| `TRANSCRIPTION_BASE_URL` | `https://api.xiaomimimo.com/v1` | OpenAI-compatible ASR base URL. |
| `TRANSCRIPTION_API_KEY` | empty | External ASR API key loaded from local `.env`; never commit real values. |
| `MIMO_API_KEY` | empty | Optional Xiaomi MiMo API key alias; `TRANSCRIPTION_API_KEY` takes precedence. |
| `TRANSCRIPTION_MODEL` | `mimo-v2.5-asr` | External ASR model name. |
| `TRANSCRIPTION_AUTH_HEADER` | `api-key` | ASR auth mode: `api-key` for Xiaomi MiMo or `bearer` for Bearer-compatible providers. |
| `TRANSCRIPTION_LANGUAGE` | `auto` | ASR language hint: `auto`, `zh`, or `en` for Xiaomi MiMo. |
| `LLM_PROVIDER` | `fake_llm` | LLM provider selector. Use `openai_compatible` for manual external-provider summary and mind-map runs. |
| `LLM_BASE_URL` | `https://dashscope.aliyuncs.com/compatible-mode/v1` | OpenAI-compatible LLM endpoint used when `LLM_PROVIDER=openai_compatible`. |
| `LLM_API_KEY` | empty | External LLM API key loaded from local `.env`; falls back to `DASHSCOPE_API_KEY` when empty. |
| `LLM_MODEL` | `qwen3.7-plus` | External LLM model name used when `LLM_PROVIDER=openai_compatible`. |
| `PRIVACY_DELETE_ORIGINAL_AUDIO_AFTER_TRANSCRIPTION` | `false` | When `true`, the worker deletes the original uploaded audio object after transcription succeeds; normalized audio remains for later pipeline steps. |
| `PRIVACY_ALLOW_EXTERNAL_MODEL_PROVIDERS` | `true` | Allows configured external ASR/LLM providers; set `false` to force local-safe provider choices. |

Do not commit real secrets. Keep real API keys and credentials in local environment files only.

For Aliyun OSS, keep the same `s3_compatible` storage provider and point the S3
endpoint at the OSS S3-compatible endpoint. Example:

```env
STORAGE_PROVIDER=s3_compatible
S3_ENDPOINT=https://s3.oss-cn-hangzhou.aliyuncs.com
S3_REGION=cn-hangzhou
S3_BUCKET=<your-oss-bucket>
S3_ACCESS_KEY=<your-ram-access-key-id>
S3_SECRET_KEY=<your-ram-access-key-secret>
S3_FORCE_PATH_STYLE=false
```

The backend uses the AWS SDK as a generic S3 protocol client. This does not mean
objects are stored in AWS; the configured `S3_ENDPOINT` decides whether requests
go to local MinIO, Aliyun OSS, AWS S3, R2, COS, OBS, or another compatible
service.

## Password Auth

Email/password auth is the local API runtime mode. Apply Soniq application migrations first so `users.password_hash` and `user_sessions` exist:

```bash
make migrate
make api
```

Then open the Web UI. If you do not have an account, use Sign up. It calls `POST /auth/signup`, creates a user, creates a default workspace, creates an owner membership, creates a `user_sessions` row, and sets the httpOnly `soniq_session` cookie plus readable `soniq_csrf` cookie. Existing users use `POST /auth/signin`; `POST /auth/signout` revokes the current session and clears both cookies.

The password hasher is Argon2id with encoded per-password salt and parameters (`m=19456`, `t=2`, `p=1`), matching OWASP's current minimum recommendation. Passwords must be 8 to 1024 bytes. The database stores only `users.password_hash` and `user_sessions.token_hash`; the opaque session token only lives in the browser cookie.

`POST /auth/signin` and `POST /auth/signup` use an in-process fixed-window rate limiter keyed by auth action, client IP, and normalized email. The default limits are 10 sign-in attempts and 5 sign-up attempts per 5 minutes. Limited requests return `429` with `code: "rate_limited"`. This protects the single-process local/self-hosted runtime from simple password guessing and signup spam; a multi-instance deployment should replace it with a shared store such as Redis.

## OpenAI-compatible ASR smoke modes

The automated ASR smoke uses a local fake server and never calls Xiaomi MiMo:

```bash
API_URL=http://localhost:18080 API_ADDRESS=:18080 bash scripts/smoke-openai-compatible-asr-fake.sh
```

Use the fake-server smoke for normal development and CI-style verification. It proves that the worker can call the OpenAI-compatible ASR adapter, send a presigned normalized-audio URL in `input_audio.data`, and persist transcript rows with `provider=openai_compatible_asr` and `model=mimo-v2.5-asr` without calling an external ASR provider.

To manually test real Xiaomi MiMo ASR, put the real key only in local `.env` or your shell environment:

```bash
cp .env.example .env
# edit .env locally; do not commit it
TRANSCRIPTION_PROVIDER=openai_compatible_asr
TRANSCRIPTION_BASE_URL=https://api.xiaomimimo.com/v1
TRANSCRIPTION_API_KEY=<your Xiaomi MiMo key>
TRANSCRIPTION_MODEL=mimo-v2.5-asr
TRANSCRIPTION_AUTH_HEADER=api-key
TRANSCRIPTION_LANGUAGE=zh
PRIVACY_ALLOW_EXTERNAL_MODEL_PROVIDERS=true
STORAGE_PROVIDER=s3_compatible
```

Then export the values and run the regular end-to-end smoke:

```bash
set -a
. ./.env
set +a
API_URL=http://localhost:18080 API_ADDRESS=:18080 bash scripts/smoke-postgres-temporal.sh
```

This manual smoke sends a normalized-audio URL to Xiaomi MiMo. Use object storage with an endpoint reachable by Xiaomi MiMo, and use only non-sensitive test audio unless you have confirmed the privacy/compliance implications. Never paste or commit the real API key; `.env` is ignored by git.

## Current milestone boundaries

The current backend foundation provides:

- a Go module under `backend/`;
- a pnpm workspace with `apps/web` for the product Web UI and `packages/api-client` for shared typed recording API calls;
- config loading and validation;
- a chi HTTP router;
- `GET /healthz` and `GET /readyz`;
- identity endpoints for `GET /me` and `GET /workspaces`;
- email/password auth endpoints for signup, signin, and signout;
- Postgres-backed workspace-scoped recording endpoints for listing, metadata-only creation, audio upload, full-recording lookup, and status lookup;
- completed-recording details lookup for transcript segments, summary, and mind map results;
- embedded SQL migrations for `users`, `workspaces`, `workspace_members`, `user_sessions`, the `recordings` table, failure metadata, audio object metadata columns, `recording_audio_probes`, `recording_normalized_audios`, `recording_transcripts`, `recording_transcript_segments`, `recording_summaries`, `recording_mind_maps`, and purge artifact cleanup rows;
- a container-ready Go migration command under `backend/cmd/migrate`;
- S3-compatible object storage selected with `STORAGE_PROVIDER=s3_compatible`;
- API and Temporal worker command entrypoints;
- a Temporal-backed recording processor that starts `RecordingProcessingWorkflow` after successful audio upload requests;
- a Temporal SDK recording processing workflow;
- Soniq Postgres-backed activities for recording validation, durable status transitions, original-audio `ffprobe` metadata persistence, ffmpeg normalized-audio metadata persistence, fake transcription persistence from normalized audio, optional original uploaded audio deletion after successful transcription, fake summary persistence, and fake mind map persistence;
- root `Makefile` quality, migration, and smoke commands.
- a local developer Web UI for password signup/signin, workspace selection, recording history, upload, status polling, and completed transcript/summary/mind-map display.

It does not yet provide:

- provider webhooks or asynchronous provider callbacks;
- multi-user account management, invitations, password reset, and production RBAC;
- recording management actions beyond listing, selecting, uploading, and viewing completed results.
