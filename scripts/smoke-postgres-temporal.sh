#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-compose.temporal.yml}"
API_URL="${API_URL:-http://localhost:8080}"
API_ADDRESS="${API_ADDRESS:-:8080}"
TEMPORAL_NAMESPACE="${TEMPORAL_NAMESPACE:-default}"
TEMPORAL_TASK_QUEUE="${TEMPORAL_TASK_QUEUE:-soniq-audio-pipeline}"
POSTGRES_USER="${POSTGRES_USER:-soniq_user}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-soniq_password}"
POSTGRES_DSN="${POSTGRES_DSN:-postgres://$POSTGRES_USER:${POSTGRES_PASSWORD}@localhost:5432/soniq?sslmode=disable}"
STORAGE_PROVIDER="${STORAGE_PROVIDER:-local}"
LOCAL_STORAGE_PATH="${LOCAL_STORAGE_PATH:-$ROOT_DIR/var/uploads/smoke}"
SMOKE_DOWN="${SMOKE_DOWN:-0}"

API_PID=""
WORKER_PID=""
LOG_DIR="$(mktemp -d "${TMPDIR:-/tmp}/soniq-smoke.XXXXXX")"
API_LOG="$LOG_DIR/api.log"
WORKER_LOG="$LOG_DIR/worker.log"

log() {
  printf '[smoke] %s\n' "$*"
}

cleanup() {
  local exit_code=$?
  if [[ -n "$API_PID" ]] && kill -0 "$API_PID" 2>/dev/null; then
    log "stopping API process $API_PID"
    kill "$API_PID" 2>/dev/null || true
    wait "$API_PID" 2>/dev/null || true
  fi
  if [[ -n "$WORKER_PID" ]] && kill -0 "$WORKER_PID" 2>/dev/null; then
    log "stopping worker process $WORKER_PID"
    kill "$WORKER_PID" 2>/dev/null || true
    wait "$WORKER_PID" 2>/dev/null || true
  fi
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
  (
    cd "$ROOT_DIR"
    TEMPORAL_NAMESPACE="$TEMPORAL_NAMESPACE" \
    TEMPORAL_TASK_QUEUE="$TEMPORAL_TASK_QUEUE" \
    make worker
  ) >"$WORKER_LOG" 2>&1 &
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
  (
    cd "$ROOT_DIR"
    API_ADDRESS="$API_ADDRESS" \
    POSTGRES_DSN="$POSTGRES_DSN" \
    TEMPORAL_NAMESPACE="$TEMPORAL_NAMESPACE" \
    TEMPORAL_TASK_QUEUE="$TEMPORAL_TASK_QUEUE" \
    STORAGE_PROVIDER="$STORAGE_PROVIDER" \
    LOCAL_STORAGE_PATH="$LOCAL_STORAGE_PATH" \
    make api
  ) >>"$API_LOG" 2>&1 &
  API_PID=$!
  wait_for_api
}

stop_api() {
  if [[ -n "$API_PID" ]] && kill -0 "$API_PID" 2>/dev/null; then
    log "stopping API process $API_PID"
    kill "$API_PID" 2>/dev/null || true
    wait "$API_PID" 2>/dev/null || true
  fi
  API_PID=""
}

extract_recording_id() {
  python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])'
}

