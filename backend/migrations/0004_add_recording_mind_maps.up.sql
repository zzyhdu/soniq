CREATE TABLE recording_mind_maps (
  recording_id TEXT PRIMARY KEY REFERENCES recordings(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  model TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  root_json JSONB NOT NULL,
  content_markdown TEXT NOT NULL,
  raw_result_json JSONB NOT NULL,
  generated_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
