#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-compose.temporal.yml}"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-soniq}"
K8S_NAMESPACE="${K8S_NAMESPACE:-soniq}"
KUSTOMIZE_DIR="${KUSTOMIZE_DIR:-deploy/kubernetes/base}"
HELM_CHART="${HELM_CHART:-deploy/helm/soniq}"
HELM_RELEASE="${HELM_RELEASE:-soniq}"
HELM_BIN="${HELM_BIN:-helm}"
HELM_WRAPPER="${HELM_WRAPPER:-}"
KIND_SMOKE_DEPLOYER="${KIND_SMOKE_DEPLOYER:-kubectl}"
POSTGRES_USER="${POSTGRES_USER:-soniq_user}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-soniq_password}"
POSTGRES_DB="${POSTGRES_DB:-soniq}"
S3_BUCKET="${S3_BUCKET:-soniq}"
S3_ACCESS_KEY="${S3_ACCESS_KEY:-soniq_minio_user}"
S3_SECRET_KEY="${S3_SECRET_KEY:-soniq_minio_password}"
KIND_SMOKE_BUILD_IMAGES="${KIND_SMOKE_BUILD_IMAGES:-1}"
KIND_SMOKE_CLEAN_NAMESPACE="${KIND_SMOKE_CLEAN_NAMESPACE:-1}"
KIND_SMOKE_API_PORT="${KIND_SMOKE_API_PORT:-18080}"
KIND_SMOKE_WORKFLOW="${KIND_SMOKE_WORKFLOW:-1}"
SMOKE_EMAIL="${SMOKE_EMAIL:-kind-smoke@local.soniq}"
SMOKE_DISPLAY_NAME="${SMOKE_DISPLAY_NAME:-Kind Smoke Tester}"
SMOKE_PASSWORD="${SMOKE_PASSWORD:-correct horse kind smoke}"
SMOKE_WORKSPACE_ID="${SMOKE_WORKSPACE_ID:-}"
SMOKE_TITLE="${SMOKE_TITLE:-Kind smoke upload}"
SMOKE_LANGUAGE="${SMOKE_LANGUAGE:-en}"
EXPECTED_TRANSCRIPT_LANGUAGE="${EXPECTED_TRANSCRIPT_LANGUAGE:-$SMOKE_LANGUAGE}"

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/soniq-kind-smoke.XXXXXX")"
BASE_MANIFEST="$TMP_DIR/base.yaml"
SMOKE_MANIFEST="$TMP_DIR/smoke.yaml"
PORT_FORWARD_LOG="$TMP_DIR/port-forward.log"
COOKIE_JAR="$TMP_DIR/cookies.txt"
PORT_FORWARD_PID=""

