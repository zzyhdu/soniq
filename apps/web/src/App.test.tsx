import '@testing-library/jest-dom/vitest';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
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
    expect(await screen.findAllByRole('button', { name: /upload recording/i })).not.toHaveLength(0);
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

  it('shows a local API unavailable state when the session check cannot reach the API', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(jsonResponse('Bad Gateway', {
      status: 502,
      statusText: 'Bad Gateway',
    }));

    renderApp();

    expect(await screen.findByRole('heading', { name: /api unavailable/i })).toBeInTheDocument();
    expect(screen.getByText(/start the API on localhost:8080/i)).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('Bad Gateway');
  });

  it('loads user, workspaces, recording history, and selected recording details', async () => {
    mockAppFetch();
    const user = userEvent.setup();

    renderApp();

    expect(await screen.findByText('Local Developer')).toBeInTheDocument();
    expect(await screen.findByRole('button', { name: /Weekly sync/i })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /Weekly sync/i }));

    expect(await screen.findByText('Weekly sync summary')).toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: /transcript/i }));
    expect(screen.getByText('Full transcript text.')).toBeInTheDocument();
  });

  it('renames the selected recording inline', async () => {
    const requests = mockAppFetch();
    const user = userEvent.setup();

    renderApp();

    await user.click(await screen.findByRole('button', { name: /Weekly sync/i }));
    expect(await screen.findByText('Weekly sync summary')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /edit recording title/i }));
    const titleInput = screen.getByRole('textbox', { name: /^Recording title$/i });
    await user.clear(titleInput);
    await user.type(titleInput, 'Customer interview');
    await user.click(screen.getByRole('button', { name: /save recording title/i }));

    await waitFor(() => {
      expect(requests.some((request) => (
        request.method === 'PATCH' &&
        request.url === '/workspaces/wsp_default/recordings/rec_done' &&
        request.body === JSON.stringify({ title: 'Customer interview' })
      ))).toBe(true);
    });
    expect(await screen.findByRole('heading', { name: /Customer interview/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Customer interview/i })).toBeInTheDocument();
  });

  it('soft deletes the selected recording after confirmation', async () => {
    const requests = mockAppFetch();
    const user = userEvent.setup();

    renderApp();

    await user.click(await screen.findByRole('button', { name: /Weekly sync/i }));
    expect(await screen.findByText('Weekly sync summary')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /delete recording/i }));
    const dialog = await screen.findByRole('dialog', { name: /delete recording/i });
    expect(within(dialog).getByText(/moves/i)).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: /^Delete recording$/i }));

    await waitFor(() => {
      expect(requests.some((request) => (
        request.method === 'DELETE' &&
        request.url === '/workspaces/wsp_default/recordings/rec_done'
      ))).toBe(true);
    });
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /Weekly sync/i })).not.toBeInTheDocument();
    });
    expect(screen.getByText(/Select a recording from the library/i)).toBeInTheDocument();
  });

  it('restores a soft-deleted recording from Trash', async () => {
    const requests = mockAppFetch();
    const user = userEvent.setup();

    renderApp();

    await user.click(await screen.findByRole('button', { name: /Weekly sync/i }));
    await user.click(screen.getByRole('button', { name: /delete recording/i }));
    const dialog = await screen.findByRole('dialog', { name: /delete recording/i });
    await user.click(within(dialog).getByRole('button', { name: /^Delete recording$/i }));
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /Weekly sync/i })).not.toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: /^Trash$/i }));

    expect(await screen.findByRole('heading', { name: /^Trash$/i })).toBeInTheDocument();
    expect(await screen.findByText('Weekly sync')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /^Restore$/i }));

    await waitFor(() => {
      expect(requests.some((request) => (
        request.method === 'POST' &&
        request.url === '/workspaces/wsp_default/recordings/rec_done/restore'
      ))).toBe(true);
    });
    expect(await screen.findByText(/Trash is empty/i)).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /^Recordings$/i }));

    expect(await screen.findByRole('button', { name: /Weekly sync/i })).toBeInTheDocument();
  });

  it('permanently deletes a soft-deleted recording from Trash', async () => {
    const requests = mockAppFetch();
    const user = userEvent.setup();

    renderApp();

    await user.click(await screen.findByRole('button', { name: /Weekly sync/i }));
    await user.click(screen.getByRole('button', { name: /delete recording/i }));
    const deleteDialog = await screen.findByRole('dialog', { name: /delete recording/i });
    await user.click(within(deleteDialog).getByRole('button', { name: /^Delete recording$/i }));
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /Weekly sync/i })).not.toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: /^Trash$/i }));

    expect(await screen.findByText('Weekly sync')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /^Delete forever$/i }));
    const purgeDialog = await screen.findByRole('dialog', { name: /delete forever/i });
    await user.click(within(purgeDialog).getByRole('button', { name: /^Delete forever$/i }));

    await waitFor(() => {
      expect(requests.some((request) => (
        request.method === 'DELETE' &&
        request.url === '/workspaces/wsp_default/recordings/rec_done/purge'
      ))).toBe(true);
    });
    expect(await screen.findByText(/Trash is empty/i)).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /^Recordings$/i }));

    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /Weekly sync/i })).not.toBeInTheDocument();
    });
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
    const user = userEvent.setup();
    window.history.replaceState(null, '', '/#/workspaces/wsp_default/recordings/rec_done');

    renderApp();

    expect(await screen.findByText('Weekly sync summary')).toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: /transcript/i }));
    expect(screen.getByText('Full transcript text.')).toBeInTheDocument();
  });

  it('uploads into the selected workspace and refreshes recording history', async () => {
    const requests = mockAppFetch();
    const user = userEvent.setup();

    renderApp();

    await screen.findByRole('button', { name: /Weekly sync/i });
    const uploadButton = screen
      .getAllByRole('button', { name: /upload recording/i })
      .find((button) => !button.hasAttribute('disabled'));
    expect(uploadButton).toBeDefined();
    await user.click(uploadButton as HTMLButtonElement);
    const dialog = await screen.findByRole('dialog', { name: /upload recording/i });
    await user.upload(within(dialog).getByLabelText(/audio file/i), new File(['audio'], 'meeting.wav', { type: 'audio/wav' }));
    await user.click(within(dialog).getByRole('button', { name: /upload recording/i }));

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

  it('shows construction pages for planned navigation entries', async () => {
    mockAppFetch();
    const user = userEvent.setup();

    renderApp();

    expect(await screen.findByRole('button', { name: /Weekly sync/i })).toBeInTheDocument();

    for (const label of ['Analytics', 'Workflows', 'Library']) {
      await user.click(screen.getByRole('button', { name: new RegExp(`^${label}$`, 'i') }));

      expect(screen.getByRole('heading', { name: label })).toBeInTheDocument();
      expect(screen.getByText('Under construction')).toBeInTheDocument();
    }

    await user.click(screen.getByRole('button', { name: /Back to Recordings/i }));

    expect(await screen.findByRole('button', { name: /Weekly sync/i })).toBeInTheDocument();
  });

  it('updates the recording list status from status polling', async () => {
    mockAppFetch({ includeCompletingRecording: true });
    const user = userEvent.setup();

    renderApp();

    const recordingRow = await screen.findByRole('button', { name: /Recently uploaded/i });
    expect(within(recordingRow).getByText('Uploaded')).toBeInTheDocument();

    await user.click(recordingRow);

    await waitFor(() => {
      expect(within(recordingRow).getByText('Completed')).toBeInTheDocument();
    });
    expect(screen.getAllByRole('button', { name: /Recently uploaded/i })).toHaveLength(1);
  });

  it('shows failed recording reasons and retries failed recordings', async () => {
    const requests = mockAppFetch({ includeFailedRecording: true });
    const user = userEvent.setup();

    renderApp();

    await user.click(await screen.findByRole('button', { name: /Failed upload/i }));

    expect(await screen.findAllByText(/transcribe audio: provider failed/i)).not.toHaveLength(0);
    await user.click(screen.getByRole('button', { name: /^retry$/i }));

    await waitFor(() => {
      expect(requests.some((request) => (
        request.method === 'POST' &&
        request.url === '/workspaces/wsp_default/recordings/rec_failed/retry'
      ))).toBe(true);
    });
    expect(await screen.findByText('Processing enqueued')).toBeInTheDocument();
    expect(screen.getByText('yes')).toBeInTheDocument();
  });
});

