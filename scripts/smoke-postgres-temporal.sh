#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-compose.temporal.yml}"
API_URL="${API_URL:-http://localhost:8080}"
API_ADDRESS="${API_ADDRESS:-:8080}"
AUTH_SESSION_TTL_HOURS="${AUTH_SESSION_TTL_HOURS:-720}"
AUTH_COOKIE_SECURE="${AUTH_COOKIE_SECURE:-false}"
TEMPORAL_NAMESPACE="${TEMPORAL_NAMESPACE:-default}"
TEMPORAL_TASK_QUEUE="${TEMPORAL_TASK_QUEUE:-soniq-audio-pipeline}"
POSTGRES_USER="${POSTGRES_USER:-soniq_user}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-soniq_password}"
POSTGRES_DB="${POSTGRES_DB:-soniq}"
POSTGRES_DSN="${POSTGRES_DSN:-postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@localhost:5432/$POSTGRES_DB?sslmode=disable}"
SMOKE_WORKSPACE_ID="${SMOKE_WORKSPACE_ID:-}"
SMOKE_EMAIL="${SMOKE_EMAIL:-smoke@local.soniq}"
SMOKE_DISPLAY_NAME="${SMOKE_DISPLAY_NAME:-Smoke Tester}"
SMOKE_PASSWORD="${SMOKE_PASSWORD:-correct horse smoke}"
STORAGE_PROVIDER="${STORAGE_PROVIDER:-local}"
LOCAL_STORAGE_PATH="${LOCAL_STORAGE_PATH:-$ROOT_DIR/var/uploads/smoke}"
SMOKE_EXTERNAL_PROVIDERS="${SMOKE_EXTERNAL_PROVIDERS:-0}"
TRANSCRIPTION_BASE_URL="${TRANSCRIPTION_BASE_URL:-https://api.xiaomimimo.com/v1}"
TRANSCRIPTION_API_KEY="${TRANSCRIPTION_API_KEY:-}"
MIMO_API_KEY="${MIMO_API_KEY:-}"
TRANSCRIPTION_MODEL="${TRANSCRIPTION_MODEL:-mimo-v2.5-asr}"
TRANSCRIPTION_AUTH_HEADER="${TRANSCRIPTION_AUTH_HEADER:-api-key}"
TRANSCRIPTION_LANGUAGE="${TRANSCRIPTION_LANGUAGE:-auto}"
TRANSCRIPTION_MAX_BASE64_BYTES="${TRANSCRIPTION_MAX_BASE64_BYTES:-10485760}"
DASHSCOPE_BASE_URL="${DASHSCOPE_BASE_URL:-https://dashscope.aliyuncs.com/api/v1}"
DASHSCOPE_API_KEY="${DASHSCOPE_API_KEY:-}"
DASHSCOPE_ASR_MODEL="${DASHSCOPE_ASR_MODEL:-paraformer-v2}"
LLM_BASE_URL="${LLM_BASE_URL:-https://dashscope.aliyuncs.com/compatible-mode/v1}"
LLM_API_KEY="${LLM_API_KEY:-${DASHSCOPE_API_KEY:-}}"
LLM_MODEL="${LLM_MODEL:-qwen-plus}"
EXPECTED_TRANSCRIPT_PROVIDER="${EXPECTED_TRANSCRIPT_PROVIDER:-}"
EXPECTED_TRANSCRIPT_MODEL="${EXPECTED_TRANSCRIPT_MODEL:-}"
EXPECTED_SUMMARY_PROVIDER="${EXPECTED_SUMMARY_PROVIDER:-}"
EXPECTED_SUMMARY_MODEL="${EXPECTED_SUMMARY_MODEL:-}"
SMOKE_DOWN="${SMOKE_DOWN:-0}"

if [[ "$SMOKE_EXTERNAL_PROVIDERS" == "1" ]]; then
  TRANSCRIPTION_PROVIDER="${TRANSCRIPTION_PROVIDER:-fake_transcription}"
  LLM_PROVIDER="${LLM_PROVIDER:-fake_llm}"
else
  TRANSCRIPTION_PROVIDER="fake_transcription"
  LLM_PROVIDER="fake_llm"
fi

