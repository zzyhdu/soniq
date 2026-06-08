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
  title: string;
  status: RecordingStatus;
  workflow_type: WorkflowType;
  language: string;
  audio_object_key?: string;
  audio_content_type?: string;
  audio_size_bytes?: number;
  created_at: string;
  updated_at: string;
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

export type RecordingStatusResponse = {
  id: string;
  status: RecordingStatus;
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

export type SoniqApiClientOptions = {
  baseUrl?: string;
  fetch?: typeof fetch;
};

export class SoniqApiError extends Error {
  readonly status: number;
  readonly statusText: string;
  readonly body: unknown;

  constructor(message: string, status: number, statusText: string, body: unknown) {
    super(message);
    this.name = 'SoniqApiError';
    this.status = status;
    this.statusText = statusText;
    this.body = body;
  }
}

export async function uploadRecording(
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

  return requestJSON<UploadRecordingResponse>('/recordings/upload', {
    method: 'POST',
    body: formData,
  }, options);
}

export async function getRecordingStatus(
  recordingId: string,
  options: SoniqApiClientOptions = {},
): Promise<RecordingStatusResponse> {
  return requestJSON<RecordingStatusResponse>(
    `/recordings/${encodeURIComponent(recordingId)}/status`,
    { method: 'GET' },
    options,
  );
}

export async function getRecordingDetails(
  recordingId: string,
  options: SoniqApiClientOptions = {},
): Promise<RecordingDetails> {
  return requestJSON<RecordingDetails>(
    `/recordings/${encodeURIComponent(recordingId)}/details`,
    { method: 'GET' },
    options,
  );
}

async function requestJSON<T>(
  path: string,
  init: RequestInit,
  options: SoniqApiClientOptions,
): Promise<T> {
  const fetchImpl = options.fetch ?? globalThis.fetch;
  const response = await fetchImpl(buildUrl(path, options.baseUrl), init);
  const body = await parseResponseBody(response);

  if (!response.ok) {
    throw new SoniqApiError(errorMessage(body, response), response.status, response.statusText, body);
  }

  return body as T;
}

async function parseResponseBody(response: Response): Promise<unknown> {
  const text = await response.text();
  if (text.length === 0) {
    return null;
  }

  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

function errorMessage(body: unknown, response: Response): string {
  if (typeof body === 'string' && body.length > 0) {
    return body;
  }

  if (body !== null && typeof body === 'object') {
    const maybeMessage = (body as { message?: unknown; error?: unknown }).message ??
      (body as { message?: unknown; error?: unknown }).error;
    if (typeof maybeMessage === 'string' && maybeMessage.length > 0) {
      return maybeMessage;
    }
  }

  return response.statusText || `HTTP ${response.status}`;
}

function buildUrl(path: string, baseUrl?: string): string {
  if (baseUrl === undefined || baseUrl.length === 0) {
    return path;
  }

  return `${baseUrl.replace(/\/$/, '')}${path}`;
}
