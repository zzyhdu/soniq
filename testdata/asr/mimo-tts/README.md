# ASR Audio Test Fixtures from MiMo TTS

Synthetic Chinese dialogue fixtures for ASR quality review. Each top-level `.txt` file is the expected source text for all sibling audio variants with the same stem.

The generator parses speaker-prefixed lines such as `小王：...`, assigns each distinct speaker a stable speaker profile for that fixture, calls MiMo TTS per utterance, then concatenates utterances with short pauses. This better simulates a real conversation than asking one voice to read the entire dialogue transcript.

These fixtures intentionally include multiple **input** audio formats so the Soniq workflow exercises upload metadata, `ffprobe`, `ffmpeg` normalization, normalized metadata persistence, and ASR transcription. Do not treat these files as already-normalized output fixtures.

## Text Cases

| Stem | Purpose |
|---|---|
| `zh-meeting-dialogue` | Ordinary meeting conversation. |
| `zh-numbers-dates` | Numbers, dates, time, percentages. |
| `zh-product-terms` | Soniq, MiMo, Postgres, Temporal, and related technical terms. |
| `zh-casual-pauses` | Casual speech, pauses, and filler words. |

## Audio Variants

| Directory | Extension | MIME for upload | Purpose |
|---|---:|---|---|
| `wav-16k/` | `.wav` | `audio/wav` | Already close to normalized target; baseline ASR fixture. |
| `wav-24k/` | `.wav` | `audio/wav` | WAV with a different sample rate; should normalize to 16 kHz mono. |
| `mp3/` | `.mp3` | `audio/mpeg` | Common compressed upload format. |
| `flac/` | `.flac` | `audio/flac` | Lossless compressed upload format. |
| `ogg-opus/` | `.ogg` | `audio/ogg` | Opus-in-Ogg upload format. |

## Regenerating Fixtures

Regenerate these synthetic fixtures with Xiaomi MiMo TTS from the repository root:

```bash
set -a
. ./.env
set +a

python3 scripts/generate-mimo-tts-fixtures.py
```

The generator reads the top-level `.txt` files in this directory, parses speaker-prefixed dialogue lines, calls MiMo TTS (`mimo-v2.5-tts`) per utterance, concatenates utterances with short pauses, writes all audio variants, and refreshes `manifest.json`. It reads the API key from `TRANSCRIPTION_API_KEY` or `MIMO_API_KEY` but never writes secrets to the fixture files.

Optional overrides:

```bash
MIMO_TTS_MODEL=mimo-v2.5-tts MIMO_TTS_VOICE=茉莉 python3 scripts/generate-mimo-tts-fixtures.py
```

## Manual ASR Smoke Examples

Run with MP3 input to exercise decode + normalization:

```bash
set -a
. ./.env
set +a

SMOKE_AUDIO_FILE="$PWD/testdata/asr/mimo-tts/mp3/zh-product-terms.mp3" \
SMOKE_TITLE="MiMo TTS product terms MP3 review" \
SMOKE_LANGUAGE=zh \
TRANSCRIPTION_LANGUAGE=zh \
EXPECTED_TRANSCRIPT_LANGUAGE=zh \
API_URL=http://localhost:18080 \
API_ADDRESS=:18080 \
bash scripts/smoke-postgres-temporal.sh
```

Run with Ogg/Opus input:

```bash
SMOKE_AUDIO_FILE="$PWD/testdata/asr/mimo-tts/ogg-opus/zh-casual-pauses.ogg" \
SMOKE_TITLE="MiMo TTS casual pauses Ogg review" \
SMOKE_LANGUAGE=zh \
TRANSCRIPTION_LANGUAGE=zh \
EXPECTED_TRANSCRIPT_LANGUAGE=zh \
API_URL=http://localhost:18080 \
API_ADDRESS=:18080 \
bash scripts/smoke-postgres-temporal.sh
```

The workflow should persist the original upload metadata with the input MIME type, then persist `recording_normalized_audios` as WAV/PCM 16 kHz mono before ASR.

These fixtures are synthetic, non-private, and safe to commit. Real user audio should stay out of the repository.
