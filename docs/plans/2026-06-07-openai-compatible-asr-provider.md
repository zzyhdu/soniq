# OpenAI-Compatible ASR Provider Implementation Plan

> **For Hermes:** Execute this plan task-by-task. Keep repository docs tool-agnostic, use small commits, and ask before every commit. Do not commit real API keys, tokens, transcripts, or private audio.

**Goal:** Replace the local deterministic fake transcription provider with an optional real ASR provider that calls an OpenAI-compatible speech-recognition API, with Xiaomi MiMo `mimo-v2.5-asr` as the first target provider. Keep fake transcription as the default test/local-safe provider.

**Primary target:** Xiaomi MiMo ASR (`mimo-v2.5-asr`) via the MiMo OpenAI-compatible chat-completions API.

**Non-goals for this milestone:**

- Do not add local faster-whisper yet.
- Do not add real LLM summary providers.
- Do not add webhook/polling ASR providers.
- Do not add chunking or long-audio splitting.
- Do not add S3/MinIO storage support.
- Do not commit or print real `MIMO_API_KEY` / `TRANSCRIPTION_API_KEY` values.

---

## Current-state findings

The current J milestone pipeline is complete and verified:

```txt
POST /recordings/upload
  -> original audio stored under LOCAL_STORAGE_PATH
  -> Temporal RecordingProcessingWorkflow
  -> ProbeRecordingAudioActivity runs ffprobe against original audio
  -> NormalizeRecordingAudioActivity runs ffmpeg
  -> local normalized.wav artifact
  -> recording_normalized_audios row
  -> TranscribeRecordingAudioActivity requires normalized audio metadata
  -> fake_transcription reads normalized local path
  -> recording_transcripts + recording_transcript_segments
  -> fake_llm summary
  -> recording_summaries
  -> recordings.status = completed
```

Important existing boundaries:

- Transcription already consumes normalized audio, not the original upload.
- Normalized target is WAV, PCM signed 16-bit, mono, 16 kHz.
- Real persistence path is Postgres only; do not reintroduce `MemoryStore`.
- Worker registration currently injects deterministic fake transcription/summary providers.
- Config currently has `TRANSCRIPTION_PROVIDER`, but no transcription base URL/API key/model fields yet.
- `.env` / `.env.*` are ignored; `.env.example` is committed and should contain placeholders only.

---

## Xiaomi MiMo ASR documentation findings

Sources checked:

- `https://platform.xiaomimimo.com/docs/zh-CN/api/audio/Speech-Recognition`
- `https://platform.xiaomimimo.com/docs/zh-CN/usage-guide/Speech-Recognition`
- `https://platform.xiaomimimo.com/docs/zh-CN/quick-start/model`
- English equivalents under `/docs/en-US/...`

Key details found from Xiaomi MiMo docs:

| Item | Xiaomi MiMo ASR detail |
|---|---|
| Base URL | `https://api.xiaomimimo.com/v1` |
| Endpoint | `POST /chat/completions` |
| Model | `mimo-v2.5-asr` |
| API style | OpenAI chat-completions compatible with `input_audio` content part |
| Auth option 1 | Header `api-key: $MIMO_API_KEY` |
| Auth option 2 | `Authorization` header in Bearer mode |
| Content-Type | `application/json` |
| Audio input | Base64 data URL in `messages[].content[].input_audio.data` |
| Supported audio formats | `wav`, `mp3` |
| MIME types | `audio/wav`, `audio/mpeg`, `audio/mp3` |
| Base64 size limit | 10MB after base64 encoding |
| Language option | `asr_options.language`: `auto`, `zh`, `en` |
| Streaming | Supported, but not needed for first integration |

Example shape from docs:

```json
{
  "model": "mimo-v2.5-asr",
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "input_audio",
          "input_audio": {
            "data": "data:audio/wav;base64,$BASE64_AUDIO"
          }
        }
      ]
    }
  ],
  "asr_options": {
    "language": "zh"
  }
}
```

