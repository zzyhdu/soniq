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
