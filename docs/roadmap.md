# Roadmap

## Current progress snapshot

Soniq has moved from documentation-only architecture into an executable backend foundation.

Completed foundation milestones:

- Go backend skeleton with `GET /healthz`.
- Postgres-backed recording API: `POST /recordings`, `POST /recordings/upload`, `GET /recordings/{id}`, `GET /recordings/{id}/status`.
- Local filesystem object storage for uploaded recording audio.
- Temporal workflow skeleton with Postgres-backed recording status activities.
- Temporal worker registration with Soniq Postgres-backed activities.
- API-to-Temporal workflow start: successful recording creation/upload starts `RecordingProcessingWorkflow` asynchronously.
- Local Temporal development environment: Docker Compose services, Makefile targets, docs, and a full smoke helper.
- Verified local smoke path: API → local object store → Soniq Postgres → Temporal → worker → `recordings.status=completed`.

The next focus is replacing skeleton processing with real audio/AI activities.

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
- API starts workflow after successful recording creation.
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
- Wire production `cmd/api` to use Postgres, while tests can continue to use in-memory/fake stores.

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

### 1E — Audio upload and object storage

Status: planned.

- S3-compatible storage provider with MinIO local setup.
- Audio upload session and recording creation API.
- Store original audio artifact metadata.

### 1F — First real processing activities

Status: planned.

- ffmpeg probe/normalize activities.
- One transcription provider.
- One LLM provider.
- Persist transcript and summary.

### 1G — Basic web UI

Status: planned.

- Basic web UI for upload/status/result.

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
