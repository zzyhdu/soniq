# Local Development

This document describes the current local backend workflow for Soniq.

Soniq is currently in the local audio-upload, Postgres-backed recording persistence, original-audio probe, normalized-audio artifact, and fake transcription/summarization milestone. The commands below intentionally run a small backend foundation:

- the API exposes `GET /healthz`;
- the API exposes recording endpoints: `POST /recordings`, `POST /recordings/upload`, `GET /recordings/{id}`, and `GET /recordings/{id}/status`;
- the production API command uses Soniq Postgres for recording metadata persistence;
- `POST /recordings` creates metadata-only recordings without starting processing; `POST /recordings/upload` accepts multipart audio, writes the original audio through an object-store seam, persists audio metadata, and then invokes the same injectable recording processor seam;
- the production API command wires the recording processor seam to Temporal and starts `RecordingProcessingWorkflow` asynchronously;
- the worker starts a real Temporal SDK worker, registers the recording processing workflow and Soniq Postgres-backed recording status/audio-probe/normalized-audio/transcript/summary activities, and polls the configured task queue;
- local filesystem object storage is implemented for development; the worker resolves local object keys, runs `ffprobe` against uploaded original audio to persist probe metadata, and runs `ffmpeg` to write a deterministic normalized WAV/PCM artifact;
- deterministic fake transcription and summarization providers are wired for local development verification; transcription reads the normalized audio artifact and persists transcript, transcript segment, and summary rows without external credentials;
- S3-compatible storage, real ASR providers, real LLM providers, authentication, and the web UI are not implemented in this milestone.

## Prerequisites

- Go 1.26 or newer. The backend module pins `toolchain go1.26.4`, which was the latest stable Go toolchain checked for this milestone.
- `make`.
- Required for audio-probe smoke testing: `ffmpeg` and `ffprobe` available on `PATH`.
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

This target runs `scripts/smoke-postgres-temporal.sh`. The script starts the Compose infrastructure, applies the recording migrations if needed, starts the API and worker as temporary local background processes, generates and uploads a small valid WAV file through `POST /recordings/upload`, verifies the local object file and Postgres audio metadata, verifies the recording can be read from Postgres before and after an API restart, confirms the Temporal workflow reaches `COMPLETED`, verifies `recordings.status=completed`, verifies a `recording_audio_probes` row was persisted from real `ffprobe` output, verifies a `recording_normalized_audios` row and local `normalized.wav` artifact were persisted from real `ffmpeg` normalization, verifies configured transcription provider transcript/segment rows and summary rows were persisted, and then stops the API/worker processes it started.

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

The recording endpoints now persist metadata in Soniq Postgres in the production API path. Records survive API process restarts as long as the local Postgres volume remains intact. Audio-backed recordings also write the original uploaded file under `LOCAL_STORAGE_PATH` through the local object-store provider; the worker later writes a sibling normalized artifact named `normalized.wav`.

`POST /recordings` creates a metadata-only recording row and returns that metadata record without enqueueing processing. After an audio-backed `POST /recordings/upload` succeeds in Postgres, the API calls an injectable `RecordingProcessor` seam. In the production API command, that seam is wired to a Temporal-backed processor that starts `RecordingProcessingWorkflow` asynchronously with workflow ID `recording-processing-<recording_id>` on `TEMPORAL_TASK_QUEUE`. The upload HTTP response returns an explicit `{recording, processing_enqueued}` envelope; it does not wait for workflow completion. The worker consumes that workflow from the same task queue, uses Soniq Postgres-backed activities, and updates the recording status from `uploaded` through `processing`, `transcribing`, `summarizing`, and `completed`. For audio-backed recordings, the worker resolves the original local object path, runs `ffprobe`, persists original-audio probe metadata in `recording_audio_probes`, runs `ffmpeg` normalization to create a local WAV/PCM artifact, persists normalized metadata in `recording_normalized_audios`, calls deterministic fake transcription against the normalized audio path, calls deterministic fake summary providers, persists `recording_transcripts`, `recording_transcript_segments`, and `recording_summaries`, and then marks the recording `completed`. If probe, normalization, transcription, summarization, or completion fails, the workflow schedules a best-effort `failed` status update before returning the original error.

### Upload an audio-backed recording

Use `POST /recordings/upload` for recordings that include an original audio file:

