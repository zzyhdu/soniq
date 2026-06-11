import '@testing-library/jest-dom/vitest';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { App } from './App';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function renderApp() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
      mutations: {
        retry: false,
      },
    },
  });

  render(
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>,
  );
}

describe('App workspace recording flow', () => {
  it('loads user, workspaces, recording history, and selected recording details', async () => {
    mockAppFetch();
    const user = userEvent.setup();

    renderApp();

    expect(await screen.findByText('Local Developer')).toBeInTheDocument();
    expect(await screen.findByRole('button', { name: /Weekly sync/i })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /Weekly sync/i }));

    expect(await screen.findByText('Weekly sync summary')).toBeInTheDocument();
    expect(screen.getByText('Full transcript text.')).toBeInTheDocument();
  });

  it('uploads into the selected workspace and refreshes recording history', async () => {
    const requests = mockAppFetch();
    const user = userEvent.setup();

    renderApp();

    await screen.findByRole('button', { name: /upload recording/i });
    await user.upload(screen.getByLabelText(/audio file/i), new File(['audio'], 'meeting.wav', { type: 'audio/wav' }));
    await user.click(screen.getByRole('button', { name: /upload recording/i }));

    await waitFor(() => {
      expect(requests.some((request) => (
        request.method === 'POST' &&
        request.url === '/workspaces/wsp_default/recordings/upload'
      ))).toBe(true);
    });
    await waitFor(() => {
      expect(requests.filter((request) => request.url === '/workspaces/wsp_default/recordings')).toHaveLength(2);
    });
  });

  it('loads recording history for the selected workspace', async () => {
    const requests = mockAppFetch({ includeTeamWorkspace: true });
    const user = userEvent.setup();

    renderApp();

    await screen.findByRole('button', { name: /Weekly sync/i });
    await user.selectOptions(screen.getByLabelText(/current workspace/i), 'wsp_team');

    expect(await screen.findByRole('button', { name: /Team review/i })).toBeInTheDocument();
    expect(requests.some((request) => request.url === '/workspaces/wsp_team/recordings')).toBe(true);
  });
});

type CapturedRequest = {
  url: string;
  method: string;
};

function mockAppFetch(options: { includeTeamWorkspace?: boolean } = {}) {
  const requests: CapturedRequest[] = [];

  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url = requestUrl(input);
    const method = init?.method ?? 'GET';
    requests.push({ url, method });

    if (url === '/me') {
      return jsonResponse({
        id: 'usr_dev',
        email: 'dev@local.soniq',
        display_name: 'Local Developer',
        created_at: '2026-06-11T00:00:00Z',
        updated_at: '2026-06-11T00:00:00Z',
      });
    }

    if (url === '/workspaces') {
      return jsonResponse({
        workspaces: [
          {
            id: 'wsp_default',
            name: 'Default Workspace',
            role: 'owner',
            created_at: '2026-06-11T00:00:00Z',
            updated_at: '2026-06-11T00:00:00Z',
          },
          ...(options.includeTeamWorkspace ? [{
            id: 'wsp_team',
            name: 'Team Workspace',
            role: 'member',
            created_at: '2026-06-11T00:00:00Z',
            updated_at: '2026-06-11T00:00:00Z',
          }] : []),
        ],
      });
    }

    if (url === '/workspaces/wsp_default/recordings' && method === 'GET') {
      return jsonResponse({
        recordings: [
          recordingFixture({ id: 'rec_done', title: 'Weekly sync', status: 'completed' }),
        ],
      });
    }

    if (url === '/workspaces/wsp_team/recordings' && method === 'GET') {
      return jsonResponse({
        recordings: [
          recordingFixture({ id: 'rec_team', workspace_id: 'wsp_team', title: 'Team review', status: 'uploaded' }),
        ],
      });
    }

    if (url === '/workspaces/wsp_default/recordings/upload' && method === 'POST') {
      return jsonResponse({
        recording: recordingFixture({ id: 'rec_uploaded', title: 'Recording', status: 'uploaded' }),
        processing_enqueued: true,
      }, { status: 201 });
    }

    if (url === '/workspaces/wsp_default/recordings/rec_done/status') {
      return jsonResponse({ id: 'rec_done', workspace_id: 'wsp_default', status: 'completed' });
    }

    if (url === '/workspaces/wsp_default/recordings/rec_uploaded/status') {
      return jsonResponse({ id: 'rec_uploaded', workspace_id: 'wsp_default', status: 'uploaded' });
    }

    if (url === '/workspaces/wsp_default/recordings/rec_done/details') {
      return jsonResponse(recordingDetails());
    }

    return jsonResponse('not found', { status: 404, statusText: 'Not Found' });
  });

  return requests;
}

function requestUrl(input: RequestInfo | URL): string {
  if (typeof input === 'string') {
    return input;
  }
  if (input instanceof URL) {
    return input.toString();
  }
  return input.url;
}

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    statusText: 'OK',
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
}

function recordingFixture(overrides: Partial<Record<string, string>> = {}) {
  return {
    id: 'rec-1',
    workspace_id: 'wsp_default',
    title: 'Recording',
    status: 'uploaded',
    workflow_type: 'meeting',
    language: 'en',
    created_at: '2026-06-11T00:00:00Z',
    updated_at: '2026-06-11T00:00:00Z',
    ...overrides,
  };
}

function recordingDetails() {
  return {
    recording: recordingFixture({ id: 'rec_done', title: 'Weekly sync', status: 'completed' }),
    transcript: {
      recording_id: 'rec_done',
      provider: 'fake',
      model: 'fake-transcriber',
      language: 'en',
      text: 'Full transcript text.',
      transcribed_at: '2026-06-11T00:01:00Z',
    },
    segments: [],
    summary: {
      recording_id: 'rec_done',
      provider: 'fake',
      model: 'fake-summary',
      type: 'meeting',
      title: 'Weekly sync summary',
      overview: 'Launch status was covered.',
      content_markdown: 'Action item: finish the dashboard.',
      summarized_at: '2026-06-11T00:02:00Z',
    },
  };
}