Important implication: although the docs call it OpenAI-compatible, this is **not** the classic OpenAI Whisper-style multipart endpoint:

```txt
POST /audio/transcriptions
multipart/form-data file=@normalized.wav
```

Instead, Xiaomi MiMo ASR uses:

```txt
POST /chat/completions
JSON body with base64 input_audio data URL
```

So the Soniq provider should be named and designed as a chat-completions audio-input ASR adapter, not only as a `/audio/transcriptions` multipart adapter.

---

## Proposed provider naming

Use provider name:

```txt
openai_compatible_asr
```

Why not `xiaomi_asr` only?

- Xiaomi MiMo is the first target, but the same adapter shape may also support other OpenAI-compatible chat-completions audio-input providers.
- Provider-specific defaults can still be documented for Xiaomi.

Optional future aliases:

```txt
mimo_asr
xiaomi_mimo_asr
```

These aliases can map to the same implementation later if useful.

---

## Proposed env/config contract

Add generic transcription provider settings:

```env
TRANSCRIPTION_PROVIDER=fake_transcription
TRANSCRIPTION_BASE_URL=https://api.xiaomimimo.com/v1
TRANSCRIPTION_API_KEY=replace-me
TRANSCRIPTION_MODEL=mimo-v2.5-asr
TRANSCRIPTION_AUTH_HEADER=api-key
TRANSCRIPTION_LANGUAGE=auto
TRANSCRIPTION_MAX_BASE64_BYTES=10485760
```

Notes:

- `TRANSCRIPTION_API_KEY` comes from `.env` at runtime and is never committed with a real value.
- For Xiaomi MiMo, default/recommended auth header mode is `api-key` because docs show `api-key: $MIMO_API_KEY` first.
- Also support `bearer` because Xiaomi docs state Bearer authentication is accepted and other OpenAI-compatible providers usually expect Bearer.
- Keep committed `.env.example` placeholders only.
- Consider accepting `MIMO_API_KEY` as a fallback alias only for Xiaomi convenience:

```txt
TRANSCRIPTION_API_KEY if set, otherwise MIMO_API_KEY
```

This gives the user the direct `.env` shape they expect without hard-coding provider-specific credentials everywhere.

### Default provider choice

Current config default is:

```txt
TRANSCRIPTION_PROVIDER=faster_whisper
```

For the actual current code, fake transcription is wired in worker regardless of this setting. During this milestone, make the default explicit and honest:

```txt
TRANSCRIPTION_PROVIDER=fake_transcription
```

Then `openai_compatible_asr` must be explicitly enabled in `.env` for real external calls.

---

## Proposed Go config additions

Modify:

```txt
backend/internal/config/config.go
backend/internal/config/config_test.go
.env.example
docs/development.md
docs/providers.md
```

Add fields:

```go
type Config struct {
    // existing fields...
    TranscriptionProvider string
    TranscriptionBaseURL  string
    TranscriptionAPIKey   string
    TranscriptionModel    string
    TranscriptionAuthHeader string
    TranscriptionLanguage string
    TranscriptionMaxBase64Bytes int64
}
```

Potential helper:

```go
func (c Config) NeedsTranscriptionAPIKeyForExternalProvider() bool
```

Rules:

- `fake_transcription` requires no API key.
- `openai_compatible_asr` requires API key when `PRIVACY_ALLOW_EXTERNAL_MODEL_PROVIDERS=true`.
- In private/offline mode, external providers should fail startup or provider construction clearly.

---

## Proposed provider abstraction

Current activity-level abstraction already exists:

```go
type TranscriptionProvider interface {
    Transcribe(ctx context.Context, input TranscriptionInput) (TranscriptionResult, error)
}
```

Keep this interface if possible.

Add implementation file, likely:

```txt
backend/internal/activities/openai_compatible_asr.go
backend/internal/activities/openai_compatible_asr_test.go
```

Or, if the project introduces a provider package:

```txt
backend/internal/providers/transcription/openai_compatible_asr.go
```

