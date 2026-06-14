ALTER TABLE recordings
  ADD COLUMN deleted_at TIMESTAMPTZ,
  ADD COLUMN deleted_by_user_id TEXT REFERENCES users(id);

CREATE INDEX recordings_workspace_active_created_at_idx
  ON recordings (workspace_id, created_at DESC, id DESC)
  WHERE deleted_at IS NULL;

CREATE INDEX recordings_workspace_deleted_at_idx
  ON recordings (workspace_id, deleted_at DESC, id DESC)
  WHERE deleted_at IS NOT NULL;
