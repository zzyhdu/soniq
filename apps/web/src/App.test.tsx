import '@testing-library/jest-dom/vitest';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { App } from './App';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.history.replaceState(null, '', '/');
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
  it('shows login when the session is missing and loads the workspace after login', async () => {
    const requests = mockPasswordAuthFetch();
    const user = userEvent.setup();

    renderApp();

    expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument();
    const submitButton = screen.getByRole('button', { name: /sign in/i });
    await waitFor(() => expect(submitButton).toBeEnabled());
    await user.type(screen.getByLabelText(/email/i), 'owner@local.soniq');
    await user.type(screen.getByLabelText(/password/i), 'correct horse');
    await user.click(submitButton);

    expect(await screen.findByText('Local Developer')).toBeInTheDocument();
    expect(await screen.findByRole('button', { name: /upload recording/i })).toBeInTheDocument();
    expect(requests.some((request) => request.method === 'POST' && request.url === '/auth/signin')).toBe(true);
  });

  it('shows auth errors without leaving the sign-in screen', async () => {
    mockPasswordAuthFetch({ rejectSignIn: true });
    const user = userEvent.setup();

    renderApp();

    const submitButton = await screen.findByRole('button', { name: /sign in/i });
    await user.type(screen.getByLabelText(/email/i), 'owner@local.soniq');
    await user.type(screen.getByLabelText(/password/i), 'wrong horse');
    await user.click(submitButton);

    expect(await screen.findByRole('alert')).toHaveTextContent('invalid email or password');
    expect(screen.getByRole('heading', { name: /sign in/i })).toBeInTheDocument();
  });

  it('signs up and loads the new user workspace', async () => {
    const requests = mockPasswordAuthFetch();
    const user = userEvent.setup();

    renderApp();

    expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /^sign up$/i }));
    expect(await screen.findByRole('heading', { name: /sign up/i })).toBeInTheDocument();
    const submitButton = screen.getByRole('button', { name: /^sign up$/i });
    await user.type(screen.getByLabelText(/email/i), 'owner@local.soniq');
    await user.type(screen.getByLabelText(/display name/i), 'Owner');
    await user.type(screen.getByLabelText(/password/i), 'correct horse');
    await user.click(submitButton);

    expect(await screen.findByText('Local Developer')).toBeInTheDocument();
    expect(requests.some((request) => request.method === 'POST' && request.url === '/auth/signup')).toBe(true);
  });

  it('signs out and returns to the sign-in screen', async () => {
    const requests = mockAppFetch();
    const user = userEvent.setup();

    renderApp();

    await screen.findByText('Local Developer');
    await user.click(screen.getByRole('button', { name: /sign out/i }));

    expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument();
    expect(requests.some((request) => request.method === 'POST' && request.url === '/auth/signout')).toBe(true);
  });

  it('clears browser state when signout revoke fails', async () => {
    const requests = mockAppFetch({ signOutFails: true });
    const user = userEvent.setup();

    renderApp();

    await screen.findByText('Local Developer');
    await user.click(screen.getByRole('button', { name: /sign out/i }));

    expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument();
    expect(requests.some((request) => request.method === 'POST' && request.url === '/auth/signout')).toBe(true);
  });

  it('returns to sign in when a protected query is unauthorized', async () => {
    mockAppFetch({ workspacesUnauthorized: true });

    renderApp();

    expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument();
  });

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

  it('returns to sign in when recording details are unauthorized', async () => {
    mockAppFetch({ detailsUnauthorized: true });
    const user = userEvent.setup();

    renderApp();

    await user.click(await screen.findByRole('button', { name: /Weekly sync/i }));

    expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument();
  });

  it('opens a recording from a bookmarkable hash route', async () => {
    mockAppFetch();
    window.history.replaceState(null, '', '/#/workspaces/wsp_default/recordings/rec_done');

    renderApp();

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

  it('shows failed recording reasons and retries failed recordings', async () => {
    const requests = mockAppFetch({ includeFailedRecording: true });
    const user = userEvent.setup();

    renderApp();

    await user.click(await screen.findByRole('button', { name: /Failed upload/i }));

    expect(await screen.findByText(/transcribe audio: provider failed/i)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /^retry$/i }));

    await waitFor(() => {
      expect(requests.some((request) => (
        request.method === 'POST' &&
        request.url === '/workspaces/wsp_default/recordings/rec_failed/retry'
      ))).toBe(true);
    });
    expect(await screen.findByText('Retry requested')).toBeInTheDocument();
    expect(screen.queryByText('Upload created')).not.toBeInTheDocument();
  });
});

