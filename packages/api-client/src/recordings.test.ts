import { describe, expect, it, vi } from 'vitest';

import { SoniqApiError } from './http';
import {
  createRecording,
  deleteRecording,
  getRecording,
  getRecordingDetails,
  getRecordingStatus,
  listDeletedRecordings,
  listRecordings,
  purgeRecording,
  restoreRecording,
  retryRecording,
  updateRecording,
  uploadRecording,
} from './recordings';

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
}

describe('uploadRecording', () => {
  it('sends multipart form data to the workspace upload endpoint', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse({
        recording: recordingFixture({ id: 'rec-1' }),
        processing_enqueued: true,
      }, { status: 201 }),
    );
    const audio = new File(['audio-bytes'], 'meeting.mp3', { type: 'audio/mpeg' });

    const result = await uploadRecording(
      'wsp_default',
      {
        audio,
        workflow_type: 'meeting',
        title: 'Weekly standup',
        language: 'zh',
      },
      { fetch: fetchMock },
    );

    expect(result.processing_enqueued).toBe(true);
    expect(fetchMock).toHaveBeenCalledWith('/workspaces/wsp_default/recordings/upload', {
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

    await uploadRecording('wsp_default', { audio, workflow_type: 'memo' }, { fetch: fetchMock });

    const formData = fetchMock.mock.calls[0]?.[1]?.body as FormData;
    expect(formData.get('audio')).toBe(audio);
    expect(formData.get('workflow_type')).toBe('memo');
    expect(formData.has('title')).toBe(false);
    expect(formData.has('language')).toBe(false);
  });
});

describe('recording writes', () => {
  it('creates recording metadata in a workspace', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse(recordingFixture({ id: 'rec-created' }), { status: 201 }),
    );

    const result = await createRecording(
      'wsp_default',
      { workflow_type: 'meeting', title: 'Weekly standup', language: 'en' },
      { fetch: fetchMock },
    );

    expect(result.id).toBe('rec-created');
    expect(fetchMock).toHaveBeenCalledWith('/workspaces/wsp_default/recordings', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ workflow_type: 'meeting', title: 'Weekly standup', language: 'en' }),
    });
  });

  it('adds a csrf header to unsafe browser requests', async () => {
    vi.stubGlobal('document', { cookie: 'soniq_csrf=csrf-token' });
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse(recordingFixture({ id: 'rec-created' }), { status: 201 }),
    );

    try {
      await createRecording(
        'wsp_default',
        { workflow_type: 'meeting', title: 'Weekly standup', language: 'en' },
        { fetch: fetchMock },
      );
    } finally {
      vi.unstubAllGlobals();
    }

    expect(fetchMock).toHaveBeenCalledWith('/workspaces/wsp_default/recordings', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': 'csrf-token',
      },
      body: JSON.stringify({ workflow_type: 'meeting', title: 'Weekly standup', language: 'en' }),
    });
  });

  it('updates recording metadata by encoded id', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse(recordingFixture({ id: 'rec/with space', title: 'Customer interview' })),
    );

    const result = await updateRecording(
      'wsp/default',
      'rec/with space',
      { title: 'Customer interview' },
      { fetch: fetchMock },
    );

    expect(result.title).toBe('Customer interview');
    expect(fetchMock).toHaveBeenCalledWith('/workspaces/wsp%2Fdefault/recordings/rec%2Fwith%20space', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title: 'Customer interview' }),
    });
  });
});

