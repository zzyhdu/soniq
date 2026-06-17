# Soniq

> Workflow-native, self-hostable audio intelligence platform for recording transcription, summarization, and structured notes.

Soniq is an open-source project for the **recording → transcription → cleanup → summarization** scenario. It is designed as an enterprise-grade, provider-agnostic platform that can run in global, China, self-hosted, and offline infrastructure environments.

## Goals

- **Workflow-native**: use Temporal as the durable execution backbone for long-running audio pipelines.
- **Self-hostable**: local Docker Compose first; Kubernetes/Helm later.
- **Provider-agnostic**: pluggable storage, transcription, LLM, notification, and secret providers.
- **China/global ready**: avoid hard dependency on OpenAI, AWS, Vercel, Cloudflare, or any single vendor.
- **Enterprise-oriented**: workspace, audit log, retention policy, RBAC, webhooks, and private deployment are first-class design concerns.
- **Learning-friendly**: clear architecture and small, testable modules for learning Go, Temporal, audio processing, and AI providers.

## Non-goals for the MVP

- Real-time meeting assistant.
- Mobile apps.
- Full enterprise SSO/RBAC implementation.
- Payment/billing.
- Template marketplace.
- Complex microservice architecture.

The first version should prove the core durable pipeline:

```txt
Upload audio
  ↓
Store original artifact
  ↓
Temporal RecordingProcessingWorkflow
  ↓
Probe / normalize audio
  ↓
Transcribe
  ↓
Clean transcript
  ↓
Generate summary/title/action items
  ↓
Persist results
  ↓
Notify client/webhook
```

## Recommended tech stack

| Layer | Choice | Rationale |
|---|---|---|
| Frontend | React / Vite or Next.js | Fast UI iteration, broad contributor base |
| API server | Go + chi | High-concurrency, simple deployment, good infra fit |
| Workflow | Temporal Go SDK | Durable execution, retries, signals, long-running jobs |
| DB | Postgres | Reliable system-of-record |
| Object storage | S3-compatible + MinIO default | Works locally, globally, and in private deployments |
| Audio | ffmpeg | Standard audio probe/normalize/split tool |
| Transcription | Provider interface | OpenAI/Groq/AssemblyAI/Deepgram/local Whisper/domestic ASR |
| LLM | OpenAI-compatible first | Covers OpenAI, DeepSeek, Qwen compatible mode, Moonshot, vLLM, etc. |
| Local AI | faster-whisper + Ollama/vLLM optional | Offline/private deployment |

## Repository layout

```txt
soniq/
├── backend/                  # Go backend services
│   ├── cmd/
│   │   ├── api/              # Go API server entrypoint
│   │   └── worker/           # Go Temporal worker entrypoint
│   └── internal/
│       ├── api/              # HTTP handlers, middleware, routing
│       ├── workflows/        # Temporal workflows
│       ├── activities/       # Temporal activities with side effects
│       ├── providers/        # Storage/transcription/LLM provider implementations
│       ├── db/               # Repositories and migrations
│       ├── storage/          # Object storage abstractions
│       ├── config/           # Configuration loading/validation
│       └── domain/           # Core domain types
├── web/                      # Frontend app
├── deploy/
│   └── kubernetes/base/      # Raw Kubernetes deployment foundation
├── docs/
│   ├── architecture.md
│   ├── workflows.md
│   ├── providers.md
│   ├── infrastructure.md
│   ├── data-model.md
│   ├── security-privacy.md
│   ├── roadmap.md
│   └── adr/
└── .env.example
```

## MVP capabilities

1. Create recording upload session.
2. Upload audio to S3-compatible storage / MinIO.
3. Start Temporal workflow.
4. Probe and normalize audio with ffmpeg.
5. Transcribe with one configured transcription provider.
6. Clean transcript and generate summary with one configured LLM provider.
7. Persist transcript segments, summary, artifacts, and workflow status.
8. Query workflow/recording status from API.
9. Display transcript and summary in web UI.

## Deployment profiles

Soniq should support profiles instead of hardcoded vendors:

- `global`: S3/R2 + OpenAI/Anthropic/Gemini/Groq.
- `china`: OSS/COS/OBS + Qwen/DeepSeek/Kimi/Doubao + domestic ASR.
- `self_hosted`: MinIO + OpenAI-compatible endpoint or Ollama.
- `offline`: local filesystem/MinIO + faster-whisper + vLLM/Ollama.

## Local development target

The default local stack should eventually be:

```txt
web
api
worker
temporal
postgres
minio
optional whisper-worker
optional ollama
```

## Documentation

Start with:

- [Architecture](docs/architecture.md)
- [Local development](docs/development.md)
- [Temporal workflows](docs/workflows.md)
- [Provider abstraction](docs/providers.md)
- [Domestic/global infrastructure](docs/infrastructure.md)
- [Data model](docs/data-model.md)
- [Security and privacy](docs/security-privacy.md)
- [Roadmap](docs/roadmap.md)
- [ADR-0001: Use Temporal](docs/adr/0001-use-temporal.md)
- [ADR-0002: Use Go for backend and workers](docs/adr/0002-use-go-backend.md)

## License

Apache-2.0.
