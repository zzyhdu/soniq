import { type SoniqApiClientOptions, requestJSON } from './http';

export type WorkflowType = 'memo' | 'meeting' | 'lecture' | 'interview';

export type RecordingStatus =
  | 'uploaded'
  | 'processing'
  | 'transcribing'
  | 'summarizing'
  | 'completed'
  | 'failed'
  | 'cancelled';

export type Recording = {
  id: string;
  workspace_id: string;
  title: string;
  status: RecordingStatus;
  workflow_type: WorkflowType;
  language: string;
  audio_object_key?: string;
  audio_content_type?: string;
  audio_size_bytes?: number;
  failure_reason?: string;
  completed_at?: string;
  failed_at?: string;
  created_at: string;
  updated_at: string;
};

export type CreateRecordingInput = {
  workflow_type: WorkflowType;
  title?: string;
  language?: string;
};

export type ListRecordingsInput = {
  limit?: number;
};

export type ListRecordingsResponse = {
  recordings: Recording[];
};

export type UploadRecordingInput = {
  audio: File | Blob;
  workflow_type: WorkflowType;
  title?: string;
  language?: string;
};

export type UploadRecordingResponse = {
  recording: Recording;
  processing_enqueued: boolean;
};

export type RetryRecordingResponse = {
  recording: Recording;
  processing_enqueued: boolean;
};

export type RecordingStatusResponse = {
  id: string;
  workspace_id: string;
  status: RecordingStatus;
  failure_reason?: string;
  completed_at?: string;
  failed_at?: string;
};

export type RecordingTranscript = {
  recording_id: string;
  provider: string;
  model: string;
  language: string;
  text: string;
  transcribed_at: string;
};

export type RecordingTranscriptSegment = {
  id: string;
  recording_id: string;
  segment_index: number;
  start_ms: number;
  end_ms: number;
  speaker_label: string;
  text: string;
  confidence: number;
};

export type RecordingSummary = {
  recording_id: string;
  provider: string;
  model: string;
  type: WorkflowType;
  title: string;
  overview: string;
  content_markdown: string;
  summarized_at: string;
};

export type RecordingDetails = {
  recording: Recording;
  transcript?: RecordingTranscript | null;
  segments: RecordingTranscriptSegment[];
  summary?: RecordingSummary | null;
};

export async function listRecordings(
  workspaceId: string,
  input: ListRecordingsInput = {},
  options: SoniqApiClientOptions = {},
): Promise<ListRecordingsResponse> {
  const query = new URLSearchParams();
  if (input.limit !== undefined) {
    query.set('limit', String(input.limit));
  }

  const suffix = query.size > 0 ? `?${query.toString()}` : '';
  return requestJSON<ListRecordingsResponse>(
    `${workspaceRecordingsPath(workspaceId)}${suffix}`,
    { method: 'GET' },
    options,
  );
}

export async function createRecording(
  workspaceId: string,
  input: CreateRecordingInput,
  options: SoniqApiClientOptions = {},
): Promise<Recording> {
  return requestJSON<Recording>(
    workspaceRecordingsPath(workspaceId),
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    },
    options,
  );
}

export async function uploadRecording(
  workspaceId: string,
  input: UploadRecordingInput,
  options: SoniqApiClientOptions = {},
): Promise<UploadRecordingResponse> {
  const formData = new FormData();
  formData.append('workflow_type', input.workflow_type);
  formData.append('audio', input.audio);

  if (input.title !== undefined) {
    formData.append('title', input.title);
  }

  if (input.language !== undefined) {
    formData.append('language', input.language);
  }

  return requestJSON<UploadRecordingResponse>(
    `${workspaceRecordingsPath(workspaceId)}/upload`,
    {
      method: 'POST',
      body: formData,
    },
    options,
  );
}

export async function getRecording(
  workspaceId: string,
  recordingId: string,
  options: SoniqApiClientOptions = {},
): Promise<Recording> {
  return requestJSON<Recording>(
    recordingPath(workspaceId, recordingId),
    { method: 'GET' },
    options,
  );
}

export async function getRecordingStatus(
  workspaceId: string,
  recordingId: string,
  options: SoniqApiClientOptions = {},
): Promise<RecordingStatusResponse> {
  return requestJSON<RecordingStatusResponse>(
    `${recordingPath(workspaceId, recordingId)}/status`,
    { method: 'GET' },
    options,
  );
}

export async function getRecordingDetails(
  workspaceId: string,
  recordingId: string,
  options: SoniqApiClientOptions = {},
): Promise<RecordingDetails> {
  return requestJSON<RecordingDetails>(
    `${recordingPath(workspaceId, recordingId)}/details`,
    { method: 'GET' },
    options,
  );
}

export async function retryRecording(
  workspaceId: string,
  recordingId: string,
  options: SoniqApiClientOptions = {},
): Promise<RetryRecordingResponse> {
  return requestJSON<RetryRecordingResponse>(
    `${recordingPath(workspaceId, recordingId)}/retry`,
    { method: 'POST' },
    options,
  );
}

function workspaceRecordingsPath(workspaceId: string): string {
  return `/workspaces/${encodeURIComponent(workspaceId)}/recordings`;
}

function recordingPath(workspaceId: string, recordingId: string): string {
  return `${workspaceRecordingsPath(workspaceId)}/${encodeURIComponent(recordingId)}`;
}
