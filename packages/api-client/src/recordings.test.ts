import { describe, expect, it, vi } from 'vitest';

import {
  SoniqApiError,
  getRecordingDetails,
  getRecordingStatus,
  uploadRecording,
} from './recordings';

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
}

describe('uploadRecording', () => {
  it('sends multipart form data to the upload endpoint', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse({
        recording: recordingFixture({ id: 'rec-1' }),
        processing_enqueued: true,
      }, { status: 201 }),
    );
    const audio = new File(['audio-bytes'], 'meeting.mp3', { type: 'audio/mpeg' });

    const result = await uploadRecording(
      {
        audio,
        workflow_type: 'meeting',
        title: 'Weekly standup',
        language: 'zh',
      },
      { fetch: fetchMock },
    );

    expect(result.processing_enqueued).toBe(true);
    expect(fetchMock).toHaveBeenCalledWith('/recordings/upload', {
      method: 'POST',
      body: expect.any(FormData),
    });

    const formData = fetchMock.mock.calls[0]?.[1]?.body as FormData;
    expect(formData.get('audio')).toBe(audio);
    expect(formData.get('workflow_type')).toBe('meeting');
    expect(formData.get('title')).toBe('Weekly standup');
    expect(formData.get('language')).toBe('zh');
  });

  it('omits optional upload fields when they are not provided', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse({
        recording: recordingFixture({ id: 'rec-2' }),
        processing_enqueued: false,
      }, { status: 201 }),
    );
    const audio = new File(['audio-bytes'], 'memo.wav', { type: 'audio/wav' });

    await uploadRecording({ audio, workflow_type: 'memo' }, { fetch: fetchMock });

    const formData = fetchMock.mock.calls[0]?.[1]?.body as FormData;
    expect(formData.get('audio')).toBe(audio);
    expect(formData.get('workflow_type')).toBe('memo');
    expect(formData.has('title')).toBe(false);
    expect(formData.has('language')).toBe(false);
  });
});

describe('recording reads', () => {
  it('URL-encodes recording ids for status and details requests', async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse({ id: 'rec/with space', status: 'completed' }))
      .mockResolvedValueOnce(jsonResponse({
        recording: recordingFixture({ id: 'rec/with space' }),
        transcript: null,
        segments: [],
        summary: null,
      }));

    await getRecordingStatus('rec/with space', { fetch: fetchMock });
    await getRecordingDetails('rec/with space', { fetch: fetchMock });

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/recordings/rec%2Fwith%20space/status', { method: 'GET' });
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/recordings/rec%2Fwith%20space/details', { method: 'GET' });
  });
});

describe('API errors', () => {
  it('turns JSON error responses into typed errors', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse({ message: 'recording not found' }, { status: 404, statusText: 'Not Found' }),
    );

    const errorPromise = getRecordingStatus('missing', { fetch: fetchMock });

    await expect(errorPromise).rejects.toMatchObject({
      name: 'SoniqApiError',
      status: 404,
      message: 'recording not found',
    });
    await expect(errorPromise).rejects.toBeInstanceOf(SoniqApiError);
  });
});

type RecordingFixture = {
  id: string;
  title: string;
  status: string;
  workflow_type: string;
  language: string;
  created_at: string;
  updated_at: string;
};

function recordingFixture(overrides: Partial<RecordingFixture> = {}): RecordingFixture {
  return {
    id: 'rec-1',
    title: 'Recording',
    status: 'uploaded',
    workflow_type: 'meeting',
    language: 'zh',
    created_at: '2026-06-07T00:00:00Z',
    updated_at: '2026-06-07T00:00:00Z',
    ...overrides,
  };
}
