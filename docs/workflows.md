# Temporal Workflows

Soniq uses Temporal for durable audio processing workflows.

## Current implementation status

The repository currently contains a local end-to-end Temporal processing foundation through original-audio probing, ffmpeg-based audio normalization, deterministic fake transcription, optional original-audio deletion after successful transcription, deterministic fake summarization, and deterministic fake mind map generation. The implemented boundary uses the real Temporal Go SDK and Temporal SDK testsuite, wires successful workspace-scoped audio uploads through a Temporal-backed recording processor, starts `RecordingProcessingWorkflow` asynchronously with workflow ID `recording-processing-<recording_id>`, and registers the workflow on the worker. The API supports metadata-only `POST /workspaces/{workspace_id}/recordings`, multipart `POST /workspaces/{workspace_id}/recordings/upload`, soft delete through `DELETE /workspaces/{workspace_id}/recordings/{recording_id}`, Trash listing through `GET /workspaces/{workspace_id}/recordings/trash`, Trash restore through `POST /workspaces/{workspace_id}/recordings/{recording_id}/restore`, permanent Trash purge through `DELETE /workspaces/{workspace_id}/recordings/{recording_id}/purge`, and failed recording retry through `POST /workspaces/{workspace_id}/recordings/{recording_id}/retry`; metadata-only requests only create recording rows, upload and retry requests enqueue the workflow, soft delete marks `deleted_at` / `deleted_by_user_id` without physically purging artifacts, restore clears the deletion metadata without re-running processing, and purge writes object cleanup rows before deleting recording metadata. The worker connects to Soniq application Postgres and registers store-backed recording activities under stable Temporal activity names, so the current workflow validates recordings by `workspace_id + recording_id`, persists status transitions through `uploaded -> processing -> transcribing -> summarizing -> completed` with workspace-scoped updates, prepares recording audio in one activity by downloading object-store audio to a temporary local file when needed, runs `ffprobe` against that staged original audio, persists one `recording_audio_probes` row, normalizes the same staged input to a WAV/PCM artifact (`pcm_s16le`, 16 kHz, mono), uploads the normalized artifact back through object storage, persists one `recording_normalized_audios` row, persists `recording_transcripts` and `recording_transcript_segments` using the configured transcription provider against a presigned normalized-audio URL, optionally deletes the original uploaded audio object when `PRIVACY_DELETE_ORIGINAL_AUDIO_AFTER_TRANSCRIPTION=true`, persists `recording_summaries`, persists `recording_mind_maps` using the fake LLM provider, and periodically retries pending/failed `recording_purge_artifacts` object cleanup rows, with a best-effort `failed` transition that records `failure_reason` and `failed_at` if audio preparation/transcription/original-audio-deletion/summarization/mind-map generation/completion fails. Real external ASR providers require object storage that can produce URLs reachable by the provider.

New uploaded original audio objects use workspace-scoped keys under `workspaces/{workspace_id}/recordings/`.

## Primary workflow

`RecordingProcessingWorkflow`

Input:

```go
type RecordingProcessingInput struct {
    WorkspaceID string
    RecordingID string
    WorkflowType string // memo, meeting, lecture, interview
    Language string // optional, auto if empty
    DeleteOriginalAudioAfterTranscription bool
}
```

## Step sequence

Current implemented workflow:

```txt
ValidateRecording
  ↓
MarkRecordingProcessing
  ↓
PrepareRecordingAudio
  ↓
MarkRecordingTranscribing
  ↓
TranscribeRecordingAudio  // deterministic fake provider for now
  ↓
DeleteOriginalRecordingAudio  // optional when privacy flag is true
  ↓
MarkRecordingSummarizing
  ↓
SummarizeRecording        // deterministic fake provider for now
  ↓
GenerateMindMap          // deterministic fake provider for now
  ↓
CompleteRecordingProcessing
```

Future workflow steps still planned after this boundary:

```txt
SplitAudioIfNeeded
  ↓
Real provider-backed TranscribeAudio / TranscribeChunks
  ↓
MergeTranscriptChunks
  ↓
CleanupTranscript
  ↓
Provider-backed GenerateSummary / GenerateTitle / GenerateActionItems / GenerateTags
  ↓
NotifyCompletion
```

## Implemented persistence boundary

The current implemented persistence path is deliberately provider-neutral:

```txt
recording.AudioObjectKey
  ↓
PrepareRecordingAudio: object storage GetObject -> temporary local file when needed
  ↓
ffprobe -v error -print_format json -show_format -show_streams
  ↓
recording_audio_probes upsert
  ↓
ffmpeg -y -hide_banner -loglevel error -ac 1 -ar 16000 -c:a pcm_s16le against the same staged input
  ↓
object storage PutObject for normalized.wav + recording_normalized_audios upsert
  ↓
configured transcription provider receives a presigned normalized audio URL
  ↓
recording_transcripts + recording_transcript_segments upsert
  ↓
optional original object delete when PRIVACY_DELETE_ORIGINAL_AUDIO_AFTER_TRANSCRIPTION=true
  ↓
fake summary provider
  ↓
recording_summaries upsert
  ↓
fake mind map provider
  ↓
recording_mind_maps upsert
```

With `STORAGE_PROVIDER=s3_compatible`, API upload, worker fetch/upload,
temporary ASR read URLs, optional original-audio deletion, and purge cleanup use
the configured S3-compatible bucket. `ffprobe` and `ffmpeg` still operate on
temporary local files, while real ASR providers receive the presigned normalized
audio URL instead of local files or Base64 payloads. The fake
providers are deterministic local development providers used to verify
workflow, activity, and persistence wiring without credentials. They are not
real ASR or LLM integrations.

## Workflow types

### Memo

```txt
transcribe → clean transcript → concise summary → optional title/tags
```

For personal notes, thought capture, diary-like voice notes, and short recordings.

### Meeting

```txt
transcribe with speaker labels → summary → decisions → action items → timeline
```

For team meetings, sales calls, customer calls, interviews, and internal syncs.

### Lecture

```txt
transcribe → outline → key concepts → study notes → quiz/flashcards
```

For courses, training sessions, podcasts, and knowledge sharing.

## Asynchronous providers

Some ASR providers use jobs and webhooks. The workflow should support two patterns.

### Polling pattern

```txt
SubmitTranscriptionJob activity
  ↓
PollTranscriptionJob activity with retry/backoff
  ↓
FetchTranscriptionResult activity
```

### Signal pattern

```txt
SubmitTranscriptionJob activity
  ↓
Workflow waits for signal
  ↓
Webhook endpoint receives callback
  ↓
API signals workflow
  ↓
Workflow continues
```

The signal pattern is preferred when provider webhooks are reliable.

## Retry policy guidance

- ffprobe probe / ffmpeg normalize: short retry, deterministic output path.
- third-party ASR/LLM: exponential backoff, provider-specific retryable errors.
- persistence: retry safely with idempotency keys.
- notification/webhook: retry but do not fail the entire recording after final result is persisted.

## Idempotency rules

Activities should use deterministic artifact keys:

```txt
workspaces/{workspace_id}/recordings/{recording_id}/audio/original
workspaces/{workspace_id}/recordings/{recording_id}/audio/normalized.wav
workspaces/{workspace_id}/recordings/{recording_id}/transcripts/raw.v1.json
workspaces/{workspace_id}/recordings/{recording_id}/summaries/summary.v1.json
workspaces/{workspace_id}/recordings/{recording_id}/mind-maps/mind-map.v1.json
```

A retried activity should overwrite or reuse the same output instead of creating unbounded duplicate artifacts.

## Cancellation and reprocessing

The workflow should support:

- User cancellation.
- Retry failed step.
- Re-run transcription with a different provider.
- Re-run summary with a different template/model.
- Delete original audio after successful transcription when retention policy requires it.

## Recording deletion

Current user-facing delete is a soft delete with restore:

- `DELETE /workspaces/{workspace_id}/recordings/{recording_id}` sets `recordings.deleted_at` and `recordings.deleted_by_user_id`.
- `GET /workspaces/{workspace_id}/recordings/trash` lists soft-deleted recordings ordered by `deleted_at DESC, id DESC`.
- `POST /workspaces/{workspace_id}/recordings/{recording_id}/restore` clears `recordings.deleted_at` and `recordings.deleted_by_user_id`.
- `DELETE /workspaces/{workspace_id}/recordings/{recording_id}/purge` permanently deletes a soft-deleted recording and writes object cleanup rows to `recording_purge_artifacts`.
- Active list, detail, status, retry, and workflow persistence reads exclude soft-deleted recordings.
- Transcript, summary, mind map, audio probe, normalized audio metadata, and storage artifacts are retained across soft delete and restore.
- Purge explicitly deletes current recording child rows before deleting the parent recording row, while `ON DELETE CASCADE` remains a database integrity backstop.
- Original and normalized audio object keys are persisted to `recording_purge_artifacts` during purge; the API attempts immediate object deletion and the worker periodically retries pending/failed cleanup rows.
