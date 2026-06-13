DO $$
BEGIN
  IF to_regclass('public.schema_migrations') IS NOT NULL THEN
    DELETE FROM schema_migrations WHERE version = '3';
  END IF;
END $$;

DROP TABLE IF EXISTS user_sessions;

ALTER TABLE users
  DROP COLUMN IF EXISTS password_hash;
