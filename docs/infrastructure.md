# Domestic and Global Infrastructure

Soniq is intended to work across global cloud, domestic cloud, private cloud, and offline environments.

## Deployment profiles

### global

Typical stack:

- Temporal Cloud or self-hosted Temporal.
- Postgres on RDS/Neon/Supabase.
- AWS S3 or Cloudflare R2.
- OpenAI, Anthropic, Gemini, Groq, AssemblyAI, Deepgram.

### china

Typical stack:

- Self-hosted Temporal.
- Postgres on Aliyun/Tencent/Huawei or self-managed.
- Aliyun OSS / Tencent COS / Huawei OBS / MinIO.
- Qwen, DeepSeek, Kimi, Doubao, GLM.
- Aliyun ASR, Tencent ASR, Volcengine ASR, iFlytek.

### self_hosted

Typical stack:

- Docker Compose or Kubernetes.
- Temporal plus its own internal Postgres database.
- Soniq application Postgres for business data.
- MinIO.
- OpenAI-compatible LLM endpoint or Ollama.
- faster-whisper worker.

### offline

Typical stack:

- On-prem server or private Kubernetes.
- MinIO or local filesystem.
- faster-whisper on CPU/GPU.
- vLLM/Ollama/Xinference.
- No external model provider calls.

## Configuration example

```yaml
server:
  public_url: http://localhost:8080
  region_profile: self_hosted

database:
  url: postgres://soniq_user:***@soniq-postgresql:5432/soniq?sslmode=disable

temporal:
  address: temporal:7233
  namespace: default
  task_queue: soniq-audio-pipeline

storage:
  provider: s3_compatible
  endpoint: http://minio:9000
  region: us-east-1
  bucket: soniq
  access_key: ${S3_ACCESS_KEY}
  secret_key: ${S3_SECRET_KEY}
  force_path_style: true

transcription:
  provider: faster_whisper
  endpoint: http://whisper-worker:8088

llm:
  provider: openai_compatible
  base_url: ${LLM_BASE_URL}
  api_key: ${LLM_API_KEY}
  model: ${LLM_MODEL}

privacy:
  delete_original_audio_after_transcription: false
  allow_external_model_providers: true
```

## Local database ownership

Local development uses two separate Postgres ownership boundaries:

- `temporal-postgresql` is Temporal-owned infrastructure state. Temporal manages its schema and migrations.
- `soniq-postgresql` is Soniq-owned application data. Soniq migrations create and update business tables such as `recordings`.

Do not apply Soniq migrations to Temporal's database, and do not place Soniq business tables in Temporal's schema. This keeps local development closer to production setups where Temporal Cloud or managed Temporal may not expose an internal database at all.

## Portability rules

- Do not require a specific cloud provider.
- Do not require OpenAI.
- Do not require serverless infrastructure.
- Support Docker Compose for local development.
- Support Kubernetes/Helm for production eventually.
- Keep object storage behind an interface.
- Keep AI providers behind interfaces.
