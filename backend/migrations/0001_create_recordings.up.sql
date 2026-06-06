CREATE TABLE recordings (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  workflow_type TEXT NOT NULL,
  language TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT recordings_status_check
    CHECK (status IN ('uploaded', 'processing', 'transcribing', 'summarizing', 'completed', 'failed', 'cancelled')),
  CONSTRAINT recordings_workflow_type_check
    CHECK (workflow_type IN ('memo', 'meeting', 'lecture', 'interview'))
);
