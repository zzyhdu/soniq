ALTER TABLE recordings
  ADD COLUMN failure_reason TEXT NOT NULL DEFAULT '',
  ADD COLUMN completed_at TIMESTAMPTZ,
  ADD COLUMN failed_at TIMESTAMPTZ;

UPDATE recordings
SET completed_at = updated_at
WHERE status = 'completed'
  AND completed_at IS NULL;

UPDATE recordings
SET failed_at = updated_at
WHERE status = 'failed'
  AND failed_at IS NULL;
