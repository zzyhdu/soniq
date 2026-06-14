CREATE TABLE recording_purge_artifacts (
  id TEXT PRIMARY KEY,
  recording_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL,
  object_key TEXT NOT NULL,
  artifact_kind TEXT NOT NULL,
  status TEXT NOT NULL,
  attempt_count INTEGER NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL,
  last_error TEXT NOT NULL DEFAULT '',
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  UNIQUE (recording_id, object_key),
  CONSTRAINT recording_purge_artifacts_kind_check
    CHECK (artifact_kind IN ('original_audio', 'normalized_audio')),
  CONSTRAINT recording_purge_artifacts_status_check
    CHECK (status IN ('pending', 'deleting', 'deleted', 'failed'))
);

CREATE INDEX recording_purge_artifacts_status_next_attempt_idx
  ON recording_purge_artifacts (status, next_attempt_at, created_at, id)
  WHERE deleted_at IS NULL;

CREATE INDEX recording_purge_artifacts_recording_id_idx
  ON recording_purge_artifacts (recording_id);