describe('recording reads', () => {
  it('lists workspace recordings with an optional limit', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse({ recordings: [recordingFixture({ id: 'rec-list' })] }),
    );

    const result = await listRecordings('wsp_default', { limit: 10 }, { fetch: fetchMock });

    expect(result.recordings).toHaveLength(1);
    expect(fetchMock).toHaveBeenCalledWith('/workspaces/wsp_default/recordings?limit=10', { method: 'GET' });
  });

  it('lists deleted workspace recordings with an optional limit', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse({ recordings: [recordingFixture({ id: 'rec-deleted', deleted_at: '2026-06-07T01:00:00Z' })] }),
    );

    const result = await listDeletedRecordings('wsp_default', { limit: 5 }, { fetch: fetchMock });

    expect(result.recordings).toHaveLength(1);
    expect(fetchMock).toHaveBeenCalledWith('/workspaces/wsp_default/recordings/trash?limit=5', { method: 'GET' });
  });

  it('URL-encodes workspace and recording ids for read requests', async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(recordingFixture({ id: 'rec/with space' })))
      .mockResolvedValueOnce(jsonResponse({ id: 'rec/with space', workspace_id: 'wsp/default', status: 'completed' }))
      .mockResolvedValueOnce(jsonResponse({
        recording: recordingFixture({ id: 'rec/with space', workspace_id: 'wsp/default' }),
        transcript: null,
        segments: [],
        summary: null,
      }));

    await getRecording('wsp/default', 'rec/with space', { fetch: fetchMock });
    await getRecordingStatus('wsp/default', 'rec/with space', { fetch: fetchMock });
    await getRecordingDetails('wsp/default', 'rec/with space', { fetch: fetchMock });

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/workspaces/wsp%2Fdefault/recordings/rec%2Fwith%20space', { method: 'GET' });
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/workspaces/wsp%2Fdefault/recordings/rec%2Fwith%20space/status', { method: 'GET' });
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/workspaces/wsp%2Fdefault/recordings/rec%2Fwith%20space/details', { method: 'GET' });
  });
});

describe('recording retry', () => {
  it('posts to the workspace retry endpoint', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse({
        recording: recordingFixture({ id: 'rec-failed', status: 'uploaded' }),
        processing_enqueued: true,
      }),
    );

    const result = await retryRecording('wsp_default', 'rec-failed', { fetch: fetchMock });

    expect(result.processing_enqueued).toBe(true);
    expect(fetchMock).toHaveBeenCalledWith('/workspaces/wsp_default/recordings/rec-failed/retry', {
      method: 'POST',
    });
  });
});

describe('recording delete', () => {
  it('deletes a workspace recording by encoded id', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(new Response(null, { status: 204 }));

    await deleteRecording('wsp/default', 'rec/with space', { fetch: fetchMock });

    expect(fetchMock).toHaveBeenCalledWith('/workspaces/wsp%2Fdefault/recordings/rec%2Fwith%20space', {
      method: 'DELETE',
    });
  });
});

describe('recording restore', () => {
  it('posts to the workspace restore endpoint with encoded ids', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse(recordingFixture({ id: 'rec/with space', workspace_id: 'wsp/default' })),
    );

    const result = await restoreRecording('wsp/default', 'rec/with space', { fetch: fetchMock });

    expect(result.id).toBe('rec/with space');
    expect(fetchMock).toHaveBeenCalledWith('/workspaces/wsp%2Fdefault/recordings/rec%2Fwith%20space/restore', {
      method: 'POST',
    });
  });
});

describe('recording purge', () => {
  it('deletes to the workspace purge endpoint with encoded ids', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(new Response(null, { status: 204 }));

    await purgeRecording('wsp/default', 'rec/with space', { fetch: fetchMock });

    expect(fetchMock).toHaveBeenCalledWith('/workspaces/wsp%2Fdefault/recordings/rec%2Fwith%20space/purge', {
      method: 'DELETE',
    });
  });
});

describe('API errors', () => {
  it('turns JSON error responses into typed errors', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse({ message: 'recording not found' }, { status: 404, statusText: 'Not Found' }),
    );

    const errorPromise = getRecordingStatus('wsp_default', 'missing', { fetch: fetchMock });

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
  workspace_id: string;
  title: string;
  status: string;
  workflow_type: string;
  language: string;
  failure_reason?: string;
  completed_at?: string;
  failed_at?: string;
  deleted_at?: string;
  deleted_by_user_id?: string;
  created_at: string;
  updated_at: string;
};

function recordingFixture(overrides: Partial<RecordingFixture> = {}): RecordingFixture {
  return {
    id: 'rec-1',
    workspace_id: 'wsp_default',
    title: 'Recording',
    status: 'uploaded',
    workflow_type: 'meeting',
    language: 'zh',
    created_at: '2026-06-07T00:00:00Z',
    updated_at: '2026-06-07T00:00:00Z',
    ...overrides,
  };
}
