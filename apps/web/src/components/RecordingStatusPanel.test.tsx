import '@testing-library/jest-dom/vitest';

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
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
  render(<RecordingStatusPanel {...props} />);
}

describe('RecordingStatusPanel', () => {
  it('does not request status without a recording id', () => {
    renderPanel({ recordingId: null });

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

  it('renders the current status', () => {
    renderPanel({
      recordingId: 'rec-1',
      initialStatus: 'uploaded',
      currentStatus: 'transcribing',
      processingEnqueued: true,
    });

    expect(screen.getAllByText('Transcribing')).toHaveLength(3);
    expect(screen.getByText('rec-1')).toBeInTheDocument();
    expect(screen.getByText('yes')).toBeInTheDocument();
  });

  it('uses an error badge variant for failed and cancelled statuses', () => {
    expect(recordingStatusBadgeVariant('completed')).toBe('default');
    expect(recordingStatusBadgeVariant('failed')).toBe('destructive');
    expect(recordingStatusBadgeVariant('cancelled')).toBe('destructive');
  });

  it('surfaces status request errors', () => {
    renderPanel({ recordingId: 'rec-missing', initialStatus: 'uploaded', error: 'recording not found' });

    expect(screen.getByRole('alert')).toHaveTextContent('recording not found');
    expect(screen.getByText('rec-missing')).toBeInTheDocument();
  });

  it('renders failure reasons and retry action for failed recordings', async () => {
    const retry = vi.fn();
    const user = userEvent.setup();

    renderPanel({
      recordingId: 'rec-failed',
      currentStatus: 'failed',
      failureReason: 'transcribe audio: provider failed',
      onRetry: retry,
    });

    expect(screen.getByRole('alert')).toHaveTextContent('transcribe audio: provider failed');
    await user.click(screen.getByRole('button', { name: /^retry$/i }));
    expect(retry).toHaveBeenCalledTimes(1);
  });
});