For a minimal milestone, keep it near existing fake provider activity code unless a provider package already exists.

### Provider input

Use existing normalized audio path from:

```go
TranscriptionInput.AudioPath
TranscriptionInput.Language
```

Provider should:

1. Read normalized audio file from disk.
2. Enforce max base64 payload size before making network request.
3. Encode as base64 data URL:

```txt
data:audio/wav;base64,<base64>
```

4. POST JSON to:

```txt
{base_url}/chat/completions
```

5. Include either:

```txt
api-key: <TRANSCRIPTION_API_KEY>
```

or:

```txt
Authorization header in bearer mode using the configured transcription key
```

6. Map response to `TranscriptionResult`.

---

## Response mapping strategy

The docs examples print `completion.model_dump_json()` but do not show a concrete response body in the retrieved page. First implementation should support the standard chat-completion shape:

```json
{
  "id": "...",
  "model": "mimo-v2.5-asr",
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "transcribed text"
      }
    }
  ]
}
```

Map to:

```go
TranscriptionResult{
    Provider: "openai_compatible_asr",
    Model: configuredModel,
    Language: requestedOrAutoLanguage,
    Text: content,
    Segments: []TranscriptionSegment{
        {
            ID: deterministicSegmentID(...),
            StartSeconds: 0,
            EndSeconds: 0,
            Text: content,
        },
    },
    RawResultJSON: rawResponse,
}
```

Because Xiaomi docs do not expose timestamps/segments in the retrieved text, first implementation should store one whole-text segment. If Xiaomi returns richer segment metadata, add mapping in a follow-up task after observing real response JSON in a manual smoke.

---

## Testing strategy

Do not call Xiaomi in automated tests.

Use `httptest.Server` to verify:

- URL path is `/chat/completions`.
- Method is `POST`.
- Auth header is `api-key` or `Authorization: Bearer` depending config.
- Content-Type is `application/json`.
- Body contains:
  - `model`;
  - `messages[0].role=user`;
  - `messages[0].content[0].type=input_audio`;
  - `messages[0].content[0].input_audio.data` data URL;
  - `asr_options.language`.
- Normalized WAV file bytes are base64-encoded correctly.
- Non-2xx responses return errors with status and body context, but not API key.
- Invalid JSON returns useful error.
- Empty choices/content returns useful error.
- Oversized base64 payload fails before HTTP request.

Manual Xiaomi smoke can be opt-in only:

```bash
TRANSCRIPTION_PROVIDER=openai_compatible_asr \
TRANSCRIPTION_BASE_URL=https://api.xiaomimimo.com/v1 \
TRANSCRIPTION_API_KEY="$MIMO_API_KEY" \
TRANSCRIPTION_MODEL=mimo-v2.5-asr \
TRANSCRIPTION_AUTH_HEADER=api-key \
TRANSCRIPTION_LANGUAGE=zh \
API_URL=http://localhost:18080 API_ADDRESS=:18080 \
bash scripts/smoke-postgres-temporal.sh
```

Do not print the API key in logs.

---

## Task breakdown

## Task K1: Plan Xiaomi/OpenAI-compatible ASR provider

**Objective:** Capture API findings, env/config contract, test strategy, and implementation sequence.

**Files:**

- Create: `docs/plans/2026-06-07-openai-compatible-asr-provider.md`

**Verification:**

```bash
git status --short
git diff --check
```

**Suggested commit:**

```txt
docs: add openai compatible asr provider plan
```

---

## Task K2: Add transcription provider config RED tests

**Objective:** Define config fields and `.env.example` placeholders for external ASR.

**Files:**

- Modify: `backend/internal/config/config_test.go`
- Modify: `.env.example`

**RED expectations:**

- Missing config fields on `Config`.
- Missing env parsing for `TRANSCRIPTION_BASE_URL`, `TRANSCRIPTION_API_KEY`, `TRANSCRIPTION_MODEL`, `TRANSCRIPTION_AUTH_HEADER`, `TRANSCRIPTION_LANGUAGE`, `TRANSCRIPTION_MAX_BASE64_BYTES`.

