# Provider Abstraction

Soniq must run across global, domestic, self-hosted, and offline environments. Provider abstraction is therefore a core design requirement.

## StorageProvider

```go
type StorageProvider interface {
    PutObject(ctx context.Context, key string, body io.Reader, contentType string) error
    GetObject(ctx context.Context, key string) (io.ReadCloser, error)
    DeleteObject(ctx context.Context, key string) error
    PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
    PresignPut(ctx context.Context, key string, ttl time.Duration, contentType string) (string, error)
}
```

Current implementation:

- `local` filesystem provider for development. Original uploads and derived normalized audio artifacts are stored under `LOCAL_STORAGE_PATH`; normalized audio uses a deterministic sibling object key ending in `/normalized.wav`.

Planned implementations:

- `s3_compatible`

Later implementations:

- AWS S3
- Cloudflare R2
- Aliyun OSS
- Tencent COS
- Huawei OBS

## TranscriptionProvider

```go
type TranscriptionProvider interface {
    Name() string
    Transcribe(ctx context.Context, input TranscriptionInput) (*TranscriptionResult, error)
}
```

Provider categories:

- Synchronous: Xiaomi MiMo `mimo-v2.5-asr` via OpenAI-compatible chat-completions audio input, OpenAI/Groq Whisper-style providers, local faster-whisper.
- Asynchronous: AssemblyAI, Deepgram batch, Aliyun ASR, Tencent ASR, Volcengine ASR, iFlytek long audio.

Current implementation:

- `fake_transcription`, a deterministic local provider used by default smoke tests to verify the real API/storage/Postgres/Temporal/worker/persistence pipeline without credentials. In the current workflow it receives the normalized audio local path from `recording_normalized_audios`, not the original upload path.
- `openai_compatible_asr`, a real provider adapter shape that is covered by automated fake-server smoke tests and can be pointed at real compatible providers manually.
- `dashscope_asr`, a native DashScope ASR adapter for explicit/manual real-provider runs.

Verification boundary:

- Automated smoke (`make smoke-postgres-temporal`) should keep deterministic fake model providers by default. It validates the real processing pipeline and persistence boundary without network, credentials, provider cost, or privacy risk.
- Automated provider-shape smoke should use local fake servers such as `scripts/smoke-openai-compatible-asr-fake.sh`; this verifies request/response wiring without calling external model providers.
- Real external ASR verification is manual/opt-in. Set provider-specific credentials in a local ignored environment and run `scripts/smoke-postgres-temporal.sh` explicitly. Do not make real provider calls part of the default smoke path.

Initial real target providers:

- `faster_whisper` via local HTTP worker.
- `openai_compatible_transcription` where available.

## LLMProvider

```go
type LLMProvider interface {
    Name() string
    GenerateText(ctx context.Context, input GenerateTextInput) (*GenerateTextResult, error)
    GenerateStructured(ctx context.Context, input GenerateStructuredInput) (*GenerateStructuredResult, error)
}
```

Current implementation:

- `fake_llm`, a deterministic local summary provider used by default smoke tests to verify workflow/activity/persistence wiring without credentials.
- `openai_compatible`, a real LLM adapter shape for explicit/manual compatible-provider runs.

Initial real implementations:

- `openai_compatible`
- `ollama`

This covers many global and domestic options:

- OpenAI
- DeepSeek
- Qwen compatible mode
- Moonshot/Kimi
- vLLM
- local OpenAI-compatible gateways

Later native implementations:

- Anthropic
- Gemini
- Volcengine Doubao
- Zhipu GLM

## Provider selection

Provider choice should be resolved per workspace or per workflow run:

```yaml
transcription:
  provider: faster_whisper

llm:
  provider: openai_compatible
  base_url: https://api.deepseek.com/v1
  model: deepseek-chat
```

## Provider fallback

Future versions may support fallback chains:

```yaml
transcription:
  providers:
    - faster_whisper
    - groq_whisper
    - openai
```

Fallback must be explicit because compliance-sensitive deployments may forbid external providers.


## Xiaomi MiMo ASR configuration target

The first real transcription provider target is Xiaomi MiMo ASR (`mimo-v2.5-asr`). It uses `POST /chat/completions` at `https://api.xiaomimimo.com/v1` with a Base64 `input_audio` content part, not multipart `/audio/transcriptions`. Runtime API keys must come from local environment variables such as `TRANSCRIPTION_API_KEY` or `MIMO_API_KEY`; repository files must contain placeholders only.

Automated verification must use `scripts/smoke-openai-compatible-asr-fake.sh`, which starts a local fake ASR server and does not call Xiaomi. Real Xiaomi verification is manual-only: set `TRANSCRIPTION_PROVIDER=openai_compatible_asr`, `TRANSCRIPTION_API_KEY`, `TRANSCRIPTION_MODEL=mimo-v2.5-asr`, `TRANSCRIPTION_AUTH_HEADER=api-key`, and `TRANSCRIPTION_LANGUAGE=zh` in local `.env`, then run `scripts/smoke-postgres-temporal.sh`. That manual smoke sends normalized WAV audio to Xiaomi MiMo, so use only appropriate test audio and never commit real keys.