```bash
ffmpeg -hide_banner -loglevel error \
  -f lavfi -i sine=frequency=1000:duration=1 \
  -ac 1 -ar 16000 -c:a pcm_s16le /tmp/soniq-demo.wav

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
  "recording": {
    "id": "rec_...",
    "title": "Weekly sync",
    "status": "uploaded",
    "workflow_type": "meeting",
    "language": "en",
    "audio_object_key": "recordings/.../soniq-demo.wav",
    "audio_content_type": "audio/wav",
    "audio_size_bytes": 32078,
    "created_at": "...",
    "updated_at": "..."
  },
  "processing_enqueued": true
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

After the Temporal worker processes the upload, it probes the original audio with `ffprobe`, stores one probe row in `recording_audio_probes`, normalizes the audio with `ffmpeg` to a WAV/PCM target (`pcm_s16le`, 16 kHz, mono), stores one row in `recording_normalized_audios`, runs deterministic fake transcription/summarization providers, and stores transcript, segment, and summary rows. For local inspection:

```bash
docker compose -f compose.temporal.yml exec -T soniq-postgresql \
  psql -U soniq_user -d soniq -x \
  -c "SELECT recording_id, duration_seconds, format_name, codec_name, sample_rate, channels, bit_rate, probed_at FROM recording_audio_probes WHERE recording_id = '<id>'"
```

The probe, normalization, and fake transcription steps currently support local object storage only because the worker resolves object keys to `<LOCAL_STORAGE_PATH>/<object_key>` before invoking `ffprobe`, `ffmpeg`, or the local fake transcription provider. Fake transcription now reads the normalized object key from `recording_normalized_audios`; it does not silently fall back to the original upload.

### Create a metadata-only recording

Use `POST /recordings` when you only want to create a recording metadata row. This endpoint does not upload audio and does not enqueue `RecordingProcessingWorkflow`:

```bash
curl -i -X POST http://localhost:18080/recordings \
  -H 'Content-Type: application/json' \
  -d '{"title":"Weekly sync","workflow_type":"meeting","language":"en"}'
```

Use `POST /recordings/upload` for the Temporal processing path.

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

Upload a local audio file from another terminal. This is the path that starts `RecordingProcessingWorkflow`:

```bash
curl -i -X POST http://localhost:18080/recordings/upload \
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
    "title": "Weekly sync",
    "status": "uploaded",
    "workflow_type": "meeting",
    "language": "en",
    "audio_object_key": "recordings/.../audio.wav",
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

docker compose -f compose.temporal.yml exec -T soniq-postgresql \
  psql -U soniq_user -d soniq -v ON_ERROR_STOP=1 \
  -f - < backend/migrations/0003_create_recording_audio_probes.up.sql

docker compose -f compose.temporal.yml exec -T soniq-postgresql \
  psql -U soniq_user -d soniq -v ON_ERROR_STOP=1 \
  -f - < backend/migrations/0004_create_recording_transcripts_and_summaries.up.sql
```

For local reset/testing, apply the matching down migrations in reverse order:

```bash
docker compose -f compose.temporal.yml exec -T soniq-postgresql \
  psql -U soniq_user -d soniq -v ON_ERROR_STOP=1 \
  -f - < backend/migrations/0004_create_recording_transcripts_and_summaries.down.sql

docker compose -f compose.temporal.yml exec -T soniq-postgresql \
  psql -U soniq_user -d soniq -v ON_ERROR_STOP=1 \
  -f - < backend/migrations/0003_create_recording_audio_probes.down.sql

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
- register store-backed recording processing activities, including fake transcription and summarization activities, under the stable Temporal activity names used by the workflow;
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
- Worker-registered activities validate that the recording exists, persist `processing`, probe original audio with `ffprobe`, persist one `recording_audio_probes` row, normalize audio with `ffmpeg`, persist one `recording_normalized_audios` row, persist `transcribing`, persist configured transcription provider transcript and segment rows from the normalized audio path, optionally delete the original uploaded audio object when `PRIVACY_DELETE_ORIGINAL_AUDIO_AFTER_TRANSCRIPTION=true`, persist `summarizing`, persist a fake-provider summary row, persist `completed`, and can persist `failed` on probe/normalization/transcription/original-audio-deletion/summarization/completion failure paths.
- The API calls an injectable recording processor seam after `POST /recordings/upload`; the production API command wires that seam to a Temporal client and starts `RecordingProcessingWorkflow` asynchronously. `POST /recordings` only creates metadata and does not enqueue workflow processing.
- Worker startup is the boundary where the code leaves in-process tests and requires a real Temporal server plus Soniq application Postgres.

