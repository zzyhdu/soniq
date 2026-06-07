#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="$(mktemp -d "${TMPDIR:-/tmp}/soniq-fake-asr.XXXXXX")"
FAKE_ASR_LOG="$LOG_DIR/fake-asr.log"
FAKE_ASR_PORT_FILE="$LOG_DIR/fake-asr.port"
FAKE_ASR_PID=""

cleanup() {
  local exit_code=$?
  if [[ -n "$FAKE_ASR_PID" ]] && kill -0 "$FAKE_ASR_PID" 2>/dev/null; then
    kill "$FAKE_ASR_PID" 2>/dev/null || true
    wait "$FAKE_ASR_PID" 2>/dev/null || true
    if kill -0 "$FAKE_ASR_PID" 2>/dev/null; then
      kill -9 "$FAKE_ASR_PID" 2>/dev/null || true
      wait "$FAKE_ASR_PID" 2>/dev/null || true
    fi
  fi
  if [[ "$exit_code" -ne 0 ]]; then
    printf '[smoke-asr] failed; fake ASR log: %s\n' "$FAKE_ASR_LOG"
  else
    printf '[smoke-asr] passed; fake ASR log: %s\n' "$FAKE_ASR_LOG"
  fi
}
trap cleanup EXIT

python3 "$ROOT_DIR/scripts/smoke-openai-compatible-asr-fake-server.py" "$FAKE_ASR_PORT_FILE" >"$FAKE_ASR_LOG" 2>&1 &
FAKE_ASR_PID="$!"

for _ in {1..50}; do
  if [[ -s "$FAKE_ASR_PORT_FILE" ]]; then
    break
  fi
  if ! kill -0 "$FAKE_ASR_PID" 2>/dev/null; then
    printf '[smoke-asr] fake ASR exited early; log: %s\n' "$FAKE_ASR_LOG"
    exit 1
  fi
  sleep 0.1
done
if [[ ! -s "$FAKE_ASR_PORT_FILE" ]]; then
  printf '[smoke-asr] fake ASR did not write port file; log: %s\n' "$FAKE_ASR_LOG"
  exit 1
fi
FAKE_ASR_PORT="$(cat "$FAKE_ASR_PORT_FILE")"
printf '[smoke-asr] fake ASR listening on 127.0.0.1:%s\n' "$FAKE_ASR_PORT"

TRANSCRIPTION_PROVIDER=openai_compatible_asr \
TRANSCRIPTION_BASE_URL="http://127.0.0.1:$FAKE_ASR_PORT/v1" \
TRANSCRIPTION_API_KEY=test-api-key \
TRANSCRIPTION_MODEL=mimo-v2.5-asr \
TRANSCRIPTION_AUTH_HEADER=api-key \
TRANSCRIPTION_LANGUAGE=en \
EXPECTED_TRANSCRIPT_PROVIDER=openai_compatible_asr \
EXPECTED_TRANSCRIPT_MODEL=mimo-v2.5-asr \
"$ROOT_DIR/scripts/smoke-postgres-temporal.sh"
