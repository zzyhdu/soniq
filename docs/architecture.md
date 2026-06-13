# Architecture

Soniq is designed as a modular monolith plus durable workers. The first implementation should avoid premature microservices while keeping clear boundaries between API, workflow orchestration, providers, storage, and domain logic.

## High-level architecture

```txt
Client / Web UI
  ↓
Go API Server
  ├── auth/session boundary
  ├── recording upload/session APIs
  ├── workflow start/query APIs
  └── webhook/signal endpoints
  ↓
Temporal Server
  ↓
Temporal Workers
  ├── Audio activities
  ├── Transcription activities
  ├── Transcript cleanup activities
  ├── Summary activities
  ├── Persistence activities
  └── Notification activities
  ↓
Postgres + Object Storage
```

## Service responsibilities

### API server

The API server is the synchronous user-facing boundary.

Responsibilities:

- Validate user/workspace permissions.
- Create recording records.
- Create presigned upload URLs.
- Start Temporal workflows.
- Expose workflow and recording status.
- Receive third-party webhooks when needed.
- Signal Temporal workflows.

The API server should not run long audio or AI jobs directly.

### Temporal workflow

Temporal workflows hold orchestration state and decisions.

Responsibilities:

- Define step order.
- Apply retry/timeout policies.
- Wait for asynchronous provider callbacks or polling.
- Support cancellation and reprocessing.
- Expose workflow query state.

Workflows must stay deterministic. All external I/O belongs in activities.

### Activities

Activities perform side effects.

Examples:

- Download/upload objects.
- Run ffmpeg.
- Call ASR providers.
- Call LLM providers.
- Persist transcript, summary, and mind map rows.
- Send webhooks/notifications.

Activities must be idempotent because Temporal can retry them.

### Providers

Providers are plugin-like adapters for infrastructure and AI vendors.

Core provider categories:

- `StorageProvider`
- `TranscriptionProvider`
- `LLMProvider`
- `SecretProvider`
- `NotificationProvider`

Provider selection should be configuration-driven.

## Artifact-based design

Soniq should treat every generated item as an artifact:

```txt
audio_original
audio_normalized
audio_chunk
transcript_raw
transcript_clean
summary
mind_map
title
action_items
export
```

Artifacts make reprocessing and debugging easier. The database stores metadata; object storage stores large payloads.

## Database migration boundary

Soniq application migrations belong to the Soniq Postgres database, not Temporal's internal Postgres database. The API and worker processes do not apply migrations during startup. Local development uses `make migrate` to apply missing Soniq application migrations to `soniq-postgresql`; production deployments should run migrations as an explicit deployment step before starting new API/worker versions.

The identity/workspace foundation is recorded as baseline schema version `1` in `schema_migrations` and represented by `backend/migrations/0001_baseline.up.sql`. Later schema changes, such as recording failure metadata in version `2`, are applied as additional versions. Older local databases with the complete pre-baseline schema may be marked as version `1`; partial local schemas should be inspected or reset before migrating. Future schema changes should continue to be added as later migration versions instead of changing baseline version `1`.

## Why not serverless-first?

This project needs to run in domestic, global, and private infrastructure. A Docker/Kubernetes-first architecture is more portable than binding to Cloudflare Workers, Vercel, Lambda, or a domestic function platform.
