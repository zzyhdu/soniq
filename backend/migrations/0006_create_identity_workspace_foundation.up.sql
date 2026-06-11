CREATE TABLE users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE workspaces (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_by_user_id TEXT REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE workspace_members (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (workspace_id, user_id),
  CONSTRAINT workspace_members_role_check
    CHECK (role IN ('owner', 'member'))
);

INSERT INTO users (id, email, display_name, created_at, updated_at)
VALUES ('usr_dev', 'dev@local.soniq', 'Local Developer', NOW(), NOW());

INSERT INTO workspaces (id, name, created_by_user_id, created_at, updated_at)
VALUES ('wsp_default', 'Default Workspace', 'usr_dev', NOW(), NOW());

INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
VALUES ('wsp_default', 'usr_dev', 'owner', NOW());

ALTER TABLE recordings
  ADD COLUMN workspace_id TEXT;

UPDATE recordings
SET workspace_id = 'wsp_default'
WHERE workspace_id IS NULL;

ALTER TABLE recordings
  ALTER COLUMN workspace_id SET NOT NULL,
  ADD CONSTRAINT recordings_workspace_id_fkey
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id);

CREATE INDEX recordings_workspace_created_at_idx
  ON recordings (workspace_id, created_at DESC);
