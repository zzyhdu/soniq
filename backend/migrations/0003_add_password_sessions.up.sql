ALTER TABLE users
  ADD COLUMN password_hash TEXT NOT NULL DEFAULT '';

CREATE TABLE user_sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL,
  last_seen_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ
);

CREATE INDEX user_sessions_user_id_idx
  ON user_sessions (user_id);

CREATE INDEX user_sessions_expires_at_idx
  ON user_sessions (expires_at);
