#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-compose.temporal.yml}"
POSTGRES_USER="${POSTGRES_USER:-soniq_user}"
POSTGRES_DB="${POSTGRES_DB:-soniq}"

log() {
  printf '[migrate] %s\n' "$*"
}

psql_exec() {
  docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -c "$1"
}

psql_scalar() {
  docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "$1"
}

psql_apply() {
  local file="$1"
  docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 \
    -f - < "$ROOT_DIR/$file"
}

ensure_schema_migrations_table() {
  if [[ "$(psql_scalar "SELECT to_regclass('public.schema_migrations') IS NOT NULL")" == "t" ]]; then
    return 0
  fi

  psql_exec "CREATE TABLE schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL
  )" >/dev/null
}

migration_applied() {
  local version="$1"
  [[ "$(psql_scalar "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = '$version')")" == "t" ]]
}

record_migration() {
  local version="$1"
  psql_exec "INSERT INTO schema_migrations (version, applied_at)
    VALUES ('$version', NOW())
    ON CONFLICT (version) DO NOTHING" >/dev/null
}

baseline_v1_present() {
  local schema_exists seeds_exist

  schema_exists="$(psql_scalar "SELECT
    to_regclass('public.users') IS NOT NULL
    AND to_regclass('public.workspaces') IS NOT NULL
    AND to_regclass('public.workspace_members') IS NOT NULL
    AND to_regclass('public.recordings') IS NOT NULL
    AND to_regclass('public.recording_audio_probes') IS NOT NULL
    AND to_regclass('public.recording_transcripts') IS NOT NULL
    AND to_regclass('public.recording_transcript_segments') IS NOT NULL
    AND to_regclass('public.recording_summaries') IS NOT NULL
    AND to_regclass('public.recording_normalized_audios') IS NOT NULL
    AND to_regclass('public.recordings_workspace_created_at_idx') IS NOT NULL
    AND (
      SELECT count(*) = 4
      FROM information_schema.columns
      WHERE table_schema = 'public'
        AND table_name = 'recordings'
        AND column_name IN ('workspace_id', 'audio_object_key', 'audio_content_type', 'audio_size_bytes')
    )")"
  if [[ "$schema_exists" != "t" ]]; then
    return 1
  fi

  seeds_exist="$(psql_scalar "SELECT
    EXISTS (SELECT 1 FROM users WHERE id = 'usr_dev')
    AND EXISTS (SELECT 1 FROM workspaces WHERE id = 'wsp_default')
    AND EXISTS (
      SELECT 1
      FROM workspace_members
      WHERE workspace_id = 'wsp_default'
        AND user_id = 'usr_dev'
        AND role = 'owner'
    )")"
  [[ "$seeds_exist" == "t" ]]
}

baseline_v1_empty() {
  [[ "$(psql_scalar "SELECT
    to_regclass('public.users') IS NULL
    AND to_regclass('public.workspaces') IS NULL
    AND to_regclass('public.workspace_members') IS NULL
    AND to_regclass('public.recordings') IS NULL
    AND to_regclass('public.recording_audio_probes') IS NULL
    AND to_regclass('public.recording_transcripts') IS NULL
    AND to_regclass('public.recording_transcript_segments') IS NULL
    AND to_regclass('public.recording_summaries') IS NULL
    AND to_regclass('public.recording_normalized_audios') IS NULL")" == "t" ]]
}

apply_baseline_v1() {
  if migration_applied "1"; then
    log "baseline schema version 1 already recorded; skipping"
    return 0
  fi

  if baseline_v1_present; then
    log "baseline schema version 1 already present; recording version"
    record_migration "1"
    return 0
  fi

  if ! baseline_v1_empty; then
    log "partial baseline schema detected; inspect or reset the local Soniq application database before migrating"
    return 1
  fi

  log "applying baseline schema version 1"
  psql_apply "backend/migrations/0001_baseline.up.sql"
  record_migration "1"
  log "recorded baseline schema version 1"
}

recording_failure_metadata_present() {
  [[ "$(psql_scalar "SELECT count(*) = 3
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'recordings'
      AND column_name IN ('failure_reason', 'completed_at', 'failed_at')")" == "t" ]]
}

apply_recording_failure_metadata_v2() {
  if migration_applied "2"; then
    log "recording failure metadata version 2 already recorded; skipping"
    return 0
  fi

  if recording_failure_metadata_present; then
    log "recording failure metadata already present; recording version 2"
    record_migration "2"
    return 0
  fi

  log "applying recording failure metadata version 2"
  psql_apply "backend/migrations/0002_add_recording_failure_metadata.up.sql"
  record_migration "2"
  log "recorded recording failure metadata version 2"
}

main() {
  cd "$ROOT_DIR"

  ensure_schema_migrations_table
  apply_baseline_v1
  apply_recording_failure_metadata_v2

  log "local Soniq application migrations are up to date"
}

main "$@"
