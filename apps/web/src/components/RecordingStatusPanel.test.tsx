import '@testing-library/jest-dom/vitest';

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

    expect(screen.getAllByText('Transcribing')).toHaveLength(2);
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
});
