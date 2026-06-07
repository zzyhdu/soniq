# Data Model

This document captures the initial domain model. It is intentionally minimal but leaves room for enterprise features.

## Workspace

```txt
id
display_name
created_at
updated_at
```

A workspace is the tenant boundary.

## User

```txt
id
email
display_name
created_at
updated_at
```

## WorkspaceMember

```txt
workspace_id
user_id
role: owner | admin | member | viewer
created_at
```

## Recording

```txt
id
workspace_id
owner_id
title
status: uploaded | processing | transcribing | summarizing | completed | failed | cancelled
workflow_type: memo | meeting | lecture | interview
language
original_audio_artifact_id
normalized_audio_artifact_id
duration_seconds
created_at
updated_at
completed_at
```

## Artifact

```txt
id
workspace_id
recording_id
type: audio_original | audio_normalized | audio_chunk | transcript_raw | transcript_clean | summary | export
version
object_key
mime_type
size_bytes
checksum_sha256
metadata_json
created_at
```

## TranscriptSegment

```txt
id
recording_id
version
start_ms
end_ms
speaker_id
speaker_name
text
confidence
created_at
```

## Summary

```txt
id
recording_id
version
type: memo | meeting | lecture | interview
format: markdown | json | block_document
title
overview
content_json
model_provider
model_name
prompt_version
created_at
```

## WorkflowRun

```txt
id
recording_id
temporal_workflow_id
temporal_run_id
workflow_type
status: running | completed | failed | cancelled
current_step
error
started_at
completed_at
```

## AuditLog

```txt
id
workspace_id
actor_user_id
action
resource_type
resource_id
metadata_json
created_at
```

Examples:

- `recording.uploaded`
- `workflow.started`
- `transcription.completed`
- `summary.generated`
- `recording.deleted`

## RetentionPolicy

```txt
workspace_id
delete_original_audio_after_transcription
retain_original_audio_days
retain_normalized_audio_days
retain_transcripts_days
retain_summaries_days
allow_external_model_providers
updated_at
```


## Current implemented persistence boundary

The current application Postgres schema is intentionally smaller than the long-term model above. Implemented tables are:

- `recordings` for recording metadata, upload metadata, status, and timestamps.
- `recording_audio_probes` for original-audio `ffprobe` metadata and raw probe JSON.
- `recording_normalized_audios` for the current normalized WAV/PCM artifact metadata (`object_key`, `content_type`, `size_bytes`, `format_name`, `codec_name`, `sample_rate`, `channels`, `normalized_at`).
- `recording_transcripts` for the latest provider-neutral transcript per recording.
- `recording_transcript_segments` for ordered transcript segments.
- `recording_summaries` for the latest provider-neutral summary per recording.

A generic `Artifact` table, workspace/user tenancy tables, workflow run history, and retention policy tables are still future scope. Temporal's internal database remains separate from Soniq application Postgres.
