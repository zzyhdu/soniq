DO $$
BEGIN
  IF to_regclass('public.schema_migrations') IS NOT NULL THEN
    DELETE FROM schema_migrations WHERE version IN ('1', '2', '3');
  END IF;
END $$;

DROP TABLE IF EXISTS recording_normalized_audios;
DROP TABLE IF EXISTS recording_summaries;
DROP TABLE IF EXISTS recording_transcript_segments;
DROP TABLE IF EXISTS recording_transcripts;
DROP TABLE IF EXISTS recording_audio_probes;
DROP TABLE IF EXISTS recordings;
DROP TABLE IF EXISTS workspace_members;
DROP TABLE IF EXISTS workspaces;
DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS users;