extract_json_field() {
  local field="$1"
  python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$field"
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

apply_recording_migrations() {
  local table_exists audio_columns_exist
  table_exists="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U soniq_user -d soniq -Atc "SELECT to_regclass('public.recordings') IS NOT NULL")"
  if [[ "$table_exists" != "t" ]]; then
    log "applying recordings migration 0001"
    docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
      psql -U soniq_user -d soniq -v ON_ERROR_STOP=1 \
      -f - < backend/migrations/0001_create_recordings.up.sql
  else
    log "recordings table already exists; skipping migration 0001"
  fi

  audio_columns_exist="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U soniq_user -d soniq -Atc "SELECT count(*) = 3 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'recordings' AND column_name IN ('audio_object_key', 'audio_content_type', 'audio_size_bytes')")"
  if [[ "$audio_columns_exist" != "t" ]]; then
    log "applying recordings migration 0002"
    docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
      psql -U soniq_user -d soniq -v ON_ERROR_STOP=1 \
      -f - < backend/migrations/0002_add_recording_audio_metadata.up.sql
  else
    log "recording audio metadata columns already exist; skipping migration 0002"
  fi
}

assert_uploaded_object() {
  local object_key="$1"
  local expected_contents="$2"
  local object_path="$LOCAL_STORAGE_PATH/$object_key"
  if [[ ! -f "$object_path" ]]; then
    log "uploaded object does not exist: $object_path"
    return 1
  fi
  local actual_contents
  actual_contents="$(cat "$object_path")"
  if [[ "$actual_contents" != "$expected_contents" ]]; then
    log "uploaded object contents mismatch"
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
    psql -U soniq_user -d soniq -AtF $'\t' -c "SELECT audio_object_key, audio_content_type, audio_size_bytes FROM recordings WHERE id = '$recording_id'")"
  if [[ "$row" != "$expected_object_key"$'\t'"$expected_content_type"$'\t'"$expected_size_bytes" ]]; then
    log "unexpected DB audio metadata row: $row"
    return 1
  fi
}

main() {
  cd "$ROOT_DIR"

  if curl -fsS "$API_URL/healthz" >/dev/null 2>&1; then
    log "API already responds at $API_URL; stop it before running this smoke script so the script can verify restart behavior safely"
    exit 1
  fi

  log "starting local infrastructure via make temporal-up"
  make temporal-up

  wait_for_command "Soniq Postgres" 60 \
    docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql pg_isready -U soniq_user

  wait_for_command "Temporal frontend" 60 \
    docker compose -f "$COMPOSE_FILE" exec -T temporal temporal --address temporal:7233 operator namespace list

  apply_recording_migrations

  rm -rf "$LOCAL_STORAGE_PATH"
  mkdir -p "$LOCAL_STORAGE_PATH"

  start_worker
  start_api

  log "uploading recording audio via POST /recordings/upload"
  local response recording_id workflow_id audio_object_key audio_contents audio_size audio_file
  audio_contents="soniq-smoke-audio-bytes"
  audio_size="${#audio_contents}"
  audio_file="$LOG_DIR/weekly.wav"
  printf '%s' "$audio_contents" >"$audio_file"
  response="$(curl -fsS -X POST "$API_URL/recordings/upload" \
    -F 'title=Weekly sync' \
    -F 'workflow_type=meeting' \
    -F 'language=en' \
    -F "audio=@$audio_file;filename=weekly.wav;type=audio/wav")"
  printf '%s\n' "$response"
  recording_id="$(printf '%s\n' "$response" | extract_recording_id)"
  audio_object_key="$(printf '%s\n' "$response" | extract_json_field audio_object_key)"
  workflow_id="recording-processing-$recording_id"
  log "expected Temporal workflow ID: $workflow_id"

  assert_json_field_equals "$response" audio_content_type audio/wav
  assert_json_field_equals "$response" audio_size_bytes "$audio_size"
  assert_uploaded_object "$audio_object_key" "$audio_contents"
  assert_recording_audio_metadata_in_db "$recording_id" "$audio_object_key" audio/wav "$audio_size"

  log "verifying GET /recordings/$recording_id before API restart"
  response="$(curl -fsS "$API_URL/recordings/$recording_id")"
  assert_json_field_equals "$response" audio_object_key "$audio_object_key"
  assert_json_field_equals "$response" audio_content_type audio/wav
  assert_json_field_equals "$response" audio_size_bytes "$audio_size"

  log "restarting only API to verify Postgres persistence"
  stop_api
  start_api

  log "verifying GET /recordings/$recording_id after API restart"
  response="$(curl -fsS "$API_URL/recordings/$recording_id")"
  assert_json_field_equals "$response" audio_object_key "$audio_object_key"
  assert_json_field_equals "$response" audio_content_type audio/wav
  assert_json_field_equals "$response" audio_size_bytes "$audio_size"
  assert_uploaded_object "$audio_object_key" "$audio_contents"
  curl -fsS "$API_URL/recordings/$recording_id/status" >/dev/null

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
