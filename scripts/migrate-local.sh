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

password_sessions_present() {
  [[ "$(psql_scalar "SELECT
    to_regclass('public.user_sessions') IS NOT NULL
    AND (
      SELECT count(*) = 1
      FROM information_schema.columns
      WHERE table_schema = 'public'
        AND table_name = 'users'
        AND column_name = 'password_hash'
    )
    AND to_regclass('public.user_sessions_user_id_idx') IS NOT NULL
    AND to_regclass('public.user_sessions_expires_at_idx') IS NOT NULL")" == "t" ]]
}

apply_password_sessions_v3() {
  if migration_applied "3"; then
    log "password session schema version 3 already recorded; skipping"
    return 0
  fi

  if password_sessions_present; then
    log "password session schema already present; recording version 3"
    record_migration "3"
    return 0
  fi

  log "applying password session schema version 3"
  psql_apply "backend/migrations/0003_add_password_sessions.up.sql"
  record_migration "3"
  log "recorded password session schema version 3"
}

recording_mind_maps_present() {
  [[ "$(psql_scalar "SELECT
    to_regclass('public.recording_mind_maps') IS NOT NULL
    AND (
      SELECT count(*) = 10
      FROM information_schema.columns
      WHERE table_schema = 'public'
        AND table_name = 'recording_mind_maps'
        AND column_name IN ('recording_id', 'provider', 'model', 'title', 'root_json', 'content_markdown', 'raw_result_json', 'generated_at', 'created_at', 'updated_at')
    )")" == "t" ]]
}

apply_recording_mind_maps_v4() {
  if migration_applied "4"; then
    log "recording mind maps version 4 already recorded; skipping"
    return 0
  fi

  if recording_mind_maps_present; then
    log "recording mind maps already present; recording version 4"
    record_migration "4"
    return 0
  fi

  log "applying recording mind maps version 4"
  psql_apply "backend/migrations/0004_add_recording_mind_maps.up.sql"
  record_migration "4"
  log "recorded recording mind maps version 4"
}

recording_soft_delete_present() {
  [[ "$(psql_scalar "SELECT
    (
      SELECT count(*) = 2
      FROM information_schema.columns
      WHERE table_schema = 'public'
        AND table_name = 'recordings'
        AND column_name IN ('deleted_at', 'deleted_by_user_id')
    )
    AND to_regclass('public.recordings_workspace_active_created_at_idx') IS NOT NULL
    AND to_regclass('public.recordings_workspace_deleted_at_idx') IS NOT NULL")" == "t" ]]
}

apply_recording_soft_delete_v5() {
  if migration_applied "5"; then
    log "recording soft delete version 5 already recorded; skipping"
    return 0
  fi

  if recording_soft_delete_present; then
    log "recording soft delete already present; recording version 5"
    record_migration "5"
    return 0
  fi

  log "applying recording soft delete version 5"
  psql_apply "backend/migrations/0005_add_recording_soft_delete.up.sql"
  record_migration "5"
  log "recorded recording soft delete version 5"
}

main() {
  cd "$ROOT_DIR"

  ensure_schema_migrations_table
  apply_baseline_v1
  apply_recording_failure_metadata_v2
  apply_password_sessions_v3
  apply_recording_mind_maps_v4
  apply_recording_soft_delete_v5

  log "local Soniq application migrations are up to date"
}

main "$@"
