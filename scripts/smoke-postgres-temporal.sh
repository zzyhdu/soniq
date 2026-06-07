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
POSTGRES_DSN="${POSTGRES_DSN:-postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@localhost:5432/soniq?sslmode=disable}"
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
    POSTGRES_DSN="$POSTGRES_DSN" \
    STORAGE_PROVIDER="$STORAGE_PROVIDER" \
    LOCAL_STORAGE_PATH="$LOCAL_STORAGE_PATH" \
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
  local table_exists audio_columns_exist audio_probe_table_exists transcript_table_exists summary_table_exists normalized_audio_table_exists
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

  audio_probe_table_exists="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U soniq_user -d soniq -Atc "SELECT to_regclass('public.recording_audio_probes') IS NOT NULL")"
  if [[ "$audio_probe_table_exists" != "t" ]]; then
    log "applying recordings migration 0003"
    docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
      psql -U soniq_user -d soniq -v ON_ERROR_STOP=1 \
      -f - < backend/migrations/0003_create_recording_audio_probes.up.sql
  else
    log "recording audio probes table already exists; skipping migration 0003"
  fi

  transcript_table_exists="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U soniq_user -d soniq -Atc "SELECT to_regclass('public.recording_transcripts') IS NOT NULL AND to_regclass('public.recording_transcript_segments') IS NOT NULL")"
  summary_table_exists="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U soniq_user -d soniq -Atc "SELECT to_regclass('public.recording_summaries') IS NOT NULL")"
  if [[ "$transcript_table_exists" != "t" || "$summary_table_exists" != "t" ]]; then
    log "applying recordings migration 0004"
    docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
      psql -U soniq_user -d soniq -v ON_ERROR_STOP=1 \
      -f - < backend/migrations/0004_create_recording_transcripts_and_summaries.up.sql
  else
    log "recording transcript and summary tables already exist; skipping migration 0004"
  fi

  normalized_audio_table_exists="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U soniq_user -d soniq -Atc "SELECT to_regclass('public.recording_normalized_audios') IS NOT NULL")"
  if [[ "$normalized_audio_table_exists" != "t" ]]; then
    log "applying recordings migration 0005"
    docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
      psql -U soniq_user -d soniq -v ON_ERROR_STOP=1 \
      -f - < backend/migrations/0005_create_recording_normalized_audios.up.sql
  else
    log "recording normalized audio table already exists; skipping migration 0005"
  fi
}

assert_uploaded_object() {
  local object_key="$1"
  local expected_size_bytes="$2"
  local object_path="$LOCAL_STORAGE_PATH/$object_key"
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
    psql -U soniq_user -d soniq -AtF $'\t' -c "SELECT audio_object_key, audio_content_type, audio_size_bytes FROM recordings WHERE id = '$recording_id'")"
  if [[ "$row" != "$expected_object_key"$'\t'"$expected_content_type"$'\t'"$expected_size_bytes" ]]; then
    log "unexpected DB audio metadata row: $row"
    return 1
  fi
}

assert_recording_status_in_db() {
  local recording_id="$1"
  local expected_status="$2"
  local actual_status
  actual_status="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U soniq_user -d soniq -Atc "SELECT status FROM recordings WHERE id = '$recording_id'")"
  if [[ "$actual_status" != "$expected_status" ]]; then
    log "unexpected DB recording status: $actual_status, want $expected_status"
    return 1
  fi
}

assert_recording_audio_probe_in_db() {
  local recording_id="$1"
  local row
  row="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U soniq_user -d soniq -AtF $'\t' -c "SELECT format_name, codec_name, sample_rate, channels, (duration_seconds > 0), jsonb_typeof(raw_probe_json) FROM recording_audio_probes WHERE recording_id = '$recording_id'")"
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
    psql -U soniq_user -d soniq -AtF $'\t' -c "SELECT object_key, content_type, size_bytes, format_name, codec_name, sample_rate, channels, (normalized_at IS NOT NULL) FROM recording_normalized_audios WHERE recording_id = '$recording_id'")"
  if [[ -z "$row" ]]; then
    log "recording normalized audio row missing for $recording_id"
    return 1
  fi

  IFS=$'\t' read -r object_key content_type size_bytes format_name codec_name sample_rate channels normalized_at_set <<<"$row"
  if [[ -z "$object_key" || "$object_key" != */normalized.wav || "$content_type" != "audio/wav" || "$size_bytes" -le 0 || "$format_name" != "wav" || "$codec_name" != "pcm_s16le" || "$sample_rate" != "16000" || "$channels" != "1" || "$normalized_at_set" != "t" ]]; then
    log "unexpected DB normalized audio row: $row"
    return 1
  fi

  object_path="$LOCAL_STORAGE_PATH/$object_key"
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