**Tests:**

- Defaults are fake/local-safe.
- Env overrides load correctly.
- `TRANSCRIPTION_API_KEY` is loaded from `.env`/environment.
- `MIMO_API_KEY` can be used as fallback alias only when `TRANSCRIPTION_API_KEY` is empty.

---

## Task K3: Implement transcription provider config

**Objective:** Make K2 GREEN.

**Files:**

- Modify: `backend/internal/config/config.go`
- Modify: `backend/internal/config/config_test.go`
- Modify: `.env.example`
- Optionally modify: `docs/development.md`

**Verification:**

```bash
cd backend && go test ./internal/config -v
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
feat: add transcription provider configuration
```

---

## Task K4: Add OpenAI-compatible ASR client RED tests

**Objective:** Define HTTP request/response behavior for Xiaomi-compatible ASR without calling real Xiaomi.

**Files:**

- Create: `backend/internal/activities/openai_compatible_asr_test.go`

**RED expectations:**

- Missing provider type/config types.
- Missing `Transcribe` implementation.

**Test cases:**

1. Sends POST `/chat/completions` with JSON audio input.
2. Supports `api-key` auth header.
3. Supports `Authorization: Bearer` auth header.
4. Maps standard chat response content to transcript text.
5. Stores raw response JSON.
6. Rejects oversized base64 payload before request.
7. Returns useful errors for non-2xx, invalid JSON, empty choices, missing audio path.
8. Does not include API key in error messages.

---

## Task K5: Implement OpenAI-compatible ASR client/provider

**Objective:** Make K4 GREEN.

**Files:**

- Create: `backend/internal/activities/openai_compatible_asr.go`
- Modify existing activity/provider wiring only if needed for compile.

**Implementation notes:**

- Use `http.Client` injected or configurable for tests.
- Use context-aware requests.
- Read normalized audio file from `TranscriptionInput.AudioPath`.
- Detect content type from configured/current normalized target; first implementation can use `audio/wav` because normalization guarantees WAV.
- Preserve response body in `RawResultJSON` with reasonable size limits.
- Redact API key from errors.

**Verification:**

```bash
cd backend && go test ./internal/activities -run 'OpenAICompatibleASR|TranscribeRecordingAudio' -v
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
feat: add openai compatible asr provider
```

---

## Task K6: Add worker provider-selection RED tests

**Objective:** Define worker behavior when `TRANSCRIPTION_PROVIDER=openai_compatible_asr`.

**Files:**

- Modify: `backend/cmd/worker/main_test.go`
- Possibly modify: `backend/cmd/worker/main.go`

**Expected behavior:**

- Default worker keeps fake transcription provider for local safe operation.
- `openai_compatible_asr` config constructs real provider with env-loaded base URL/API key/model/auth header/language.
- Missing API key returns a clear startup/configuration error when external provider is selected.
- Private/offline mode can reject external provider if configured.

---

## Task K7: Implement worker provider selection

**Objective:** Make K6 GREEN and wire real ASR provider into `RecordingProcessingActivities`.

**Files:**

- Modify: `backend/cmd/worker/main.go`
- Modify: `backend/cmd/worker/main_test.go`
- Possibly modify: `backend/internal/config/config.go`

**Verification:**

```bash
cd backend && go test ./cmd/worker ./internal/activities ./internal/config -v
cd backend && go test ./...
git diff --check
```

**Suggested commit:**

```txt
feat: wire configurable transcription provider
```

---

## Task K8: Add fake-server smoke for OpenAI-compatible ASR

**Objective:** Prove the whole workflow can use an HTTP ASR provider without hitting Xiaomi.

**Files:**

- Modify or add: `scripts/smoke-postgres-temporal.sh`
- Or add separate script: `scripts/smoke-openai-compatible-asr-fake.sh`

**Approach:**

- Start a tiny local fake ASR HTTP server during smoke.
- Configure worker:

```env
TRANSCRIPTION_PROVIDER=openai_compatible_asr
TRANSCRIPTION_BASE_URL=http://127.0.0.1:<port>/v1
TRANSCRIPTION_API_KEY=test-api-key
TRANSCRIPTION_MODEL=mimo-v2.5-asr
TRANSCRIPTION_AUTH_HEADER=api-key
TRANSCRIPTION_LANGUAGE=zh
```

- Fake server asserts request body/auth and returns chat-completion-style response.
- Existing smoke verifies transcript rows persisted.
- Add a DB assertion that transcript provider/model reflect the real provider path:

```txt
provider = openai_compatible_asr
model = mimo-v2.5-asr
```

**Suggested commit:**

```txt
test: smoke openai compatible asr provider
```

---

## Task K9: Document optional Xiaomi manual smoke

**Objective:** Document how the user can run a real Xiaomi MiMo ASR smoke locally with `.env`, without committing secrets.

**Files:**

- Modify: `docs/development.md`
- Modify: `docs/providers.md`
- Modify: `.env.example`

**Manual smoke example:**

```bash
# .env local only; do not commit
TRANSCRIPTION_PROVIDER=openai_compatible_asr
TRANSCRIPTION_BASE_URL=https://api.xiaomimimo.com/v1
TRANSCRIPTION_API_KEY=$MIMO_API_KEY
TRANSCRIPTION_MODEL=mimo-v2.5-asr
TRANSCRIPTION_AUTH_HEADER=api-key
TRANSCRIPTION_LANGUAGE=zh
```

Then:

```bash
API_URL=http://localhost:18080 API_ADDRESS=:18080 make smoke-postgres-temporal
```

**Suggested commit:**

```txt
docs: document xiaomi mimo asr configuration
```

---

## Task K10: Cleanup/review pass

**Objective:** Confirm the milestone did not compromise safety or test stability.

Checklist:

- No real API keys committed.
- `.env` remains ignored.
- Automated tests use fake HTTP server, not Xiaomi.
- Errors redact API keys.
- Fake transcription remains available and is the default safe provider.
- OpenAI-compatible provider reads normalized WAV input.
- No `MemoryStore` reintroduced.
- Full tests pass.

**Verification:**

```bash
! grep -RIn "replace-with-real-secret" . --exclude-dir=.git --exclude=.env --exclude=.env.*
cd backend && go test ./...
git diff --check
git status --short
```

**Suggested commit if cleanup changes are needed:**

```txt
refactor: clean openai compatible asr provider integration
```

---

## Acceptance criteria

The K milestone is complete when:

1. The default local smoke still works without external credentials.
2. `TRANSCRIPTION_PROVIDER=openai_compatible_asr` can be enabled via `.env`.
3. API key is loaded from env (`TRANSCRIPTION_API_KEY`, optionally `MIMO_API_KEY` fallback) and never committed.
4. Automated tests verify the Xiaomi-compatible request shape using `httptest` or a local fake server.
5. Worker can inject the real provider into `TranscribeRecordingAudio`.
6. Transcription reads normalized audio, not original audio.
7. Transcript rows preserve provider/model/raw JSON.
8. Docs explain Xiaomi MiMo config and manual smoke without exposing secrets.

---

## Open questions for implementation

These are not blockers for K1, but should be resolved during K4/K9:

1. What exact response JSON does Xiaomi MiMo ASR return for non-streaming calls? Retrieved docs show request examples but not a concrete response body. First implementation should support standard chat-completion content and preserve raw JSON; manual smoke can refine mapping.
2. Does Xiaomi return segment timestamps? If not, first version should store one whole-text segment.
3. Is `Authorization: Bearer` fully equivalent to `api-key` for MiMo ASR in practice? Docs say both are accepted; tests should support both, but docs can recommend `api-key` for Xiaomi.
4. Is the 10MB limit counted after Base64 encoding or raw file? Docs say Base64-encoded string size upper limit is 10MB; enforce encoded payload limit before HTTP request.
