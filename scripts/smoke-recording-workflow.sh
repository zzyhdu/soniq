#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://localhost:8080}"
SMOKE_WORKSPACE_ID="${SMOKE_WORKSPACE_ID:-wsp_default}"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/soniq-workflow-smoke.XXXXXX")"
AUDIO_FILE="$TMP_DIR/weekly.wav"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

ffmpeg -hide_banner -loglevel error -f lavfi -i sine=frequency=1000:duration=1 -ac 1 -ar 16000 -c:a pcm_s16le "$AUDIO_FILE"

response="$(curl -fsS -X POST "${API_URL}/workspaces/${SMOKE_WORKSPACE_ID}/recordings/upload" \
  -F title='Weekly sync' \
  -F workflow_type=meeting \
  -F language=en \
  -F "audio=@${AUDIO_FILE};filename=weekly.wav;type=audio/wav")"

printf '%s\n' "$response"

recording_id="$(printf '%s\n' "$response" | python3 -c 'import json,sys; print(json.load(sys.stdin)["recording"]["id"])')"
workspace_id="$(printf '%s\n' "$response" | python3 -c 'import json,sys; print(json.load(sys.stdin)["recording"]["workspace_id"])')"
processing_enqueued="$(printf '%s\n' "$response" | python3 -c 'import json,sys; print(json.load(sys.stdin)["processing_enqueued"])')"
printf 'Workspace: %s\n' "$workspace_id"
printf 'Processing enqueued: %s\n' "$processing_enqueued"
printf 'Expected Temporal workflow ID: recording-processing-%s\n' "$recording_id"