The default transcription provider remains local-safe `fake_transcription`. `openai_compatible_asr` can be enabled manually for Xiaomi MiMo ASR or a compatible endpoint. Real LLM summarization providers, provider webhooks, and S3-compatible object storage remain future milestones. Those integrations should be added separately with explicit local service configuration.

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
| `TRANSCRIPTION_PROVIDER` | `fake_transcription` | Transcription provider selector. Use `openai_compatible_asr` for Xiaomi MiMo/OpenAI-compatible ASR. |
| `TRANSCRIPTION_BASE_URL` | `https://api.xiaomimimo.com/v1` | OpenAI-compatible ASR base URL. |
| `TRANSCRIPTION_API_KEY` | empty | External ASR API key loaded from local `.env`; never commit real values. |
| `MIMO_API_KEY` | empty | Optional Xiaomi MiMo API key alias; `TRANSCRIPTION_API_KEY` takes precedence. |
| `TRANSCRIPTION_MODEL` | `mimo-v2.5-asr` | External ASR model name. |
| `TRANSCRIPTION_AUTH_HEADER` | `api-key` | ASR auth mode: `api-key` for Xiaomi MiMo or `bearer` for Bearer-compatible providers. |
| `TRANSCRIPTION_LANGUAGE` | `auto` | ASR language hint: `auto`, `zh`, or `en` for Xiaomi MiMo. |
| `TRANSCRIPTION_MAX_BASE64_BYTES` | `10485760` | Maximum Base64 audio payload size for chat-completions audio-input ASR. |
| `LLM_PROVIDER` | `openai_compatible` | Future LLM provider selector. |
| `LLM_BASE_URL` | `https://api.openai.com/v1` | Future OpenAI-compatible endpoint. |
| `LLM_MODEL` | `gpt-4o-mini` | Future default LLM model name. |
| `PRIVACY_DELETE_ORIGINAL_AUDIO_AFTER_TRANSCRIPTION` | `false` | When `true`, the worker deletes the original uploaded audio object after transcription succeeds; normalized audio remains for later pipeline steps. |
| `PRIVACY_ALLOW_EXTERNAL_MODEL_PROVIDERS` | `true` | Allows configured external ASR/LLM providers; set `false` to force local-safe provider choices. |

Do not commit real secrets. Keep real API keys and credentials in local environment files only.


## OpenAI-compatible ASR smoke modes

The automated ASR smoke uses a local fake server and never calls Xiaomi MiMo:

```bash
API_URL=http://localhost:18080 API_ADDRESS=:18080 bash scripts/smoke-openai-compatible-asr-fake.sh
```

Use the fake-server smoke for normal development and CI-style verification. It proves that the worker can call the OpenAI-compatible ASR adapter, send normalized WAV audio as Base64 `input_audio`, and persist transcript rows with `provider=openai_compatible_asr` and `model=mimo-v2.5-asr` without sending audio outside the machine.

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
```

Then export the values and run the regular end-to-end smoke:

```bash
set -a
. ./.env
set +a
API_URL=http://localhost:18080 API_ADDRESS=:18080 bash scripts/smoke-postgres-temporal.sh
```

This manual smoke sends the normalized audio artifact to Xiaomi MiMo. Use only non-sensitive test audio unless you have confirmed the privacy/compliance implications. Never paste or commit the real API key; `.env` is ignored by git.

## Current milestone boundaries

The current backend foundation provides:

- a Go module under `backend/`;
- config loading and validation;
- a standard-library HTTP router;
- `GET /healthz`;
- Postgres-backed recording endpoints for metadata-only creation, audio upload, full-recording lookup, and status lookup;
- SQL migrations for the `recordings` table, audio object metadata columns, `recording_audio_probes`, `recording_normalized_audios`, `recording_transcripts`, `recording_transcript_segments`, and `recording_summaries`;
- a local filesystem object-store provider selected with `STORAGE_PROVIDER=local` and rooted at `LOCAL_STORAGE_PATH`;
- API and Temporal worker command entrypoints;
- a Temporal-backed recording processor that starts `RecordingProcessingWorkflow` after successful audio upload requests;
- a Temporal SDK recording processing workflow;
- Soniq Postgres-backed activities for recording validation, durable status transitions, original-audio `ffprobe` metadata persistence, ffmpeg normalized-audio metadata persistence, fake transcription persistence from normalized audio, optional original uploaded audio deletion after successful transcription, and fake summary persistence;
- root `Makefile` quality and smoke commands.

It does not yet provide:

- MinIO/S3 storage integration;
- real ASR or LLM provider calls;
- authentication/RBAC;
- frontend UI.
