#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://localhost:8080}"
SMOKE_WORKSPACE_ID="${SMOKE_WORKSPACE_ID:-}"
SMOKE_EMAIL="${SMOKE_EMAIL:-smoke@local.soniq}"
SMOKE_DISPLAY_NAME="${SMOKE_DISPLAY_NAME:-Smoke Tester}"
SMOKE_PASSWORD="${SMOKE_PASSWORD:-correct horse smoke}"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/soniq-workflow-smoke.XXXXXX")"
AUDIO_FILE="$TMP_DIR/weekly.wav"
COOKIE_JAR="$TMP_DIR/cookies.txt"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

auth_json() {
  python3 -c 'import json,sys
payload = {"email": sys.argv[1], "password": sys.argv[2]}
if sys.argv[3]:
    payload["display_name"] = sys.argv[3]
print(json.dumps(payload))' "$SMOKE_EMAIL" "$SMOKE_PASSWORD" "$SMOKE_DISPLAY_NAME"
}

extract_first_workspace_id() {
  python3 -c 'import json,sys
data = json.load(sys.stdin)
workspaces = data.get("workspaces") or []
if not workspaces:
    raise SystemExit("no workspaces returned for authenticated smoke user")
print(workspaces[0]["id"])'
}

auth_response="$TMP_DIR/auth-response.json"
status="$(auth_json | curl -sS -o "$auth_response" -w "%{http_code}" -c "$COOKIE_JAR" \
  -H 'Content-Type: application/json' \
  -X POST "${API_URL}/auth/signup" \
  --data-binary @-)"
if [[ "$status" == "409" ]]; then
  status="$(auth_json | curl -sS -o "$auth_response" -w "%{http_code}" -c "$COOKIE_JAR" \
    -H 'Content-Type: application/json' \
    -X POST "${API_URL}/auth/signin" \
    --data-binary @-)"
fi
if [[ "$status" != "200" && "$status" != "201" ]]; then
  cat "$auth_response"
  exit 1
fi
if [[ -z "$SMOKE_WORKSPACE_ID" ]]; then
  SMOKE_WORKSPACE_ID="$(curl -fsS -b "$COOKIE_JAR" "${API_URL}/workspaces" | extract_first_workspace_id)"
fi

ffmpeg -hide_banner -loglevel error -f lavfi -i sine=frequency=1000:duration=1 -ac 1 -ar 16000 -c:a pcm_s16le "$AUDIO_FILE"

response="$(curl -fsS -b "$COOKIE_JAR" -X POST "${API_URL}/workspaces/${SMOKE_WORKSPACE_ID}/recordings/upload" \
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