API_PID=""
WORKER_PID=""
LOG_DIR="$(mktemp -d "${TMPDIR:-/tmp}/soniq-smoke.XXXXXX")"
API_LOG="$LOG_DIR/api.log"
WORKER_LOG="$LOG_DIR/worker.log"
COOKIE_JAR="$LOG_DIR/cookies.txt"
case "$LOCAL_STORAGE_PATH" in
  /*) EFFECTIVE_LOCAL_STORAGE_PATH="$LOCAL_STORAGE_PATH" ;;
  *) EFFECTIVE_LOCAL_STORAGE_PATH="$ROOT_DIR/$LOCAL_STORAGE_PATH" ;;
esac
EXPECTED_TRANSCRIPT_LANGUAGE="${EXPECTED_TRANSCRIPT_LANGUAGE:-}"

log() {
  printf '[smoke] %s\n' "$*"
}

stop_process_tree() {
  local label="$1"
  local pid="$2"
  if [[ -z "$pid" ]] || ! kill -0 "$pid" 2>/dev/null; then
    return 0
  fi
  log "stopping $label process group $pid"
  kill -- -"$pid" 2>/dev/null || kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
  if kill -0 "$pid" 2>/dev/null; then
    kill -9 -- -"$pid" 2>/dev/null || kill -9 "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
}

cleanup() {
  local exit_code=$?
  stop_process_tree "API" "$API_PID"
  stop_process_tree "worker" "$WORKER_PID"
  if [[ "$SMOKE_DOWN" == "1" ]]; then
    log "SMOKE_DOWN=1, stopping Compose services"
    (cd "$ROOT_DIR" && docker compose -f "$COMPOSE_FILE" down) || true
  fi
  if [[ "$exit_code" -ne 0 ]]; then
    log "failed; logs are available at:"
    log "  API:    $API_LOG"
    log "  worker: $WORKER_LOG"
  else
    log "passed; logs are available at $LOG_DIR"
  fi
}
trap cleanup EXIT

wait_for_command() {
  local description="$1"
  local attempts="$2"
  shift 2
  local i
  for ((i = 1; i <= attempts; i++)); do
    if "$@" >/dev/null 2>&1; then
      log "$description is ready"
      return 0
    fi
    sleep 1
  done
  log "timed out waiting for $description"
  return 1
}

wait_for_api() {
  wait_for_command "API $API_URL/healthz" 60 curl -fsS "$API_URL/healthz"
}

start_worker() {
  log "starting worker; log: $WORKER_LOG"
  setsid bash -c '
    cd "$1"
    TEMPORAL_NAMESPACE="$2" \
    TEMPORAL_TASK_QUEUE="$3" \
    POSTGRES_DSN="$4" \
    STORAGE_PROVIDER="$5" \
    LOCAL_STORAGE_PATH="$6" \
    TRANSCRIPTION_PROVIDER="$7" \
    TRANSCRIPTION_BASE_URL="$8" \
    TRANSCRIPTION_API_KEY="$9" \
    MIMO_API_KEY="${10}" \
    TRANSCRIPTION_MODEL="${11}" \
    TRANSCRIPTION_AUTH_HEADER="${12}" \
    TRANSCRIPTION_LANGUAGE="${13}" \
    TRANSCRIPTION_MAX_BASE64_BYTES="${14}" \
    DASHSCOPE_BASE_URL="${15}" \
    DASHSCOPE_API_KEY="${16}" \
    DASHSCOPE_ASR_MODEL="${17}" \
    LLM_PROVIDER="${18}" \
    LLM_BASE_URL="${19}" \
    LLM_API_KEY="${20}" \
    LLM_MODEL="${21}" \
    make worker
  ' _ "$ROOT_DIR" "$TEMPORAL_NAMESPACE" "$TEMPORAL_TASK_QUEUE" "$POSTGRES_DSN" "$STORAGE_PROVIDER" "$EFFECTIVE_LOCAL_STORAGE_PATH" "$TRANSCRIPTION_PROVIDER" "$TRANSCRIPTION_BASE_URL" "$TRANSCRIPTION_API_KEY" "$MIMO_API_KEY" "$TRANSCRIPTION_MODEL" "$TRANSCRIPTION_AUTH_HEADER" "$TRANSCRIPTION_LANGUAGE" "$TRANSCRIPTION_MAX_BASE64_BYTES" "$DASHSCOPE_BASE_URL" "$DASHSCOPE_API_KEY" "$DASHSCOPE_ASR_MODEL" "$LLM_PROVIDER" "$LLM_BASE_URL" "$LLM_API_KEY" "$LLM_MODEL" >"$WORKER_LOG" 2>&1 &
  WORKER_PID=$!
  sleep 2
  if ! kill -0 "$WORKER_PID" 2>/dev/null; then
    log "worker exited early"
    return 1
  fi
}

start_api() {
  log "starting API at $API_ADDRESS; log: $API_LOG"
  : >"$API_LOG"
  setsid bash -c '
    cd "$1"
    API_ADDRESS="$2" \
    POSTGRES_DSN="$3" \
    TEMPORAL_NAMESPACE="$4" \
    TEMPORAL_TASK_QUEUE="$5" \
    STORAGE_PROVIDER="$6" \
    LOCAL_STORAGE_PATH="$7" \
    AUTH_SESSION_TTL_HOURS="$8" \
    AUTH_COOKIE_SECURE="$9" \
    make api
  ' _ "$ROOT_DIR" "$API_ADDRESS" "$POSTGRES_DSN" "$TEMPORAL_NAMESPACE" "$TEMPORAL_TASK_QUEUE" "$STORAGE_PROVIDER" "$EFFECTIVE_LOCAL_STORAGE_PATH" "$AUTH_SESSION_TTL_HOURS" "$AUTH_COOKIE_SECURE" >>"$API_LOG" 2>&1 &
  API_PID=$!
  wait_for_api
}

authenticate_api() {
  local response_file status workspaces_response
  response_file="$LOG_DIR/auth-response.json"

  log "creating or signing in smoke user $SMOKE_EMAIL"
  status="$(auth_json | curl -sS -o "$response_file" -w "%{http_code}" -c "$COOKIE_JAR" \
    -H 'Content-Type: application/json' \
    -X POST "$API_URL/auth/signup" \
    --data-binary @-)"
  if [[ "$status" == "201" ]]; then
    log "created smoke user"
  elif [[ "$status" == "409" ]]; then
    status="$(auth_json | curl -sS -o "$response_file" -w "%{http_code}" -c "$COOKIE_JAR" \
      -H 'Content-Type: application/json' \
      -X POST "$API_URL/auth/signin" \
      --data-binary @-)"
    if [[ "$status" != "200" ]]; then
      log "sign in failed with HTTP $status"
      cat "$response_file"
      return 1
    fi
    log "signed in existing smoke user"
  else
    log "signup failed with HTTP $status"
    cat "$response_file"
    return 1
  fi

  workspaces_response="$(curl -fsS -b "$COOKIE_JAR" "$API_URL/workspaces")"
  if [[ -z "$SMOKE_WORKSPACE_ID" ]]; then
    SMOKE_WORKSPACE_ID="$(printf '%s\n' "$workspaces_response" | extract_first_workspace_id)"
  fi
  log "using smoke workspace $SMOKE_WORKSPACE_ID"
}

stop_api() {
  stop_process_tree "API" "$API_PID"
  API_PID=""
}

content_type_for_file() {
  local path="$1"
  case "${path##*.}" in
    wav|WAV) printf 'audio/wav' ;;
    mp3|MP3) printf 'audio/mpeg' ;;
    flac|FLAC) printf 'audio/flac' ;;
    ogg|OGA|opus|OPUS) printf 'audio/ogg' ;;
    m4a|M4A) printf 'audio/mp4' ;;
    *) printf 'application/octet-stream' ;;
  esac
}

extract_recording_id() {
  python3 -c 'import json,sys; print(json.load(sys.stdin)["recording"]["id"])'
}

extract_json_field() {
  local field="$1"
  python3 -c 'import json,sys
value = json.load(sys.stdin)
for part in sys.argv[1].split("."):
    value = value[part]
print(value)' "$field"
}

extract_first_workspace_id() {
  python3 -c 'import json,sys
data = json.load(sys.stdin)
workspaces = data.get("workspaces") or []
if not workspaces:
    raise SystemExit("no workspaces returned for authenticated smoke user")
print(workspaces[0]["id"])'
}

csrf_token() {
  awk '$0 !~ /^#/ && $6 == "soniq_csrf" { token = $7 } END { if (token == "") exit 1; print token }' "$COOKIE_JAR"
}

auth_json() {
  python3 -c 'import json,sys
payload = {"email": sys.argv[1], "password": sys.argv[2]}
if sys.argv[3]:
    payload["display_name"] = sys.argv[3]
print(json.dumps(payload))' "$SMOKE_EMAIL" "$SMOKE_PASSWORD" "$SMOKE_DISPLAY_NAME"
}

assert_json_field_equals() {
  local json="$1"
  local field="$2"
  local expected="$3"
  local actual
  actual="$(printf '%s\n' "$json" | extract_json_field "$field")"
  if [[ "$actual" != "$expected" ]]; then
    log "expected JSON field $field=$expected, got $actual"
    return 1
  fi
}

apply_application_migrations() {
  log "applying missing Soniq application migrations"
  POSTGRES_DSN="$POSTGRES_DSN" make migrate
}

assert_uploaded_object() {
  local object_key="$1"
  local expected_size_bytes="$2"
  local object_path="$EFFECTIVE_LOCAL_STORAGE_PATH/$object_key"
  if [[ ! -f "$object_path" ]]; then
    log "uploaded object does not exist: $object_path"
    return 1
  fi
  local actual_size_bytes
  actual_size_bytes="$(wc -c <"$object_path" | tr -d ' ')"
  if [[ "$actual_size_bytes" != "$expected_size_bytes" ]]; then
    log "uploaded object size mismatch: $actual_size_bytes, want $expected_size_bytes"
    return 1
  fi
}

assert_recording_audio_metadata_in_db() {
  local recording_id="$1"
  local expected_object_key="$2"
  local expected_content_type="$3"
  local expected_size_bytes="$4"
  local row
  row="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -AtF $'\t' -c "SELECT audio_object_key, audio_content_type, audio_size_bytes FROM recordings WHERE id = '$recording_id'")"
  if [[ "$row" != "$expected_object_key"$'\t'"$expected_content_type"$'\t'"$expected_size_bytes" ]]; then
    log "unexpected DB audio metadata row: $row"
    return 1
  fi
}

assert_recording_workspace_in_db() {
  local recording_id="$1"
  local expected_workspace_id="$2"
  local actual_workspace_id
  actual_workspace_id="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "SELECT workspace_id FROM recordings WHERE id = '$recording_id'")"
  if [[ "$actual_workspace_id" != "$expected_workspace_id" ]]; then
    log "unexpected DB recording workspace_id: $actual_workspace_id, want $expected_workspace_id"
    return 1
  fi
}

assert_recording_status_in_db() {
  local recording_id="$1"
  local expected_status="$2"
  local row actual_status completed_at_set failure_reason
  row="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -AtF $'\t' -c "SELECT status, (completed_at IS NOT NULL), failure_reason FROM recordings WHERE id = '$recording_id'")"
  IFS=$'\t' read -r actual_status completed_at_set failure_reason <<<"$row"
  if [[ "$actual_status" != "$expected_status" ]]; then
    log "unexpected DB recording status: $actual_status, want $expected_status"
    return 1
  fi
  if [[ "$expected_status" == "completed" && ( "$completed_at_set" != "t" || -n "$failure_reason" ) ]]; then
    log "unexpected DB completion metadata: completed_at_set=$completed_at_set failure_reason=$failure_reason"
    return 1
  fi
}

assert_recording_audio_probe_in_db() {
  local recording_id="$1"
  local row
  row="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -AtF $'\t' -c "SELECT format_name, codec_name, sample_rate, channels, (duration_seconds > 0), jsonb_typeof(raw_probe_json) FROM recording_audio_probes WHERE recording_id = '$recording_id'")"
  if [[ -z "$row" ]]; then
    log "recording audio probe row missing for $recording_id"
    return 1
  fi

  IFS=$'\t' read -r format_name codec_name sample_rate channels has_duration raw_json_type <<<"$row"
  if [[ -z "$format_name" || -z "$codec_name" || "$sample_rate" -le 0 || "$channels" -le 0 || "$has_duration" != "t" || "$raw_json_type" != "object" ]]; then
    log "unexpected DB audio probe row: $row"
    return 1
  fi
}

assert_recording_normalized_audio_in_db() {
  local recording_id="$1"
  local row object_key content_type size_bytes format_name codec_name sample_rate channels normalized_at_set object_path actual_size_bytes
  row="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -AtF $'\t' -c "SELECT object_key, content_type, size_bytes, format_name, codec_name, sample_rate, channels, (normalized_at IS NOT NULL) FROM recording_normalized_audios WHERE recording_id = '$recording_id'")"
  if [[ -z "$row" ]]; then
    log "recording normalized audio row missing for $recording_id"
    return 1
  fi

  IFS=$'\t' read -r object_key content_type size_bytes format_name codec_name sample_rate channels normalized_at_set <<<"$row"
  if [[ -z "$object_key" || "$object_key" != */normalized.wav || "$content_type" != "audio/wav" || "$size_bytes" -le 0 || "$format_name" != "wav" || "$codec_name" != "pcm_s16le" || "$sample_rate" != "16000" || "$channels" != "1" || "$normalized_at_set" != "t" ]]; then
    log "unexpected DB normalized audio row: $row"
    return 1
  fi

  object_path="$EFFECTIVE_LOCAL_STORAGE_PATH/$object_key"
  if [[ ! -f "$object_path" ]]; then
    log "normalized audio object does not exist: $object_path"
    return 1
  fi
  actual_size_bytes="$(wc -c <"$object_path" | tr -d ' ')"
  if [[ "$actual_size_bytes" != "$size_bytes" ]]; then
    log "normalized audio object size mismatch: $actual_size_bytes, want $size_bytes"
    return 1
  fi
}

