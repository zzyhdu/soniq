ALTER TABLE recordings
  ADD COLUMN audio_object_key TEXT NOT NULL DEFAULT '',
  ADD COLUMN audio_content_type TEXT NOT NULL DEFAULT '',
  ADD COLUMN audio_size_bytes BIGINT NOT NULL DEFAULT 0;
