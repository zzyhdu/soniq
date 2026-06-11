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

CREATE TABLE recordings (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  workflow_type TEXT NOT NULL,
  language TEXT NOT NULL,
  audio_object_key TEXT NOT NULL DEFAULT '',
  audio_content_type TEXT NOT NULL DEFAULT '',
  audio_size_bytes BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT recordings_status_check
    CHECK (status IN ('uploaded', 'processing', 'transcribing', 'summarizing', 'completed', 'failed', 'cancelled')),
  CONSTRAINT recordings_workflow_type_check
    CHECK (workflow_type IN ('memo', 'meeting', 'lecture', 'interview'))
);

CREATE INDEX recordings_workspace_created_at_idx
  ON recordings (workspace_id, created_at DESC);

CREATE TABLE recording_audio_probes (
  recording_id TEXT PRIMARY KEY REFERENCES recordings(id) ON DELETE CASCADE,
  duration_seconds DOUBLE PRECISION,
  format_name TEXT NOT NULL,
  codec_name TEXT NOT NULL DEFAULT '',
  sample_rate INTEGER NOT NULL DEFAULT 0,
  channels INTEGER NOT NULL DEFAULT 0,
  bit_rate INTEGER NOT NULL DEFAULT 0,
  raw_probe_json JSONB NOT NULL,
  probed_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE recording_transcripts (
  recording_id TEXT PRIMARY KEY REFERENCES recordings(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  model TEXT NOT NULL DEFAULT '',
  language TEXT NOT NULL DEFAULT '',
  text TEXT NOT NULL,
  raw_result_json JSONB NOT NULL,
  transcribed_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE recording_transcript_segments (
  id TEXT PRIMARY KEY,
  recording_id TEXT NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
  segment_index INTEGER NOT NULL,
  start_ms INTEGER NOT NULL DEFAULT 0,
  end_ms INTEGER NOT NULL DEFAULT 0,
  speaker_label TEXT NOT NULL DEFAULT '',
  text TEXT NOT NULL,
  confidence DOUBLE PRECISION,
  created_at TIMESTAMPTZ NOT NULL,
  UNIQUE (recording_id, segment_index)
);

CREATE TABLE recording_summaries (
  recording_id TEXT PRIMARY KEY REFERENCES recordings(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  model TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  overview TEXT NOT NULL,
  content_markdown TEXT NOT NULL,
  raw_result_json JSONB NOT NULL,
  summarized_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT recording_summaries_type_check
    CHECK (type IN ('memo', 'meeting', 'lecture', 'interview'))
);

CREATE TABLE recording_normalized_audios (
  recording_id TEXT PRIMARY KEY REFERENCES recordings(id) ON DELETE CASCADE,
  object_key TEXT NOT NULL,
  content_type TEXT NOT NULL,
  size_bytes BIGINT NOT NULL DEFAULT 0,
  format_name TEXT NOT NULL DEFAULT 'wav',
  codec_name TEXT NOT NULL DEFAULT 'pcm_s16le',
  sample_rate INTEGER NOT NULL DEFAULT 16000,
  channels INTEGER NOT NULL DEFAULT 1,
  duration_seconds DOUBLE PRECISION,
  normalized_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
