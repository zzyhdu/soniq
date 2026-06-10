# Roadmap

## Current progress snapshot

Soniq has moved from documentation-only architecture into an executable backend foundation.

Completed foundation milestones:

- Go backend skeleton with `GET /healthz`.
- Postgres-backed recording API: `POST /recordings`, `POST /recordings/upload`, `GET /recordings/{id}`, `GET /recordings/{id}/status`.
- Local filesystem object storage for uploaded recording audio.
- Temporal workflow with Postgres-backed recording status, audio-probe, audio-normalization, fake transcription, and fake summarization activities.
- Temporal worker registration with Soniq Postgres-backed activities.
- API-to-Temporal workflow start: successful audio uploads start `RecordingProcessingWorkflow` asynchronously; metadata-only recording creation does not enqueue processing.
- Local Temporal development environment: Docker Compose services, Makefile targets, docs, and a full smoke helper.
- Verified local smoke path: API → local object store → Soniq Postgres → Temporal → worker → `ffprobe` → `recording_audio_probes` → `ffmpeg` → `recording_normalized_audios` + local `normalized.wav` → fake transcription → `recording_transcripts`/segments → fake summary → `recording_summaries` → `recordings.status=completed`. This is a real pipeline/infrastructure smoke; only the model providers are deterministic fakes by default.
- Backend-owned OpenAPI + Scalar API Console served at `/openapi.yaml` and `/api-console`, with same-origin browser "Try it" support for upload, status, and details endpoints.
- Basic product Web UI foundation under `apps/web`: pnpm workspace, typed `@soniq/api-client`, Vite React app shell, Tailwind/shadcn primitives, upload form, processing status polling, and completed transcript/summary display.

The next focus is choosing a product direction: keep real external model providers as manual/opt-in verification while improving provider productization, or begin the basic product web UI on top of the now-documented API.

## Phase 0 — Project skeleton and architecture docs

Status: complete.

- Create repository structure.
- Document architecture, workflows, providers, infrastructure, security, and data model.
- Add ADRs for major technical choices.

## Phase 1 — Minimal working pipeline

Phase 1 is now split into small implementation milestones.

### 1A — Backend and Recording API skeleton

Status: complete.

- Go API server.
- In-memory recording metadata API.
- Config loading and validation.
- Root quality commands.

### 1B — Temporal workflow skeleton and local runtime

Status: complete.

- Temporal worker and `RecordingProcessingWorkflow` skeleton.
- Activity stubs for validation and status transitions.
- API starts workflow after successful audio upload.
- Local Temporal Compose runtime.
- Manual smoke helper for API → Temporal → worker verification.

### 1C — Postgres recording persistence

Status: complete.

Goal: replace the production API's in-memory recording metadata store with durable Postgres-backed persistence while keeping unit tests hermetic.

Completed scope:

- Add a Soniq-owned local Postgres service separate from Temporal's internal database.
- Add migrations for the `recordings` table.
- Add Postgres DSN configuration and startup validation.
- Add repository/store interface that preserves the current API handler shape.
- Implement Postgres-backed recording create/get/status behavior.
- Wire production `cmd/api` to use Postgres, while tests use focused fakes instead of a general in-memory recording store.

Recommended database boundary:

- Temporal's Postgres is an infrastructure database owned by Temporal.
- Soniq's Postgres is an application database owned by Soniq.
- In local development they may run in the same Docker Compose project, but should be separate services/databases by default to avoid coupling application migrations to Temporal internals.

### 1D — Persist workflow status transitions

Status: complete.

- Add activity dependencies for updating recording status durably.
- Persist `processing`, `completed`, and best-effort `failed` status transitions.
- Wire the worker to Soniq Postgres-backed activities under the workflow's stable Temporal activity names.
- Verify the full smoke path reaches both Temporal `COMPLETED` and `recordings.status=completed`.
- Consider a `workflow_runs` table once workflow metadata is needed beyond recording status.

### 1E — Audio upload, local object storage, and original-audio probe

Status: complete.

- Audio upload endpoint and original audio metadata persistence.
- Local filesystem object storage for development.
- `recording_audio_probes` table for original-audio ffprobe metadata.
- Worker `ProbeRecordingAudio` activity that resolves the local object path, runs `ffprobe`, and upserts probe metadata before completion.

Remaining future scope:

- S3-compatible storage provider with MinIO local setup.
- Storage download/presigned-input support for worker-side probing when the object is not local.

### 1F — First transcription, normalization, and summarization activities

Status: complete for provider-neutral local fake-provider foundation.

Completed scope:

- `recording_transcripts`, `recording_transcript_segments`, `recording_summaries`, and `recording_normalized_audios` persistence.
- Workflow status transitions through `processing`, `transcribing`, `summarizing`, and `completed`.
- Original-audio probe plus ffmpeg normalization to WAV/PCM (`pcm_s16le`, 16 kHz, mono).
- Deterministic local fake transcription and summary providers wired into worker registration; transcription requires normalized audio metadata and reads the normalized local path.
- Full local smoke verifies probe metadata, normalized audio metadata and artifact, transcript rows, segment rows, summary rows, Temporal `COMPLETED`, and `recordings.status=completed`.

Remaining future scope:

- Real ASR provider integration.
- Real LLM provider integration.
- Provider configuration, credentials, retries, and webhook/polling support.

### 1G — API Console developer UI

Status: complete.

- Backend-owned OpenAPI 3.1 contract embedded in the API binary.
- Scalar API Console served by the Go API at `/api-console`.
- Same-origin API visualization for health, metadata-only create, upload, status, and details.

### 1H — Basic product web UI

Status: in progress.

Completed scope:

- pnpm workspace with `apps/web` and `packages/api-client`.
- Typed recording API client for upload, status, and details endpoints.
- React + Vite + TypeScript app shell with Tailwind CSS, shadcn/ui primitives, React Query, and local API proxying.
- Browser upload form for title/workflow type/language/audio that starts processing through `POST /recordings/upload` and displays the created recording id plus `processing_enqueued` result.
- Status polling UI for `GET /recordings/{id}/status`.
- Transcript and summary display for `GET /recordings/{id}/details` after processing completes.

Remaining scope:

- Local Web UI documentation refresh.
- End-to-end manual verification against the real local backend pipeline.

## Phase 2 — Provider expansion

- Local faster-whisper worker.
- OpenAI-compatible LLM provider.
- Ollama provider.
- AssemblyAI or Deepgram provider.
- Domestic provider experiments: Qwen/DeepSeek/Kimi and one domestic ASR.

## Phase 3 — Workflow robustness

- Workflow cancellation.
- Retry failed step.
- Reprocess summary with a new template.
- Reprocess transcription with a new provider.
- Webhook/signal support for async ASR providers.
- Artifact versioning.

## Phase 4 — Enterprise features

- Workspaces and RBAC.
- Audit log.
- Retention policy.
- Workspace-level provider configuration.
- Webhooks for integration.
- Helm chart.

## Phase 5 — Advanced audio intelligence

- Speaker diarization.
- Long audio chunking and map-reduce summaries.
- Meeting/lecture/interview templates.
- Action items, decisions, timeline, chapters, tags.
- Export to Markdown/PDF/Notion/Google Docs.
