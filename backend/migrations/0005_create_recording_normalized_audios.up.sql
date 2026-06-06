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