assert_recording_transcript_summary_in_db() {
  local recording_id="$1"
  local transcript_row segment_count summary_row
  transcript_row="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U soniq_user -d soniq -AtF $'\t' -c "SELECT provider, model, language, (length(text) > 0), jsonb_typeof(raw_result_json) FROM recording_transcripts WHERE recording_id = '$recording_id'")"
  if [[ -z "$transcript_row" ]]; then
    log "recording transcript row missing for $recording_id"
    return 1
  fi

  IFS=$'\t' read -r transcript_provider transcript_model transcript_language transcript_has_text transcript_raw_json_type <<<"$transcript_row"
  if [[ -z "$transcript_provider" || -z "$transcript_model" || "$transcript_language" != "en" || "$transcript_has_text" != "t" || "$transcript_raw_json_type" != "object" ]]; then
    log "unexpected DB transcript row: $transcript_row"
    return 1
  fi

  segment_count="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U soniq_user -d soniq -Atc "SELECT count(*) FROM recording_transcript_segments WHERE recording_id = '$recording_id'")"
  if [[ "$segment_count" -lt 1 ]]; then
    log "recording transcript segments missing for $recording_id"
    return 1
  fi

  summary_row="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U soniq_user -d soniq -AtF $'\t' -c "SELECT provider, model, type, (length(overview) > 0 OR length(content_markdown) > 0), jsonb_typeof(raw_result_json) FROM recording_summaries WHERE recording_id = '$recording_id'")"
  if [[ -z "$summary_row" ]]; then
    log "recording summary row missing for $recording_id"
    return 1
  fi

  IFS=$'\t' read -r summary_provider summary_model summary_type summary_has_content summary_raw_json_type <<<"$summary_row"
  if [[ -z "$summary_provider" || -z "$summary_model" || "$summary_type" != "meeting" || "$summary_has_content" != "t" || "$summary_raw_json_type" != "object" ]]; then
    log "unexpected DB summary row: $summary_row"
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
    docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql pg_isready -U soniq_user

  wait_for_command "Temporal frontend" 60 \
    docker compose -f "$COMPOSE_FILE" exec -T temporal temporal --address temporal:7233 operator namespace list

  apply_recording_migrations

  rm -rf "$LOCAL_STORAGE_PATH"
  mkdir -p "$LOCAL_STORAGE_PATH"

  start_worker
  start_api

  log "uploading recording audio via POST /recordings/upload"
  local response recording_id workflow_id audio_object_key audio_size audio_file
  audio_file="$LOG_DIR/weekly.wav"
  ffmpeg -hide_banner -loglevel error -f lavfi -i sine=frequency=1000:duration=1 -ac 1 -ar 16000 -c:a pcm_s16le "$audio_file"
  audio_size="$(wc -c <"$audio_file" | tr -d ' ')"
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
  assert_uploaded_object "$audio_object_key" "$audio_size"
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
  assert_uploaded_object "$audio_object_key" "$audio_size"
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
      assert_recording_status_in_db "$recording_id" completed
      log "recording DB status reached completed: $recording_id"
      assert_recording_audio_probe_in_db "$recording_id"
      log "recording audio probe metadata persisted: $recording_id"
      assert_recording_normalized_audio_in_db "$recording_id"
      log "recording normalized audio persisted: $recording_id"
      assert_recording_transcript_summary_in_db "$recording_id"
      log "recording transcript and summary persisted: $recording_id"
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
