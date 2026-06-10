import '@testing-library/jest-dom/vitest';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  recordingStatusBadgeVariant,
  recordingStatusRefetchInterval,
} from '@/api/queries';

import { RecordingStatusPanel } from './RecordingStatusPanel';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function renderPanel(props: React.ComponentProps<typeof RecordingStatusPanel>) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  });

  render(
    <QueryClientProvider client={queryClient}>
      <RecordingStatusPanel {...props} />
    </QueryClientProvider>,
  );
}

describe('RecordingStatusPanel', () => {
  it('does not request status without a recording id', () => {
    const fetchStatus = vi.spyOn(globalThis, 'fetch').mockResolvedValue(statusResponse('processing'));

    renderPanel({ recordingId: null });

    expect(fetchStatus).not.toHaveBeenCalled();
    expect(screen.queryByText(/processing status/i)).not.toBeInTheDocument();
  });

  it('polls while status is not terminal', () => {
    expect(recordingStatusRefetchInterval(undefined)).toBe(1500);
    expect(recordingStatusRefetchInterval('uploaded')).toBe(1500);
    expect(recordingStatusRefetchInterval('processing')).toBe(1500);
    expect(recordingStatusRefetchInterval('transcribing')).toBe(1500);
    expect(recordingStatusRefetchInterval('summarizing')).toBe(1500);
  });

  it('stops polling for terminal statuses', () => {
    expect(recordingStatusRefetchInterval('completed')).toBe(false);
    expect(recordingStatusRefetchInterval('failed')).toBe(false);
    expect(recordingStatusRefetchInterval('cancelled')).toBe(false);
  });

  it('renders the current status from the API', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(statusResponse('transcribing'));

    renderPanel({ recordingId: 'rec-1', initialStatus: 'uploaded', processingEnqueued: true });

    expect(await screen.findAllByText('Transcribing')).toHaveLength(2);
    expect(screen.getByText('rec-1')).toBeInTheDocument();
    expect(screen.getByText('yes')).toBeInTheDocument();
  });

  it('uses an error badge variant for failed and cancelled statuses', () => {
    expect(recordingStatusBadgeVariant('completed')).toBe('default');
    expect(recordingStatusBadgeVariant('failed')).toBe('destructive');
    expect(recordingStatusBadgeVariant('cancelled')).toBe('destructive');
  });

  it('surfaces status request errors', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ error: 'recording not found' }), {
        status: 404,
        statusText: 'Not Found',
        headers: { 'Content-Type': 'application/json' },
      }),
    );

    renderPanel({ recordingId: 'rec-missing', initialStatus: 'uploaded' });

    expect(await screen.findByRole('alert')).toHaveTextContent('recording not found');
    expect(screen.getByText('rec-missing')).toBeInTheDocument();
  });
});

function statusResponse(status: string) {
  return new Response(JSON.stringify({ id: 'rec-1', status }), {
    status: 200,
    statusText: 'OK',
    headers: { 'Content-Type': 'application/json' },
  });
}