type CapturedRequest = {
  url: string;
  method: string;
};

function mockAppFetch(options: {
  detailsUnauthorized?: boolean;
  includeTeamWorkspace?: boolean;
  includeFailedRecording?: boolean;
  signOutFails?: boolean;
  workspacesUnauthorized?: boolean;
} = {}) {
  const requests: CapturedRequest[] = [];
  let authenticated = true;

  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url = requestUrl(input);
    const method = init?.method ?? 'GET';
    requests.push({ url, method });

    if (url === '/me') {
      if (!authenticated) {
        return jsonResponse(apiError('unauthenticated', 'resolve current user', 401), {
          status: 401,
          statusText: 'Unauthorized',
        });
      }
      return jsonResponse({
        id: 'usr_dev',
        email: 'dev@local.soniq',
        display_name: 'Local Developer',
        created_at: '2026-06-11T00:00:00Z',
        updated_at: '2026-06-11T00:00:00Z',
      });
    }

    if (url === '/auth/signout' && method === 'POST') {
      authenticated = false;
      if (options.signOutFails) {
        return jsonResponse(apiError('internal_error', 'revoke session', 500), {
          status: 500,
          statusText: 'Internal Server Error',
        });
      }
      return new Response(null, { status: 204 });
    }

    if (url === '/workspaces') {
      if (options.workspacesUnauthorized) {
        return jsonResponse(apiError('unauthenticated', 'resolve current user', 401), {
          status: 401,
          statusText: 'Unauthorized',
        });
      }
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
          ...(options.includeFailedRecording ? [
            recordingFixture({
              id: 'rec_failed',
              title: 'Failed upload',
              status: 'failed',
              failure_reason: 'transcribe audio: provider failed',
            }),
          ] : []),
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

    if (url === '/workspaces/wsp_default/recordings/rec_failed/status') {
      return jsonResponse({
        id: 'rec_failed',
        workspace_id: 'wsp_default',
        status: 'failed',
        failure_reason: 'transcribe audio: provider failed',
      });
    }

    if (url === '/workspaces/wsp_default/recordings/rec_failed/retry' && method === 'POST') {
      return jsonResponse({
        recording: recordingFixture({ id: 'rec_failed', title: 'Failed upload', status: 'uploaded' }),
        processing_enqueued: true,
      });
    }

    if (url === '/workspaces/wsp_default/recordings/rec_done/details') {
      if (options.detailsUnauthorized) {
        return jsonResponse(apiError('unauthenticated', 'resolve current user', 401), {
          status: 401,
          statusText: 'Unauthorized',
        });
      }
      return jsonResponse(recordingDetails());
    }

    return jsonResponse('not found', { status: 404, statusText: 'Not Found' });
  });

  return requests;
}

function mockPasswordAuthFetch(options: { rejectSignIn?: boolean } = {}) {
  const requests: CapturedRequest[] = [];
  let authenticated = false;

  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url = requestUrl(input);
    const method = init?.method ?? 'GET';
    requests.push({ url, method });

    if (url === '/me') {
      if (!authenticated) {
        return jsonResponse(apiError('unauthenticated', 'resolve current user', 401), {
          status: 401,
          statusText: 'Unauthorized',
        });
      }
      return jsonResponse(userFixture());
    }

    if (url === '/auth/signin' && method === 'POST') {
      if (options.rejectSignIn) {
        return jsonResponse(apiError('invalid_credentials', 'invalid email or password', 401), {
          status: 401,
          statusText: 'Unauthorized',
        });
      }
      authenticated = true;
      return jsonResponse({ user: userFixture() });
    }

    if (url === '/auth/signup' && method === 'POST') {
      authenticated = true;
      return jsonResponse({ user: userFixture() }, { status: 201 });
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
        ],
      });
    }

    if (url === '/workspaces/wsp_default/recordings' && method === 'GET') {
      return jsonResponse({ recordings: [] });
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

function apiError(code: string, message: string, status: number) {
  return { code, message, status };
}

function userFixture() {
  return {
    id: 'usr_dev',
    email: 'dev@local.soniq',
    display_name: 'Local Developer',
    created_at: '2026-06-11T00:00:00Z',
    updated_at: '2026-06-11T00:00:00Z',
  };
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