cleanup() {
  if [[ -n "$PORT_FORWARD_PID" ]] && kill -0 "$PORT_FORWARD_PID" 2>/dev/null; then
    kill "$PORT_FORWARD_PID" 2>/dev/null || true
    wait "$PORT_FORWARD_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

log() {
  printf '[kind-smoke] %s\n' "$*"
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf '%s is required for kind Kubernetes smoke\n' "$1" >&2
    exit 127
  fi
}

run_helm() {
  if [[ -n "$HELM_WRAPPER" ]]; then
    bash "$HELM_WRAPPER" "$@"
    return
  fi
  "$HELM_BIN" "$@"
}

require_helm() {
  if ! run_helm version --short >/dev/null 2>&1; then
    printf 'helm is required for Helm kind Kubernetes smoke; set HELM_BIN to a runnable Helm command\n' >&2
    exit 127
  fi
}

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

compose_container_id() {
  local service="$1"
  local container_id
  container_id="$(docker compose -f "$COMPOSE_FILE" ps -q "$service")"
  if [[ -z "$container_id" ]]; then
    printf 'compose service %s is not running\n' "$service" >&2
    exit 1
  fi
  printf '%s\n' "$container_id"
}

container_network_name() {
  local container_id="$1"
  docker inspect -f '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}' "$container_id" | head -n 1
}

container_ip_on_network() {
  local container_id="$1"
  local network="$2"
  docker inspect -f "{{with index .NetworkSettings.Networks \"$network\"}}{{.IPAddress}}{{end}}" "$container_id"
}

ensure_kind_cluster() {
  if kind get clusters | grep -qx "$KIND_CLUSTER_NAME"; then
    log "kind cluster $KIND_CLUSTER_NAME already exists"
  else
    log "creating kind cluster $KIND_CLUSTER_NAME"
    kind create cluster --name "$KIND_CLUSTER_NAME"
  fi
  kubectl config use-context "kind-$KIND_CLUSTER_NAME" >/dev/null
  kubectl wait --for=condition=Ready "node/$KIND_CLUSTER_NAME-control-plane" --timeout=120s
  kubectl -n kube-system wait --for=condition=Ready pod --all --timeout=120s
}

ensure_kind_node_on_compose_network() {
  local network="$1"
  local node_container="$KIND_CLUSTER_NAME-control-plane"
  if docker inspect -f "{{with index .NetworkSettings.Networks \"$network\"}}connected{{end}}" "$node_container" | grep -qx connected; then
    log "kind node $node_container is already connected to Docker network $network"
    return 0
  fi
  log "connecting kind node $node_container to Docker network $network"
  docker network connect "$network" "$node_container"
}

run_minio_mc() {
  docker compose -f "$COMPOSE_FILE" exec -T \
    -e S3_BUCKET="$S3_BUCKET" \
    -e S3_ACCESS_KEY="$S3_ACCESS_KEY" \
    -e S3_SECRET_KEY="$S3_SECRET_KEY" \
    minio sh -c '
      mc alias set smoke http://127.0.0.1:9000 "$S3_ACCESS_KEY" "$S3_SECRET_KEY" >/dev/null
      mc "$@"
    ' sh "$@"
}

assert_s3_bucket_ready() {
  run_minio_mc ls "smoke/$S3_BUCKET"
}

assert_object_exists() {
  local object_key="$1"
  local expected_size_bytes="$2"
  local stat_json actual_size_bytes
  stat_json="$(run_minio_mc stat --json "smoke/$S3_BUCKET/$object_key")"
  actual_size_bytes="$(printf '%s\n' "$stat_json" | python3 -c 'import json,sys; print(json.load(sys.stdin)["size"])')"
  if [[ "$actual_size_bytes" != "$expected_size_bytes" ]]; then
    log "S3 object size mismatch for $object_key: $actual_size_bytes, want $expected_size_bytes"
    return 1
  fi
}

assert_object_missing() {
  local object_key="$1"
  if run_minio_mc stat "smoke/$S3_BUCKET/$object_key" >/dev/null 2>&1; then
    log "S3 object still exists after purge: $object_key"
    return 1
  fi
}

psql_query() {
  docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" "$@"
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

assert_recording_in_list() {
  local json="$1"
  local recording_id="$2"
  local expected_deleted="$3"
  printf '%s\n' "$json" | python3 -c '
import json
import sys

recording_id = sys.argv[1]
expected_deleted = sys.argv[2] == "true"
data = json.load(sys.stdin)
recording = next((item for item in data.get("recordings", []) if item.get("id") == recording_id), None)
if recording is None:
    raise SystemExit(f"recording {recording_id} not found in list")
actual_deleted = recording.get("deleted_at") not in (None, "")
if actual_deleted != expected_deleted:
    raise SystemExit(
        f"recording {recording_id} deleted state = {actual_deleted}, want {expected_deleted}"
    )
' "$recording_id" "$expected_deleted"
}

assert_recording_not_in_list() {
  local json="$1"
  local recording_id="$2"
  printf '%s\n' "$json" | python3 -c '
import json
import sys

recording_id = sys.argv[1]
data = json.load(sys.stdin)
if any(item.get("id") == recording_id for item in data.get("recordings", [])):
    raise SystemExit(f"recording {recording_id} unexpectedly found in list")
' "$recording_id"
}

expect_api_status() {
  local method="$1"
  local expected_status="$2"
  local url="$3"
  local csrf_token_value="${4:-}"
  local response_file status
  response_file="$TMP_DIR/api-response-$method-$expected_status-$RANDOM.txt"
  local curl_args=(-sS -o "$response_file" -w "%{http_code}" -b "$COOKIE_JAR" -X "$method")
  if [[ -n "$csrf_token_value" ]]; then
    curl_args+=(-H "X-CSRF-Token: $csrf_token_value")
  fi
  status="$(curl "${curl_args[@]}" "$url")"
  if [[ "$status" != "$expected_status" ]]; then
    log "$method $url returned HTTP $status, want $expected_status"
    cat "$response_file"
    return 1
  fi
}

authenticate_api() {
  local api_url="$1"
  local response_file status workspaces_response
  response_file="$TMP_DIR/auth-response.json"

  log "creating or signing in smoke user $SMOKE_EMAIL"
  status="$(auth_json | curl -sS -o "$response_file" -w "%{http_code}" -c "$COOKIE_JAR" \
    -H 'Content-Type: application/json' \
    -X POST "$api_url/auth/signup" \
    --data-binary @-)"
  if [[ "$status" == "201" ]]; then
    log "created smoke user"
  elif [[ "$status" == "409" ]]; then
    status="$(auth_json | curl -sS -o "$response_file" -w "%{http_code}" -c "$COOKIE_JAR" \
      -H 'Content-Type: application/json' \
      -X POST "$api_url/auth/signin" \
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

  workspaces_response="$(curl -fsS -b "$COOKIE_JAR" "$api_url/workspaces")"
  if [[ -z "$SMOKE_WORKSPACE_ID" ]]; then
    SMOKE_WORKSPACE_ID="$(printf '%s\n' "$workspaces_response" | extract_first_workspace_id)"
  fi
  log "using smoke workspace $SMOKE_WORKSPACE_ID"
}

generate_smoke_audio() {
  local audio_file="$1"
  ffmpeg -hide_banner -loglevel error -f lavfi -i sine=frequency=1000:duration=1 \
    -ac 1 -ar 16000 -c:a pcm_s16le "$audio_file"
}

assert_recording_audio_metadata_in_db() {
  local recording_id="$1"
  local expected_object_key="$2"
  local expected_content_type="$3"
  local expected_size_bytes="$4"
  local row
  row="$(psql_query -AtF $'\t' -c "SELECT audio_object_key, audio_content_type, audio_size_bytes FROM recordings WHERE id = '$recording_id'")"
  if [[ "$row" != "$expected_object_key"$'\t'"$expected_content_type"$'\t'"$expected_size_bytes" ]]; then
    log "unexpected DB audio metadata row: $row"
    return 1
  fi
}

assert_recording_status_in_db() {
  local recording_id="$1"
  local expected_status="$2"
  local row actual_status completed_at_set failure_reason
  row="$(psql_query -AtF $'\t' -c "SELECT status, (completed_at IS NOT NULL), failure_reason FROM recordings WHERE id = '$recording_id'")"
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

assert_recording_deleted_state_in_db() {
  local recording_id="$1"
  local expected_deleted="$2"
  local row actual_deleted deleted_by_user_id
  row="$(psql_query -AtF $'\t' -c "SELECT (deleted_at IS NOT NULL), COALESCE(deleted_by_user_id, '') FROM recordings WHERE id = '$recording_id'")"
  if [[ -z "$row" ]]; then
    log "recording row missing for deleted-state check: $recording_id"
    return 1
  fi

  IFS=$'\t' read -r actual_deleted deleted_by_user_id <<<"$row"
  if [[ "$actual_deleted" != "$expected_deleted" ]]; then
    log "unexpected DB deleted state for $recording_id: $actual_deleted, want $expected_deleted"
    return 1
  fi
  if [[ "$expected_deleted" == "t" && -z "$deleted_by_user_id" ]]; then
    log "recording $recording_id is deleted but deleted_by_user_id is empty"
    return 1
  fi
  if [[ "$expected_deleted" == "f" && -n "$deleted_by_user_id" ]]; then
    log "recording $recording_id is active but deleted_by_user_id is set: $deleted_by_user_id"
    return 1
  fi
}

assert_recording_audio_probe_in_db() {
  local recording_id="$1"
  local row format_name codec_name sample_rate channels has_duration raw_json_type
  row="$(psql_query -AtF $'\t' -c "SELECT format_name, codec_name, sample_rate, channels, (duration_seconds > 0), jsonb_typeof(raw_probe_json) FROM recording_audio_probes WHERE recording_id = '$recording_id'")"
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
  local row object_key content_type size_bytes format_name codec_name sample_rate channels normalized_at_set
  row="$(psql_query -AtF $'\t' -c "SELECT object_key, content_type, size_bytes, format_name, codec_name, sample_rate, channels, (normalized_at IS NOT NULL) FROM recording_normalized_audios WHERE recording_id = '$recording_id'")"
  if [[ -z "$row" ]]; then
    log "recording normalized audio row missing for $recording_id"
    return 1
  fi

  IFS=$'\t' read -r object_key content_type size_bytes format_name codec_name sample_rate channels normalized_at_set <<<"$row"
  if [[ -z "$object_key" || "$object_key" != */normalized.wav || "$content_type" != "audio/wav" || "$size_bytes" -le 0 || "$format_name" != "wav" || "$codec_name" != "pcm_s16le" || "$sample_rate" != "16000" || "$channels" != "1" || "$normalized_at_set" != "t" ]]; then
    log "unexpected DB normalized audio row: $row"
    return 1
  fi

  assert_object_exists "$object_key" "$size_bytes"
}

normalized_audio_object_key_from_db() {
  local recording_id="$1"
  local object_key
  object_key="$(psql_query -Atc "SELECT object_key FROM recording_normalized_audios WHERE recording_id = '$recording_id'")"
  if [[ -z "$object_key" ]]; then
    log "normalized audio object key missing for $recording_id"
    return 1
  fi
  printf '%s\n' "$object_key"
}

assert_recording_transcript_summary_mind_map_in_db() {
  local recording_id="$1"
  local transcript_row segment_count summary_row mind_map_row
  transcript_row="$(psql_query -AtF $'\t' -c "SELECT provider, model, language, (length(text) > 0), jsonb_typeof(raw_result_json) FROM recording_transcripts WHERE recording_id = '$recording_id'")"
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

  segment_count="$(psql_query -Atc "SELECT count(*) FROM recording_transcript_segments WHERE recording_id = '$recording_id'")"
  if [[ "$segment_count" -lt 1 ]]; then
    log "recording transcript segments missing for $recording_id"
    return 1
  fi

  summary_row="$(psql_query -AtF $'\t' -c "SELECT provider, model, type, (length(overview) > 0 OR length(content_markdown) > 0), jsonb_typeof(raw_result_json) FROM recording_summaries WHERE recording_id = '$recording_id'")"
  if [[ -z "$summary_row" ]]; then
    log "recording summary row missing for $recording_id"
    return 1
  fi

  IFS=$'\t' read -r summary_provider summary_model summary_type summary_has_content summary_raw_json_type <<<"$summary_row"
  if [[ -z "$summary_provider" || -z "$summary_model" || "$summary_type" != "meeting" || "$summary_has_content" != "t" || "$summary_raw_json_type" != "object" ]]; then
    log "unexpected DB summary row: $summary_row"
    return 1
  fi

  mind_map_row="$(psql_query -AtF $'\t' -c "SELECT provider, model, (length(title) > 0), jsonb_typeof(root_json), (length(content_markdown) > 0), jsonb_typeof(raw_result_json) FROM recording_mind_maps WHERE recording_id = '$recording_id'")"
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

assert_recording_purged_from_db() {
  local recording_id="$1"
  local count child_count
  count="$(psql_query -Atc "SELECT count(*) FROM recordings WHERE id = '$recording_id'")"
  if [[ "$count" != "0" ]]; then
    log "recording row still exists after purge: $recording_id"
    return 1
  fi

  child_count="$(psql_query -Atc "SELECT
    (SELECT count(*) FROM recording_mind_maps WHERE recording_id = '$recording_id') +
    (SELECT count(*) FROM recording_transcript_segments WHERE recording_id = '$recording_id') +
    (SELECT count(*) FROM recording_summaries WHERE recording_id = '$recording_id') +
    (SELECT count(*) FROM recording_transcripts WHERE recording_id = '$recording_id') +
    (SELECT count(*) FROM recording_audio_probes WHERE recording_id = '$recording_id') +
    (SELECT count(*) FROM recording_normalized_audios WHERE recording_id = '$recording_id')")"
  if [[ "$child_count" != "0" ]]; then
    log "recording child rows still exist after purge: count=$child_count"
    return 1
  fi
}

assert_purge_artifacts_deleted_in_db() {
  local recording_id="$1"
  local expected_count="$2"
  local row artifact_count deleted_count
  row="$(psql_query -AtF $'\t' -c "SELECT count(*), count(*) FILTER (WHERE status = 'deleted' AND deleted_at IS NOT NULL) FROM recording_purge_artifacts WHERE recording_id = '$recording_id'")"
  IFS=$'\t' read -r artifact_count deleted_count <<<"$row"
  if [[ "$artifact_count" != "$expected_count" || "$deleted_count" != "$expected_count" ]]; then
    log "unexpected purge artifact cleanup rows for $recording_id: total=$artifact_count deleted=$deleted_count, want $expected_count"
    return 1
  fi
}

run_recording_lifecycle_smoke() {
  local api_url="$1"
  local recording_id="$2"
  local original_object_key="$3"
  local normalized_object_key="$4"
  local csrf_token_value active_response trash_response

  csrf_token_value="$(csrf_token)"

  log "verifying recording is active before delete"
  active_response="$(curl -fsS -b "$COOKIE_JAR" "$api_url/workspaces/$SMOKE_WORKSPACE_ID/recordings?limit=100")"
  trash_response="$(curl -fsS -b "$COOKIE_JAR" "$api_url/workspaces/$SMOKE_WORKSPACE_ID/recordings/trash?limit=100")"
  assert_recording_in_list "$active_response" "$recording_id" false
  assert_recording_not_in_list "$trash_response" "$recording_id"
  assert_recording_deleted_state_in_db "$recording_id" f

  log "soft deleting recording"
  expect_api_status DELETE 204 "$api_url/workspaces/$SMOKE_WORKSPACE_ID/recordings/$recording_id" "$csrf_token_value"
  active_response="$(curl -fsS -b "$COOKIE_JAR" "$api_url/workspaces/$SMOKE_WORKSPACE_ID/recordings?limit=100")"
  trash_response="$(curl -fsS -b "$COOKIE_JAR" "$api_url/workspaces/$SMOKE_WORKSPACE_ID/recordings/trash?limit=100")"
  assert_recording_not_in_list "$active_response" "$recording_id"
  assert_recording_in_list "$trash_response" "$recording_id" true
  assert_recording_deleted_state_in_db "$recording_id" t

  log "restoring recording from Trash"
  expect_api_status POST 200 "$api_url/workspaces/$SMOKE_WORKSPACE_ID/recordings/$recording_id/restore" "$csrf_token_value"
  active_response="$(curl -fsS -b "$COOKIE_JAR" "$api_url/workspaces/$SMOKE_WORKSPACE_ID/recordings?limit=100")"
  trash_response="$(curl -fsS -b "$COOKIE_JAR" "$api_url/workspaces/$SMOKE_WORKSPACE_ID/recordings/trash?limit=100")"
  assert_recording_in_list "$active_response" "$recording_id" false
  assert_recording_not_in_list "$trash_response" "$recording_id"
  assert_recording_deleted_state_in_db "$recording_id" f
  curl -fsS -b "$COOKIE_JAR" "$api_url/workspaces/$SMOKE_WORKSPACE_ID/recordings/$recording_id/details" >/dev/null

  log "verifying active recordings cannot be purged directly"
  expect_api_status DELETE 404 "$api_url/workspaces/$SMOKE_WORKSPACE_ID/recordings/$recording_id/purge" "$csrf_token_value"

  log "soft deleting and permanently purging recording"
  expect_api_status DELETE 204 "$api_url/workspaces/$SMOKE_WORKSPACE_ID/recordings/$recording_id" "$csrf_token_value"
  expect_api_status DELETE 204 "$api_url/workspaces/$SMOKE_WORKSPACE_ID/recordings/$recording_id/purge" "$csrf_token_value"
  expect_api_status GET 404 "$api_url/workspaces/$SMOKE_WORKSPACE_ID/recordings/$recording_id/details"

  active_response="$(curl -fsS -b "$COOKIE_JAR" "$api_url/workspaces/$SMOKE_WORKSPACE_ID/recordings?limit=100")"
  trash_response="$(curl -fsS -b "$COOKIE_JAR" "$api_url/workspaces/$SMOKE_WORKSPACE_ID/recordings/trash?limit=100")"
  assert_recording_not_in_list "$active_response" "$recording_id"
  assert_recording_not_in_list "$trash_response" "$recording_id"
  assert_recording_purged_from_db "$recording_id"
  assert_purge_artifacts_deleted_in_db "$recording_id" 2
  assert_object_missing "$original_object_key"
  assert_object_missing "$normalized_object_key"
  log "recording lifecycle delete/restore/purge verified: $recording_id"
}

run_workflow_smoke() {
  local api_url="$1"
  local response recording_id workflow_id audio_object_key audio_size audio_file csrf_token_value
  local normalized_object_key
  local upload_content_type="audio/wav"

  require_command ffmpeg
  authenticate_api "$api_url"

  audio_file="$TMP_DIR/kind-smoke.wav"
  log "generating smoke WAV"
  generate_smoke_audio "$audio_file"
  audio_size="$(wc -c <"$audio_file" | tr -d ' ')"
  csrf_token_value="$(csrf_token)"

  log "uploading recording audio via POST /workspaces/$SMOKE_WORKSPACE_ID/recordings/upload"
  response="$(curl -fsS -b "$COOKIE_JAR" -X POST "$api_url/workspaces/$SMOKE_WORKSPACE_ID/recordings/upload" \
    -H "X-CSRF-Token: $csrf_token_value" \
    -F "title=$SMOKE_TITLE" \
    -F 'workflow_type=meeting' \
    -F "language=$SMOKE_LANGUAGE" \
    -F "audio=@$audio_file;filename=kind-smoke.wav;type=$upload_content_type")"
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
  assert_object_exists "$audio_object_key" "$audio_size"
  assert_recording_audio_metadata_in_db "$recording_id" "$audio_object_key" "$upload_content_type" "$audio_size"

  log "waiting for Temporal workflow completion"
  local describe_output=""
  local i
  for ((i = 1; i <= 90; i++)); do
    describe_output="$(docker compose -f "$COMPOSE_FILE" exec -T temporal \
      temporal --address temporal:7233 workflow describe \
      --namespace default \
      --workflow-id "$workflow_id" 2>/dev/null || true)"
    if grep -q 'Status.*COMPLETED' <<<"$describe_output"; then
      printf '%s\n' "$describe_output"
      log "Temporal workflow completed: $workflow_id"
      assert_recording_status_in_db "$recording_id" completed
      assert_recording_audio_probe_in_db "$recording_id"
      assert_recording_normalized_audio_in_db "$recording_id"
      normalized_object_key="$(normalized_audio_object_key_from_db "$recording_id")"
      assert_recording_transcript_summary_mind_map_in_db "$recording_id"
      curl -fsS -b "$COOKIE_JAR" "$api_url/workspaces/$SMOKE_WORKSPACE_ID/recordings/$recording_id/details" >/dev/null
      log "recording workflow result verified: $recording_id"
      run_recording_lifecycle_smoke "$api_url" "$recording_id" "$audio_object_key" "$normalized_object_key"
      return 0
    fi
    sleep 1
  done

  log "Temporal workflow did not reach COMPLETED"
  printf '%s\n' "$describe_output"
  show_debug_context
  return 1
}

render_smoke_manifest() {
  local postgres_ip="$1"
  local temporal_ip="$2"
  local minio_ip="$3"

  log "rendering $KUSTOMIZE_DIR"
  kubectl kustomize "$KUSTOMIZE_DIR" >"$BASE_MANIFEST"

  log "patching rendered manifest for kind smoke"
  python3 - "$BASE_MANIFEST" "$SMOKE_MANIFEST" "$postgres_ip" "$temporal_ip" "$minio_ip" \
    "$POSTGRES_USER" "$POSTGRES_PASSWORD" "$POSTGRES_DB" "$S3_BUCKET" "$S3_ACCESS_KEY" "$S3_SECRET_KEY" <<'PY'
import sys

import yaml

(
    source_path,
    output_path,
    postgres_ip,
    temporal_ip,
    minio_ip,
    postgres_user,
    postgres_password,
    postgres_db,
    s3_bucket,
    s3_access_key,
    s3_secret_key,
) = sys.argv[1:]

with open(source_path, "r", encoding="utf-8") as handle:
    resources = [resource for resource in yaml.safe_load_all(handle) if resource]

for resource in resources:
    kind = resource.get("kind")
    metadata = resource.get("metadata") or {}
    name = metadata.get("name")
    if kind == "ConfigMap" and name == "soniq-config":
        data = resource.setdefault("data", {})
        data.update(
            {
                "APP_PUBLIC_URL": "http://localhost:8080",
                "AUTH_COOKIE_SECURE": "false",
                "TEMPORAL_ADDRESS": "temporal:7233",
                "TEMPORAL_NAMESPACE": "default",
                "TEMPORAL_TASK_QUEUE": "soniq-audio-pipeline",
                "STORAGE_PROVIDER": "s3_compatible",
                "S3_ENDPOINT": "http://minio:9000",
                "S3_REGION": "us-east-1",
                "S3_BUCKET": s3_bucket,
                "S3_FORCE_PATH_STYLE": "true",
                "TRANSCRIPTION_PROVIDER": "fake_transcription",
                "LLM_PROVIDER": "fake_llm",
                "PRIVACY_ALLOW_EXTERNAL_MODEL_PROVIDERS": "false",
            }
        )
    if kind == "Secret" and name == "soniq-secret":
        data = resource.setdefault("stringData", {})
        data.update(
            {
                "POSTGRES_DSN": f"postgres://{postgres_user}:{postgres_password}@soniq-postgresql:5432/{postgres_db}?sslmode=disable",
                "S3_ACCESS_KEY": s3_access_key,
                "S3_SECRET_KEY": s3_secret_key,
            }
        )


def external_service(name, port_name, port):
    return {
        "apiVersion": "v1",
        "kind": "Service",
        "metadata": {
            "name": name,
            "namespace": "soniq",
            "labels": {
                "app.kubernetes.io/name": name,
                "app.kubernetes.io/part-of": "soniq",
                "soniq.dev/smoke-dependency": "true",
            },
        },
        "spec": {
            "ports": [
                {
                    "name": port_name,
                    "port": port,
                    "targetPort": port,
                }
            ]
        },
    }


def external_endpoint_slice(name, port_name, port, ip):
    return {
        "apiVersion": "discovery.k8s.io/v1",
        "kind": "EndpointSlice",
        "metadata": {
            "name": f"{name}-smoke",
            "namespace": "soniq",
            "labels": {
                "app.kubernetes.io/name": name,
                "app.kubernetes.io/part-of": "soniq",
                "soniq.dev/smoke-dependency": "true",
                "kubernetes.io/service-name": name,
                "endpointslice.kubernetes.io/managed-by": "soniq-kind-smoke",
            },
        },
        "addressType": "IPv4",
        "ports": [
            {
                "name": port_name,
                "protocol": "TCP",
                "port": port,
            }
        ],
        "endpoints": [{"addresses": [ip]}],
    }


resources.extend(
    [
        external_service("soniq-postgresql", "postgres", 5432),
        external_endpoint_slice("soniq-postgresql", "postgres", 5432, postgres_ip),
        external_service("temporal", "grpc", 7233),
        external_endpoint_slice("temporal", "grpc", 7233, temporal_ip),
        external_service("minio", "s3", 9000),
        external_endpoint_slice("minio", "s3", 9000, minio_ip),
    ]
)

with open(output_path, "w", encoding="utf-8") as handle:
    yaml.safe_dump_all(resources, handle, sort_keys=False)
PY
}

render_external_dependency_manifest() {
  local postgres_ip="$1"
  local temporal_ip="$2"
  local minio_ip="$3"

  log "rendering external dependency smoke manifest"
  python3 - "$SMOKE_MANIFEST" "$postgres_ip" "$temporal_ip" "$minio_ip" "$K8S_NAMESPACE" <<'PY'
import sys

import yaml

output_path, postgres_ip, temporal_ip, minio_ip, namespace = sys.argv[1:]


def external_service(name, port_name, port):
    return {
        "apiVersion": "v1",
        "kind": "Service",
        "metadata": {
            "name": name,
            "namespace": namespace,
            "labels": {
                "app.kubernetes.io/name": name,
                "app.kubernetes.io/part-of": "soniq",
                "soniq.dev/smoke-dependency": "true",
            },
        },
        "spec": {
            "ports": [
                {
                    "name": port_name,
                    "port": port,
                    "targetPort": port,
                }
            ]
        },
    }


def external_endpoint_slice(name, port_name, port, ip):
    return {
        "apiVersion": "discovery.k8s.io/v1",
        "kind": "EndpointSlice",
        "metadata": {
            "name": f"{name}-smoke",
            "namespace": namespace,
            "labels": {
                "app.kubernetes.io/name": name,
                "app.kubernetes.io/part-of": "soniq",
                "soniq.dev/smoke-dependency": "true",
                "kubernetes.io/service-name": name,
                "endpointslice.kubernetes.io/managed-by": "soniq-kind-smoke",
            },
        },
        "addressType": "IPv4",
        "ports": [
            {
                "name": port_name,
                "protocol": "TCP",
                "port": port,
            }
        ],
        "endpoints": [{"addresses": [ip]}],
    }


resources = [
    external_service("soniq-postgresql", "postgres", 5432),
    external_endpoint_slice("soniq-postgresql", "postgres", 5432, postgres_ip),
    external_service("temporal", "grpc", 7233),
    external_endpoint_slice("temporal", "grpc", 7233, temporal_ip),
    external_service("minio", "s3", 9000),
    external_endpoint_slice("minio", "s3", 9000, minio_ip),
]

with open(output_path, "w", encoding="utf-8") as handle:
    yaml.safe_dump_all(resources, handle, sort_keys=False)
PY
}

deploy_smoke_manifest() {
  local postgres_ip="$1"
  local temporal_ip="$2"
  local minio_ip="$3"

  render_smoke_manifest "$postgres_ip" "$temporal_ip" "$minio_ip"

  log "applying smoke manifest"
  kubectl apply -f "$SMOKE_MANIFEST"

  log "waiting for migration job"
  if ! kubectl -n "$K8S_NAMESPACE" wait --for=condition=complete job/soniq-migrate --timeout=180s; then
    show_debug_context
    return 1
  fi
  kubectl -n "$K8S_NAMESPACE" logs job/soniq-migrate
}

deploy_smoke_helm_release() {
  local postgres_ip="$1"
  local temporal_ip="$2"
  local minio_ip="$3"
  local postgres_dsn

  render_external_dependency_manifest "$postgres_ip" "$temporal_ip" "$minio_ip"

  log "creating namespace $K8S_NAMESPACE for Helm smoke dependencies"
  kubectl create namespace "$K8S_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

  log "applying external dependency smoke manifest"
  kubectl apply -f "$SMOKE_MANIFEST"

  postgres_dsn="postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@soniq-postgresql:5432/$POSTGRES_DB?sslmode=disable"

  log "creating external Secret soniq-secret for Helm migration hook"
  kubectl -n "$K8S_NAMESPACE" create secret generic soniq-secret \
    --from-literal=POSTGRES_DSN="$postgres_dsn" \
    --from-literal=S3_ACCESS_KEY="$S3_ACCESS_KEY" \
    --from-literal=S3_SECRET_KEY="$S3_SECRET_KEY" \
    --from-literal=TRANSCRIPTION_API_KEY="" \
    --from-literal=MIMO_API_KEY="" \
    --from-literal=DASHSCOPE_API_KEY="" \
    --from-literal=LLM_API_KEY="" \
    --dry-run=client -o yaml | kubectl apply -f -

  log "installing Helm release $HELM_RELEASE from $HELM_CHART"
  run_helm upgrade --install "$HELM_RELEASE" "$HELM_CHART" \
    --namespace "$K8S_NAMESPACE" \
    --wait \
    --timeout 180s \
    --set fullnameOverride=soniq \
    --set secret.name=soniq-secret \
    --set-string config.data.APP_PUBLIC_URL="http://localhost:$KIND_SMOKE_API_PORT" \
    --set-string config.data.AUTH_COOKIE_SECURE=false \
    --set-string config.data.TEMPORAL_ADDRESS=temporal:7233 \
    --set-string config.data.TEMPORAL_NAMESPACE=default \
    --set-string config.data.TEMPORAL_TASK_QUEUE=soniq-audio-pipeline \
    --set-string config.data.STORAGE_PROVIDER=s3_compatible \
    --set-string config.data.S3_ENDPOINT=http://minio:9000 \
    --set-string config.data.S3_REGION=us-east-1 \
    --set-string config.data.S3_BUCKET="$S3_BUCKET" \
    --set-string config.data.S3_FORCE_PATH_STYLE=true \
    --set-string config.data.TRANSCRIPTION_PROVIDER=fake_transcription \
    --set-string config.data.LLM_PROVIDER=fake_llm \
    --set-string config.data.PRIVACY_ALLOW_EXTERNAL_MODEL_PROVIDERS=false

  run_helm -n "$K8S_NAMESPACE" status "$HELM_RELEASE"
}

show_debug_context() {
  log "recent pod status"
  kubectl -n "$K8S_NAMESPACE" get pods -o wide || true
  if [[ "$KIND_SMOKE_DEPLOYER" == "helm" ]]; then
    log "helm release status"
    run_helm -n "$K8S_NAMESPACE" status "$HELM_RELEASE" || true
  fi
  log "migrate logs"
  kubectl -n "$K8S_NAMESPACE" logs job/soniq-migrate || true
  log "api logs"
  kubectl -n "$K8S_NAMESPACE" logs deploy/soniq-api --tail=100 || true
  log "worker logs"
  kubectl -n "$K8S_NAMESPACE" logs deploy/soniq-worker --tail=100 || true
}

main() {
  cd "$ROOT_DIR"
  require_command docker
  require_command kind
  require_command kubectl
  if [[ "$KIND_SMOKE_DEPLOYER" == "helm" ]]; then
    require_helm
  elif [[ "$KIND_SMOKE_DEPLOYER" != "kubectl" ]]; then
    printf 'KIND_SMOKE_DEPLOYER must be kubectl or helm, got %s\n' "$KIND_SMOKE_DEPLOYER" >&2
    exit 2
  fi
  require_command python3
  python3 -c 'import yaml' >/dev/null 2>&1 || {
    printf 'python3 with PyYAML is required for kind Kubernetes smoke\n' >&2
    exit 127
  }

  log "starting Compose dependencies"
  docker compose -f "$COMPOSE_FILE" up -d

  wait_for_command "Soniq Postgres" 60 \
    docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql pg_isready -U "$POSTGRES_USER"
  wait_for_command "Temporal frontend" 60 \
    docker compose -f "$COMPOSE_FILE" exec -T temporal temporal --address temporal:7233 operator namespace list
  wait_for_command "MinIO bucket $S3_BUCKET" 60 assert_s3_bucket_ready

  ensure_kind_cluster

  local postgres_container temporal_container minio_container compose_network
  postgres_container="$(compose_container_id soniq-postgresql)"
  temporal_container="$(compose_container_id temporal)"
  minio_container="$(compose_container_id minio)"
  compose_network="$(container_network_name "$postgres_container")"
  if [[ -z "$compose_network" ]]; then
    log "could not determine Compose network from soniq-postgresql"
    return 1
  fi
  ensure_kind_node_on_compose_network "$compose_network"

  local postgres_ip temporal_ip minio_ip
  postgres_ip="$(container_ip_on_network "$postgres_container" "$compose_network")"
  temporal_ip="$(container_ip_on_network "$temporal_container" "$compose_network")"
  minio_ip="$(container_ip_on_network "$minio_container" "$compose_network")"
  log "Compose dependency IPs on $compose_network: postgres=$postgres_ip temporal=$temporal_ip minio=$minio_ip"

  if [[ "$KIND_SMOKE_BUILD_IMAGES" == "1" ]]; then
    log "building Soniq runtime images"
    make docker-build
  fi
  log "loading Soniq images into kind cluster $KIND_CLUSTER_NAME"
  kind load docker-image soniq-api:dev soniq-worker:dev soniq-migrate:dev --name "$KIND_CLUSTER_NAME"

  if [[ "$KIND_SMOKE_CLEAN_NAMESPACE" == "1" ]]; then
    log "cleaning namespace $K8S_NAMESPACE"
    kubectl delete namespace "$K8S_NAMESPACE" --ignore-not-found
    while kubectl get namespace "$K8S_NAMESPACE" >/dev/null 2>&1; do
      sleep 1
    done
  fi

  if [[ "$KIND_SMOKE_DEPLOYER" == "helm" ]]; then
    deploy_smoke_helm_release "$postgres_ip" "$temporal_ip" "$minio_ip"
  else
    deploy_smoke_manifest "$postgres_ip" "$temporal_ip" "$minio_ip"
  fi

  log "waiting for API and worker deployments"
  if ! kubectl -n "$K8S_NAMESPACE" rollout status deployment/soniq-api --timeout=180s; then
    show_debug_context
    return 1
  fi
  if ! kubectl -n "$K8S_NAMESPACE" rollout status deployment/soniq-worker --timeout=180s; then
    show_debug_context
    return 1
  fi

  log "port-forwarding soniq-api on localhost:$KIND_SMOKE_API_PORT"
  kubectl -n "$K8S_NAMESPACE" port-forward service/soniq-api "$KIND_SMOKE_API_PORT:80" >"$PORT_FORWARD_LOG" 2>&1 &
  PORT_FORWARD_PID=$!
  wait_for_command "API /healthz" 30 curl -fsS "http://localhost:$KIND_SMOKE_API_PORT/healthz"
  wait_for_command "API /readyz" 30 curl -fsS "http://localhost:$KIND_SMOKE_API_PORT/readyz"
  if [[ "$KIND_SMOKE_WORKFLOW" == "1" ]]; then
    run_workflow_smoke "http://localhost:$KIND_SMOKE_API_PORT"
  else
    log "skipping workflow smoke; set KIND_SMOKE_WORKFLOW=1 to enable it"
  fi

  log "passed"
}

main "$@"