assert_recording_transcript_summary_mind_map_in_db() {
  local recording_id="$1"
  local transcript_row segment_count summary_row mind_map_row
  transcript_row="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -AtF $'\t' -c "SELECT provider, model, language, (length(text) > 0), jsonb_typeof(raw_result_json) FROM recording_transcripts WHERE recording_id = '$recording_id'")"
  if [[ -z "$transcript_row" ]]; then
    log "recording transcript row missing for $recording_id"
    return 1
  fi

  IFS=$'\t' read -r transcript_provider transcript_model transcript_language transcript_has_text transcript_raw_json_type <<<"$transcript_row"
  if [[ -z "$transcript_provider" || -z "$transcript_model" || "$transcript_has_text" != "t" || "$transcript_raw_json_type" != "object" ]]; then
    log "unexpected DB transcript row: $transcript_row"
    return 1
  fi
  if [[ -n "$EXPECTED_TRANSCRIPT_LANGUAGE" && "$transcript_language" != "$EXPECTED_TRANSCRIPT_LANGUAGE" ]]; then
    log "unexpected DB transcript language: $transcript_language, want $EXPECTED_TRANSCRIPT_LANGUAGE"
    return 1
  fi

  segment_count="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "SELECT count(*) FROM recording_transcript_segments WHERE recording_id = '$recording_id'")"
  if [[ "$segment_count" -lt 1 ]]; then
    log "recording transcript segments missing for $recording_id"
    return 1
  fi

  summary_row="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -AtF $'\t' -c "SELECT provider, model, type, (length(overview) > 0 OR length(content_markdown) > 0), jsonb_typeof(raw_result_json) FROM recording_summaries WHERE recording_id = '$recording_id'")"
  if [[ -z "$summary_row" ]]; then
    log "recording summary row missing for $recording_id"
    return 1
  fi

  IFS=$'\t' read -r summary_provider summary_model summary_type summary_has_content summary_raw_json_type <<<"$summary_row"
  if [[ -z "$summary_provider" || -z "$summary_model" || "$summary_type" != "meeting" || "$summary_has_content" != "t" || "$summary_raw_json_type" != "object" ]]; then
    log "unexpected DB summary row: $summary_row"
    return 1
  fi
  if [[ -n "$EXPECTED_SUMMARY_PROVIDER" && "$summary_provider" != "$EXPECTED_SUMMARY_PROVIDER" ]]; then
    log "unexpected DB summary provider: $summary_provider, want $EXPECTED_SUMMARY_PROVIDER"
    return 1
  fi
  if [[ -n "$EXPECTED_SUMMARY_MODEL" && "$summary_model" != "$EXPECTED_SUMMARY_MODEL" ]]; then
    log "unexpected DB summary model: $summary_model, want $EXPECTED_SUMMARY_MODEL"
    return 1
  fi

  mind_map_row="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -AtF $'\t' -c "SELECT provider, model, (length(title) > 0), jsonb_typeof(root_json), (length(content_markdown) > 0), jsonb_typeof(raw_result_json) FROM recording_mind_maps WHERE recording_id = '$recording_id'")"
  if [[ -z "$mind_map_row" ]]; then
    log "recording mind map row missing for $recording_id"
    return 1
  fi

  IFS=$'\t' read -r mind_map_provider mind_map_model mind_map_has_title mind_map_root_json_type mind_map_has_content mind_map_raw_json_type <<<"$mind_map_row"
  if [[ -z "$mind_map_provider" || -z "$mind_map_model" || "$mind_map_has_title" != "t" || "$mind_map_root_json_type" != "object" || "$mind_map_has_content" != "t" || "$mind_map_raw_json_type" != "object" ]]; then
    log "unexpected DB mind map row: $mind_map_row"
    return 1
  fi
}

