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
