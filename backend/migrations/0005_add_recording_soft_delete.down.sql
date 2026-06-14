DROP INDEX IF EXISTS recordings_workspace_deleted_at_idx;
DROP INDEX IF EXISTS recordings_workspace_active_created_at_idx;

ALTER TABLE recordings
  DROP COLUMN IF EXISTS deleted_by_user_id,
  DROP COLUMN IF EXISTS deleted_at;
