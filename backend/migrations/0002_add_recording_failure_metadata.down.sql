DO $$
BEGIN
  IF to_regclass('public.schema_migrations') IS NOT NULL THEN
    DELETE FROM schema_migrations WHERE version = '2';
  END IF;
END $$;

ALTER TABLE recordings
  DROP COLUMN IF EXISTS failed_at,
  DROP COLUMN IF EXISTS completed_at,
  DROP COLUMN IF EXISTS failure_reason;
