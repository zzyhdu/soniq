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

- `local` filesystem provider for development.

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

- Synchronous: OpenAI, Groq Whisper, local faster-whisper.
- Asynchronous: AssemblyAI, Deepgram batch, Aliyun ASR, Tencent ASR, Volcengine ASR, iFlytek long audio.

Current implementation:

- `fake_transcription`, a deterministic local provider used to verify workflow/activity/persistence wiring without credentials.

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

- `fake_llm`, a deterministic local summary provider used to verify workflow/activity/persistence wiring without credentials.

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