type CapturedRequest = {
  url: string;
  method: string;
  body?: BodyInit | null;
};

function mockAppFetch(options: {
  detailsUnauthorized?: boolean;
  includeTeamWorkspace?: boolean;
  includeFailedRecording?: boolean;
  includeCompletingRecording?: boolean;
  signOutFails?: boolean;
  workspacesUnauthorized?: boolean;
} = {}) {
  const requests: CapturedRequest[] = [];
  let authenticated = true;
  const deletedRecordingIds = new Set<string>();
  const purgedRecordingIds = new Set<string>();
  let recordingTitle = 'Weekly sync';

  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url = requestUrl(input);
    const method = init?.method ?? 'GET';
    requests.push({ url, method, body: init?.body });

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
          ...(options.includeCompletingRecording ? [
            recordingFixture({
              id: 'rec_completing',
              title: 'Recently uploaded',
              status: 'uploaded',
              updated_at: '2026-06-11T00:00:00Z',
            }),
          ] : []),
          recordingFixture({ id: 'rec_done', title: recordingTitle, status: 'completed' }),
        ].filter((recording) => !deletedRecordingIds.has(recording.id) && !purgedRecordingIds.has(recording.id)),
      });
    }

    if (url === '/workspaces/wsp_default/recordings/trash' && method === 'GET') {
      return jsonResponse({
        recordings: [
          recordingFixture({
            id: 'rec_done',
            title: recordingTitle,
            status: 'completed',
            deleted_at: '2026-06-11T00:05:00Z',
            deleted_by_user_id: 'usr_dev',
          }),
        ].filter((recording) => deletedRecordingIds.has(recording.id) && !purgedRecordingIds.has(recording.id)),
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
      if (deletedRecordingIds.has('rec_done') || purgedRecordingIds.has('rec_done')) {
        return jsonResponse(apiError('not_found', 'not found', 404), {
          status: 404,
          statusText: 'Not Found',
        });
      }
      return jsonResponse({ id: 'rec_done', workspace_id: 'wsp_default', status: 'completed' });
    }

    if (url === '/workspaces/wsp_default/recordings/rec_uploaded/status') {
      return jsonResponse({ id: 'rec_uploaded', workspace_id: 'wsp_default', status: 'uploaded' });
    }

    if (url === '/workspaces/wsp_default/recordings/rec_completing/status') {
      return jsonResponse({
        id: 'rec_completing',
        workspace_id: 'wsp_default',
        status: 'completed',
        completed_at: '2026-06-11T00:03:00Z',
      });
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
      if (deletedRecordingIds.has('rec_done') || purgedRecordingIds.has('rec_done')) {
        return jsonResponse(apiError('not_found', 'not found', 404), {
          status: 404,
          statusText: 'Not Found',
        });
      }
      if (options.detailsUnauthorized) {
        return jsonResponse(apiError('unauthenticated', 'resolve current user', 401), {
          status: 401,
          statusText: 'Unauthorized',
        });
      }
      return jsonResponse(recordingDetails({
        recording: recordingFixture({ id: 'rec_done', title: recordingTitle, status: 'completed' }),
      }));
    }

    if (url === '/workspaces/wsp_default/recordings/rec_done' && method === 'PATCH') {
      const body = typeof init?.body === 'string' ? JSON.parse(init.body) as { title?: string } : {};
      recordingTitle = body.title ?? recordingTitle;
      return jsonResponse(recordingFixture({ id: 'rec_done', title: recordingTitle, status: 'completed' }));
    }

    if (url === '/workspaces/wsp_default/recordings/rec_done' && method === 'DELETE') {
      deletedRecordingIds.add('rec_done');
      return new Response(null, { status: 204 });
    }

    if (url === '/workspaces/wsp_default/recordings/rec_done/restore' && method === 'POST') {
      if (!deletedRecordingIds.has('rec_done') || purgedRecordingIds.has('rec_done')) {
        return jsonResponse(apiError('not_found', 'not found', 404), {
          status: 404,
          statusText: 'Not Found',
        });
      }
      deletedRecordingIds.delete('rec_done');
      return jsonResponse(recordingFixture({ id: 'rec_done', title: recordingTitle, status: 'completed' }));
    }

    if (url === '/workspaces/wsp_default/recordings/rec_done/purge' && method === 'DELETE') {
      if (!deletedRecordingIds.has('rec_done') || purgedRecordingIds.has('rec_done')) {
        return jsonResponse(apiError('not_found', 'not found', 404), {
          status: 404,
          statusText: 'Not Found',
        });
      }
      deletedRecordingIds.delete('rec_done');
      purgedRecordingIds.add('rec_done');
      return new Response(null, { status: 204 });
    }

    if (url === '/workspaces/wsp_default/recordings/rec_completing/details') {
      return jsonResponse(recordingDetails({
        recording: recordingFixture({
          id: 'rec_completing',
          title: 'Recently uploaded',
          status: 'completed',
          completed_at: '2026-06-11T00:03:00Z',
        }),
      }));
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

function recordingDetails(overrides: Record<string, unknown> = {}) {
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
    ...overrides,
  };
}
