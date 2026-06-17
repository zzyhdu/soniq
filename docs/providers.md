# Provider Abstraction

Soniq must run across global, domestic, self-hosted, and offline environments. Provider abstraction is therefore a core design requirement.

## StorageProvider

```go
type ObjectStore interface {
    PutObject(ctx context.Context, input PutObjectInput) (PutObjectResult, error)
    GetObject(ctx context.Context, key string) (GetObjectResult, error)
    PresignGetObject(ctx context.Context, key string, ttl time.Duration) (string, error)
    DeleteObject(ctx context.Context, key string) error
}
```

Current implementation:

- `s3_compatible` provider for MinIO, AWS S3, Cloudflare R2, Aliyun OSS, Tencent COS, Huawei OBS, and other S3-compatible services. The implementation uses the AWS SDK as a generic S3 protocol client; it is not an AWS cloud-service dependency. API upload uses `PutObject`, worker processing downloads objects to temporary local files for `ffprobe` and `ffmpeg`, normalized audio is uploaded back through object storage, real ASR providers receive a presigned normalized audio URL, and deletion/purge cleanup use `DeleteObject`.

S3-compatible target examples:

- AWS S3
- Cloudflare R2
- Aliyun OSS
- Tencent COS
- Huawei OBS

Provider-specific notes:

- MinIO local development normally uses path-style addressing, so set `S3_FORCE_PATH_STYLE=true`.
- Aliyun OSS documents AWS SDK access through its S3-compatible endpoint, such as `https://s3.oss-cn-hangzhou.aliyuncs.com`; use virtual-host style addressing for OSS by setting `S3_FORCE_PATH_STYLE=false`.

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

- `fake_transcription`, a deterministic local provider used by default smoke tests to verify the real API/storage/Postgres/Temporal/worker/persistence pipeline without credentials. It uses the same URL-based transcription request contract without calling an external provider.
- `openai_compatible_asr`, a real provider adapter shape that is covered by automated fake-server smoke tests and can be pointed at real compatible providers manually.
- `dashscope_asr`, a native DashScope ASR adapter for explicit/manual real-provider runs.

Verification boundary:

- Automated smoke (`make smoke-postgres-temporal`) should keep deterministic fake model providers by default. It validates the real processing pipeline and persistence boundary without network, credentials, provider cost, or privacy risk.
- Automated provider-shape smoke should use local fake servers such as `scripts/smoke-openai-compatible-asr-fake.sh`; this verifies request/response wiring without calling external model providers.
- Real external ASR verification is manual/opt-in. Set provider-specific credentials in a local ignored environment and run `scripts/smoke-postgres-temporal.sh` explicitly. Do not make real provider calls part of the default smoke path.
- Real ASR providers are URL-only. They receive object-storage URLs and do not read local audio files or Base64 payloads. DashScope native non-realtime ASR uses `file_urls`; OpenAI-compatible ASR providers receive the same URL in `input_audio.data`. For manual real-provider runs, configure `STORAGE_PROVIDER=s3_compatible` with an endpoint the ASR provider can reach. Local MinIO URLs are useful for fake-server smoke tests but are not reachable by external providers.

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

- `fake_llm`, a deterministic local summary and mind map provider used by default smoke tests to verify workflow/activity/persistence wiring without credentials.
- `openai_compatible`, a real LLM adapter shape for explicit/manual compatible-provider summary and mind map runs.

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

The first real transcription provider target is Xiaomi MiMo ASR (`mimo-v2.5-asr`). It uses `POST /chat/completions` at `https://api.xiaomimimo.com/v1` with a URL in the `input_audio` content part, not multipart `/audio/transcriptions`. Runtime API keys must come from local environment variables such as `TRANSCRIPTION_API_KEY` or `MIMO_API_KEY`; repository files must contain placeholders only.

Automated verification must use `scripts/smoke-openai-compatible-asr-fake.sh`, which starts a local fake ASR server and does not call Xiaomi. Real Xiaomi verification is manual-only: set `STORAGE_PROVIDER=s3_compatible` with externally reachable object storage, `TRANSCRIPTION_PROVIDER=openai_compatible_asr`, `TRANSCRIPTION_API_KEY`, `TRANSCRIPTION_MODEL=mimo-v2.5-asr`, `TRANSCRIPTION_AUTH_HEADER=api-key`, and `TRANSCRIPTION_LANGUAGE=zh` in local `.env`, then run `scripts/smoke-postgres-temporal.sh` with `SMOKE_EXTERNAL_PROVIDERS=1`. That manual smoke sends a normalized-audio URL to Xiaomi MiMo, so use only appropriate test audio and never commit real keys.
