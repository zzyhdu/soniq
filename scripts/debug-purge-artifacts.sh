#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/.env}"
LIMIT="${LIMIT:-20}"
STUCK_AFTER_MINUTES="${STUCK_AFTER_MINUTES:-10}"

usage() {
  cat <<'USAGE'
Usage: scripts/debug-purge-artifacts.sh [--limit N] [--stuck-after-minutes N]

Shows recording purge artifact cleanup status from Soniq Postgres.

Environment:
  POSTGRES_DSN            Postgres connection string. Defaults to local Soniq DB.
  ENV_FILE                Optional env file to load when POSTGRES_DSN is unset.
  COMPOSE_FILE            Compose file for docker fallback. Default: compose.temporal.yml.
  POSTGRES_USER           Docker fallback Postgres user. Default: soniq_user.
  POSTGRES_DB             Docker fallback database. Default: soniq.
  LIMIT                   Number of failed/deleting rows to show. Default: 20.
  STUCK_AFTER_MINUTES     Age threshold for deleting rows. Default: 10.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --limit)
      LIMIT="${2:-}"
      shift 2
      ;;
    --stuck-after-minutes)
      STUCK_AFTER_MINUTES="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'unknown argument: %s\n\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ ! "$LIMIT" =~ ^[0-9]+$ ]] || [[ "$LIMIT" -le 0 ]]; then
  printf 'LIMIT must be a positive integer\n' >&2
  exit 2
fi

if [[ ! "$STUCK_AFTER_MINUTES" =~ ^[0-9]+$ ]] || [[ "$STUCK_AFTER_MINUTES" -le 0 ]]; then
  printf 'STUCK_AFTER_MINUTES must be a positive integer\n' >&2
  exit 2
fi

if [[ -z "${POSTGRES_DSN:-}" && -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
fi

POSTGRES_DSN="${POSTGRES_DSN:-postgres://soniq_user:soniq_password@localhost:5432/soniq?sslmode=disable}"
COMPOSE_FILE="${COMPOSE_FILE:-compose.temporal.yml}"
POSTGRES_USER="${POSTGRES_USER:-soniq_user}"
POSTGRES_DB="${POSTGRES_DB:-soniq}"

psql_cmd() {
  if command -v psql >/dev/null 2>&1; then
    psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 "$@"
    return
  fi
  if command -v docker >/dev/null 2>&1; then
    docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 "$@"
    return
  fi
  printf 'psql or docker compose is required for debug-purge-artifacts\n' >&2
  exit 127
}

cd "$ROOT_DIR"

printf 'Purge artifact status\n'
psql_cmd -P pager=off -c "
SELECT status, count(*) AS count
FROM recording_purge_artifacts
GROUP BY status
ORDER BY status;
"

printf '\nFailed artifacts\n'
psql_cmd -P pager=off -c "
SELECT
  id,
  recording_id,
  workspace_id,
  artifact_kind,
  attempt_count,
  next_attempt_at,
  COALESCE(NULLIF(left(last_error, 200), ''), '(empty)') AS error
FROM recording_purge_artifacts
WHERE status = 'failed'
ORDER BY updated_at DESC
LIMIT ${LIMIT};
"

printf '\nDeleting artifacts older than %s minutes\n' "$STUCK_AFTER_MINUTES"
psql_cmd -P pager=off -c "
SELECT
  id,
  recording_id,
  workspace_id,
  artifact_kind,
  attempt_count,
  updated_at
FROM recording_purge_artifacts
WHERE status = 'deleting'
  AND updated_at < NOW() - (${STUCK_AFTER_MINUTES} * INTERVAL '1 minute')
ORDER BY updated_at ASC
LIMIT ${LIMIT};
"
