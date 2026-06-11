DROP INDEX IF EXISTS recordings_workspace_created_at_idx;

ALTER TABLE recordings
  DROP CONSTRAINT IF EXISTS recordings_workspace_id_fkey,
  DROP COLUMN IF EXISTS workspace_id;

DROP TABLE IF EXISTS workspace_members;
DROP TABLE IF EXISTS workspaces;
DROP TABLE IF EXISTS users;
