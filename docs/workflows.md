# Temporal Workflows

Soniq uses Temporal for durable audio processing workflows.

## Current implementation status

The repository currently contains a Temporal workflow skeleton, not the full audio pipeline described below. The implemented boundary uses the real Temporal Go SDK and Temporal SDK testsuite, registers `RecordingProcessingWorkflow`, and provides activity stubs for validation and recording status transitions. It does not yet call ffmpeg, object storage, ASR providers, LLM providers, Postgres persistence, provider webhooks, or a production Temporal smoke test.

## Primary workflow

`RecordingProcessingWorkflow`

Input:

```go
type RecordingProcessingInput struct {
    RecordingID string
    WorkspaceID string
    AudioObjectKey string
    WorkflowType string // memo, meeting, lecture, interview
    Language string // optional, auto if empty
    TranscriptionProvider string
    LLMProvider string
    EnableDiarization bool
    GenerateTitle bool
}
```

## Step sequence

```txt
ValidateRecording
  ↓
ProbeAudio
  ↓
NormalizeAudio
  ↓
SplitAudioIfNeeded
  ↓
TranscribeAudio / TranscribeChunks
  ↓
MergeTranscriptChunks
  ↓
CleanupTranscript
  ↓
GenerateSummary
  ↓
GenerateTitle / GenerateActionItems / GenerateTags
  ↓
PersistFinalResult
  ↓
NotifyCompletion
```

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

- ffmpeg probe/normalize: short retry, deterministic output path.
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
