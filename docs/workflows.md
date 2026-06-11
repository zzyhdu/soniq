# Temporal Workflows

Soniq uses Temporal for durable audio processing workflows.

## Current implementation status

The repository currently contains a local end-to-end Temporal processing foundation through original-audio probing, ffmpeg-based audio normalization, deterministic fake transcription, optional original-audio deletion after successful transcription, and deterministic fake summarization. The implemented boundary uses the real Temporal Go SDK and Temporal SDK testsuite, wires successful audio uploads through a Temporal-backed recording processor, starts `RecordingProcessingWorkflow` asynchronously with workflow ID `recording-processing-<recording_id>`, and registers the workflow on the worker. The API supports metadata-only `POST /recordings` and multipart `POST /recordings/upload`; metadata-only requests only create recording rows, while upload requests store the original audio through the local object-store provider and persist audio metadata before starting the workflow. The worker connects to Soniq application Postgres and registers store-backed recording activities under stable Temporal activity names, so the current workflow validates recordings by `workspace_id + recording_id`, persists status transitions through `uploaded -> processing -> transcribing -> summarizing -> completed` with workspace-scoped updates, runs `ffprobe` against the uploaded original audio, persists one `recording_audio_probes` row, normalizes audio to a local WAV/PCM artifact (`pcm_s16le`, 16 kHz, mono), persists one `recording_normalized_audios` row, persists `recording_transcripts` and `recording_transcript_segments` using the fake transcription provider against the normalized audio path, optionally deletes the original uploaded audio object when `PRIVACY_DELETE_ORIGINAL_AUDIO_AFTER_TRANSCRIPTION=true`, and persists `recording_summaries` using the fake summary provider, with a best-effort `failed` transition if probe/normalization/transcription/original-audio-deletion/summarization/completion fails. Real ASR providers, real LLM providers, provider webhooks, and S3-compatible object storage remain future milestones.

New uploaded original audio objects use workspace-scoped keys under `workspaces/{workspace_id}/recordings/`. Existing legacy `recordings/...` object keys remain readable by the local storage resolver as safe relative object keys.

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
ProbeRecordingAudio
  ↓
NormalizeRecordingAudio
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

The current implemented persistence path is deliberately provider-neutral and local-development friendly:

```txt
recording.AudioObjectKey
  ↓
local object path resolver
  ↓
ffprobe -v error -print_format json -show_format -show_streams
  ↓
recording_audio_probes upsert
  ↓
ffmpeg -y -hide_banner -loglevel error -ac 1 -ar 16000 -c:a pcm_s16le
  ↓
recording_normalized_audios upsert + local normalized.wav artifact
  ↓
fake transcription provider reads normalized audio path
  ↓
recording_transcripts + recording_transcript_segments upsert
  ↓
optional original object delete when PRIVACY_DELETE_ORIGINAL_AUDIO_AFTER_TRANSCRIPTION=true
  ↓
fake summary provider
  ↓
recording_summaries upsert
```

The worker resolves local object keys under `LOCAL_STORAGE_PATH`, so the probe, normalization, and fake transcription steps are currently local-storage-only. The fake providers are deterministic local development providers used to verify workflow, activity, and persistence wiring without credentials. They are not real ASR or LLM integrations.

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
```

A retried activity should overwrite or reuse the same output instead of creating unbounded duplicate artifacts.

## Cancellation and reprocessing

The workflow should support:

- User cancellation.
- Retry failed step.
- Re-run transcription with a different provider.
- Re-run summary with a different template/model.
- Delete original audio after successful transcription when retention policy requires it.
