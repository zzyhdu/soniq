#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-compose.temporal.yml}"
API_URL="${API_URL:-http://localhost:8080}"
API_ADDRESS="${API_ADDRESS:-:8080}"
TEMPORAL_NAMESPACE="${TEMPORAL_NAMESPACE:-default}"
TEMPORAL_TASK_QUEUE="${TEMPORAL_TASK_QUEUE:-soniq-audio-pipeline}"
POSTGRES_DSN="${POSTGRES_DSN:-postgres://soniq_user:soniq_password@localhost:5432/soniq?sslmode=disable}"
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

  local table_exists
  table_exists="$(docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U soniq_user -d soniq -Atc "SELECT to_regclass('public.recordings') IS NOT NULL")"
  if [[ "$table_exists" != "t" ]]; then
    log "applying recordings migration"
    docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
      psql -U soniq_user -d soniq -v ON_ERROR_STOP=1 \
      -f - < backend/migrations/0001_create_recordings.up.sql
  else
    log "recordings table already exists; skipping migration apply"
  fi

  start_worker
  start_api

  log "creating recording via POST /recordings"
  local response recording_id workflow_id
  response="$(curl -fsS -X POST "$API_URL/recordings" \
    -H 'Content-Type: application/json' \
    -d '{"title":"Weekly sync","workflow_type":"meeting","language":"en"}')"
  printf '%s\n' "$response"
  recording_id="$(printf '%s\n' "$response" | extract_recording_id)"
  workflow_id="recording-processing-$recording_id"
  log "expected Temporal workflow ID: $workflow_id"

  log "verifying GET /recordings/$recording_id before API restart"
  curl -fsS "$API_URL/recordings/$recording_id" >/dev/null

  log "restarting only API to verify Postgres persistence"
  stop_api
  start_api

  log "verifying GET /recordings/$recording_id after API restart"
  curl -fsS "$API_URL/recordings/$recording_id" >/dev/null
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
