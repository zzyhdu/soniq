# Roadmap

## Phase 0 — Project skeleton and architecture docs

- Create repository structure.
- Document architecture, workflows, providers, infrastructure, security, and data model.
- Add ADRs for major technical choices.

## Phase 1 — Minimal working pipeline

- Go API server.
- Postgres schema and migrations.
- S3-compatible storage provider with MinIO local setup.
- Temporal worker and `RecordingProcessingWorkflow`.
- Audio upload session and recording creation API.
- ffmpeg probe/normalize activities.
- One transcription provider.
- One LLM provider.
- Persist transcript and summary.
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