main() {
  cd "$ROOT_DIR"

  if curl -fsS "$API_URL/healthz" >/dev/null 2>&1; then
    log "API already responds at $API_URL; stop it before running this smoke script so the script can verify restart behavior safely"
    exit 1
  fi
  if ! command -v ffmpeg >/dev/null 2>&1 || ! command -v ffprobe >/dev/null 2>&1; then
    log "ffmpeg and ffprobe are required for audio probe smoke verification"
    exit 1
  fi

  log "starting local infrastructure via make temporal-up"
  make temporal-up

  wait_for_command "Soniq Postgres" 60 \
    docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql pg_isready -U "$POSTGRES_USER"

  wait_for_command "Temporal frontend" 60 \
    docker compose -f "$COMPOSE_FILE" exec -T temporal temporal --address temporal:7233 operator namespace list

  apply_application_migrations

  rm -rf "$EFFECTIVE_LOCAL_STORAGE_PATH"
  mkdir -p "$EFFECTIVE_LOCAL_STORAGE_PATH"

  start_worker
  start_api
  authenticate_api

  log "uploading recording audio via POST /workspaces/$SMOKE_WORKSPACE_ID/recordings/upload"
  local response recording_id workflow_id audio_object_key audio_size audio_file upload_title upload_language upload_filename upload_content_type csrf_token_value
  audio_file="$LOG_DIR/weekly.wav"
  upload_title="${SMOKE_TITLE:-Weekly sync}"
  upload_language="${SMOKE_LANGUAGE:-en}"
  EXPECTED_TRANSCRIPT_LANGUAGE="${EXPECTED_TRANSCRIPT_LANGUAGE:-$upload_language}"
  upload_filename="weekly.wav"
  upload_content_type="audio/wav"
  if [[ -n "${SMOKE_AUDIO_FILE:-}" ]]; then
    if [[ ! -f "$SMOKE_AUDIO_FILE" ]]; then
      log "SMOKE_AUDIO_FILE does not exist: $SMOKE_AUDIO_FILE"
      return 1
    fi
    log "using SMOKE_AUDIO_FILE=$SMOKE_AUDIO_FILE"
    audio_file="$SMOKE_AUDIO_FILE"
    upload_filename="${SMOKE_AUDIO_FILENAME:-$(basename "$SMOKE_AUDIO_FILE")}"
    upload_content_type="${SMOKE_AUDIO_CONTENT_TYPE:-$(content_type_for_file "$SMOKE_AUDIO_FILE")}"
  else
    ffmpeg -hide_banner -loglevel error -f lavfi -i sine=frequency=1000:duration=1 -ac 1 -ar 16000 -c:a pcm_s16le "$audio_file"
  fi
  audio_size="$(wc -c <"$audio_file" | tr -d ' ')"
  csrf_token_value="$(csrf_token)"
  response="$(curl -fsS -b "$COOKIE_JAR" -X POST "$API_URL/workspaces/$SMOKE_WORKSPACE_ID/recordings/upload" \
    -H "X-CSRF-Token: $csrf_token_value" \
    -F "title=$upload_title" \
    -F 'workflow_type=meeting' \
    -F "language=$upload_language" \
    -F "audio=@$audio_file;filename=$upload_filename;type=$upload_content_type")"
  printf '%s\n' "$response"
  recording_id="$(printf '%s\n' "$response" | extract_recording_id)"
  audio_object_key="$(printf '%s\n' "$response" | extract_json_field recording.audio_object_key)"
  workflow_id="recording-processing-$recording_id"
  log "expected Temporal workflow ID: $workflow_id"

  assert_json_field_equals "$response" processing_enqueued True
  assert_json_field_equals "$response" recording.workspace_id "$SMOKE_WORKSPACE_ID"
  assert_json_field_equals "$response" recording.audio_content_type "$upload_content_type"
  assert_json_field_equals "$response" recording.audio_size_bytes "$audio_size"
  if [[ "$audio_object_key" != "workspaces/$SMOKE_WORKSPACE_ID/recordings/"* ]]; then
    log "unexpected uploaded object key prefix: $audio_object_key"
    return 1
  fi
  assert_uploaded_object "$audio_object_key" "$audio_size"
  assert_recording_audio_metadata_in_db "$recording_id" "$audio_object_key" "$upload_content_type" "$audio_size"
  assert_recording_workspace_in_db "$recording_id" "$SMOKE_WORKSPACE_ID"

  log "verifying GET /workspaces/$SMOKE_WORKSPACE_ID/recordings/$recording_id before API restart"
  response="$(curl -fsS -b "$COOKIE_JAR" "$API_URL/workspaces/$SMOKE_WORKSPACE_ID/recordings/$recording_id")"
  assert_json_field_equals "$response" workspace_id "$SMOKE_WORKSPACE_ID"
  assert_json_field_equals "$response" audio_object_key "$audio_object_key"
  assert_json_field_equals "$response" audio_content_type "$upload_content_type"
  assert_json_field_equals "$response" audio_size_bytes "$audio_size"

  log "restarting only API to verify Postgres persistence"
  stop_api
  start_api

  log "verifying GET /workspaces/$SMOKE_WORKSPACE_ID/recordings/$recording_id after API restart"
  response="$(curl -fsS -b "$COOKIE_JAR" "$API_URL/workspaces/$SMOKE_WORKSPACE_ID/recordings/$recording_id")"
  assert_json_field_equals "$response" workspace_id "$SMOKE_WORKSPACE_ID"
  assert_json_field_equals "$response" audio_object_key "$audio_object_key"
  assert_json_field_equals "$response" audio_content_type "$upload_content_type"
  assert_json_field_equals "$response" audio_size_bytes "$audio_size"
  assert_uploaded_object "$audio_object_key" "$audio_size"
  curl -fsS -b "$COOKIE_JAR" "$API_URL/workspaces/$SMOKE_WORKSPACE_ID/recordings/$recording_id/status" >/dev/null

  log "waiting for Temporal workflow completion"
  local describe_output=""
  local i
  for ((i = 1; i <= 60; i++)); do
    describe_output="$(docker compose -f "$COMPOSE_FILE" exec -T temporal \
      temporal --address temporal:7233 workflow describe \
      --namespace "$TEMPORAL_NAMESPACE" \
      --workflow-id "$workflow_id" 2>/dev/null || true)"
    if grep -q 'Status.*COMPLETED' <<<"$describe_output"; then
      printf '%s\n' "$describe_output"
      log "Temporal workflow completed: $workflow_id"
      assert_recording_status_in_db "$recording_id" completed
      log "recording DB status reached completed: $recording_id"
      assert_recording_audio_probe_in_db "$recording_id"
      log "recording audio probe metadata persisted: $recording_id"
      assert_recording_normalized_audio_in_db "$recording_id"
      log "recording normalized audio persisted: $recording_id"
      assert_recording_transcript_summary_mind_map_in_db "$recording_id"
      log "recording transcript, summary, and mind map persisted: $recording_id"
      log "recording persisted across API restart: $recording_id"
      return 0
    fi
    sleep 1
  done

  log "Temporal workflow did not reach COMPLETED"
  printf '%s\n' "$describe_output"
  return 1
}

main "$@"
