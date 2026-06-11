import '@testing-library/jest-dom/vitest';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { RecordingResults } from './RecordingResults';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function renderResults(props: React.ComponentProps<typeof RecordingResults>) {
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
      <RecordingResults {...props} />
    </QueryClientProvider>,
  );
}

describe('RecordingResults', () => {
  it('does not request details before completion', () => {
    const fetchDetails = vi.spyOn(globalThis, 'fetch').mockResolvedValue(detailsResponse(recordingDetails()));

    renderResults({ recordingId: 'rec-1', enabled: false });

    expect(fetchDetails).not.toHaveBeenCalled();
    expect(screen.queryByLabelText(/recording results/i)).not.toBeInTheDocument();
  });

  it('renders summary when present', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(detailsResponse(recordingDetails()));

    renderResults({ recordingId: 'rec-1', enabled: true });

    expect(await screen.findByText('Weekly sync summary')).toBeInTheDocument();
    expect(screen.getByText('The meeting covered launch status.')).toBeInTheDocument();
    expect(screen.getAllByText(/Action item: finish the dashboard/i)).toHaveLength(2);
  });

  it('renders transcript segments in segment index order', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(detailsResponse(recordingDetails({
      segments: [
        segment({ id: 'seg-2', segment_index: 2, text: 'Second segment' }),
        segment({ id: 'seg-1', segment_index: 1, text: 'First segment' }),
      ],
    })));

    renderResults({ recordingId: 'rec-1', enabled: true });

    const renderedSegments = await screen.findAllByTestId('transcript-segment');
    expect(within(renderedSegments[0]).getByText('First segment')).toBeInTheDocument();
    expect(within(renderedSegments[1]).getByText('Second segment')).toBeInTheDocument();
  });

  it('renders readable empty states for missing transcript and summary', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(detailsResponse(recordingDetails({
      transcript: null,
      segments: [],
      summary: null,
    })));

    renderResults({ recordingId: 'rec-empty', enabled: true });

    expect(await screen.findByText('No summary available yet.')).toBeInTheDocument();
    expect(screen.getByText('No transcript available yet.')).toBeInTheDocument();
  });
});

function detailsResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    statusText: 'OK',
    headers: { 'Content-Type': 'application/json' },
  });
}

function recordingDetails(overrides: Record<string, unknown> = {}) {
  return {
    recording: {
      id: 'rec-1',
      workspace_id: 'wsp_default',
      title: 'Weekly sync',
      status: 'completed',
      workflow_type: 'meeting',
      language: 'en',
      created_at: '2026-06-10T00:00:00Z',
      updated_at: '2026-06-10T00:01:00Z',
    },
    transcript: {
      recording_id: 'rec-1',
      provider: 'dashscope_asr',
      model: 'paraformer-v2',
      language: 'en',
      text: 'Full transcript text.',
      transcribed_at: '2026-06-10T00:01:00Z',
    },
    segments: [
      segment({ id: 'seg-0', segment_index: 0, text: 'Welcome everyone.' }),
      segment({ id: 'seg-1', segment_index: 1, text: 'Action item: finish the dashboard.' }),
    ],
    summary: {
      recording_id: 'rec-1',
      provider: 'openai_compatible',
      model: 'qwen3.7-plus',
      type: 'meeting',
      title: 'Weekly sync summary',
      overview: 'The meeting covered launch status.',
      content_markdown: 'Action item: finish the dashboard.',
      summarized_at: '2026-06-10T00:02:00Z',
    },
    ...overrides,
  };
}

function segment(overrides: Partial<Record<string, string | number>> = {}) {
  return {
    id: 'seg',
    recording_id: 'rec-1',
    segment_index: 0,
    start_ms: 0,
    end_ms: 1200,
    speaker_label: 'Speaker 1',
    text: 'Transcript segment.',
    confidence: 0.95,
    ...overrides,
  };
}
