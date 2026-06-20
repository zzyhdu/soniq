import { useState } from 'react';
import {
  type Recording,
  type RecordingDetails,
  type RecordingMindMapNode,
  type RecordingSummary,
  type RecordingTranscriptSegment,
} from '@soniq/api-client';
import { Copy, Download } from 'lucide-react';

import { useRecordingDetails } from '@/api/queries';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { cn } from '@/lib/utils';

export type RecordingResultsProps = {
  workspaceId: string | null;
  recordingId: string | null;
  enabled: boolean;
  activeTab?: RecordingResultsTab;
};

export function RecordingResults({ workspaceId, recordingId, enabled, activeTab }: RecordingResultsProps) {
  const detailsQuery = useRecordingDetails(workspaceId, recordingId, enabled);

  if (workspaceId === null || recordingId === null || !enabled) {
    return null;
  }

  if (detailsQuery.isPending) {
    return (
      <Card aria-label="Recording results">
        <CardHeader>
          <CardTitle>Results</CardTitle>
          <CardDescription>Loading transcript, summary, and mind map.</CardDescription>
        </CardHeader>
      </Card>
    );
  }

  const error = detailsQuery.error instanceof Error ? detailsQuery.error.message : null;

  if (error !== null) {
    return (
      <Card aria-label="Recording results">
        <CardHeader>
          <CardTitle>Results</CardTitle>
          <CardDescription>Transcript and summary could not be loaded.</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-destructive text-sm" role="alert">{error}</p>
        </CardContent>
      </Card>
    );
  }

  if (detailsQuery.data === undefined) {
    return null;
  }

  return <RecordingResultsView details={detailsQuery.data} controlledActiveTab={activeTab} />;
}

function RecordingResultsView({
  details,
  controlledActiveTab,
}: {
  details: RecordingDetails;
  controlledActiveTab?: RecordingResultsTab;
}) {
  const [internalActiveTab, setInternalActiveTab] = useState<RecordingResultsTab>('summary');
  const activeTab = controlledActiveTab ?? internalActiveTab;
  const shouldRenderInternalTabs = controlledActiveTab === undefined;
  const segments = [...details.segments].sort((a, b) => a.segment_index - b.segment_index);

  return (
    <section aria-label="Recording results">
      {shouldRenderInternalTabs && (
        <div className="mb-4 flex min-w-0 overflow-x-auto border-b border-[#c5c6cd]" role="tablist" aria-label="Recording result views">
          {resultTabs.map((tab) => (
            <button
              key={tab.value}
              type="button"
              role="tab"
              aria-selected={activeTab === tab.value}
              className={cn(
                'shrink-0 border-b-2 px-3 pb-2 font-mono text-[12px] font-medium leading-4 tracking-[0.02em] transition-colors focus-visible:ring-2 focus-visible:ring-[#3b82f6] focus-visible:outline-none',
                activeTab === tab.value ? 'border-[#091426] text-[#091426]' : 'border-transparent text-[#45474c] hover:text-[#191c1e]',
              )}
              onClick={() => setInternalActiveTab(tab.value)}
            >
              {tab.label}
            </button>
          ))}
        </div>
      )}

      <div>
        {activeTab === 'summary' && <SummaryTab details={details} />}
        {activeTab === 'mind-map' && <MindMapTab details={details} />}
        {activeTab === 'transcript' && <TranscriptTab details={details} segments={segments} />}
        {activeTab === 'metadata' && <MetadataTab details={details} segments={segments} />}
      </div>
    </section>
  );
}

export type RecordingResultsTab = 'summary' | 'mind-map' | 'transcript' | 'metadata';

const resultTabs: Array<{ value: RecordingResultsTab; label: string }> = [
  { value: 'summary', label: 'Summary' },
  { value: 'transcript', label: 'Transcript' },
  { value: 'mind-map', label: 'Mind Map' },
  { value: 'metadata', label: 'Metadata' },
];

function SummaryTab({ details }: { details: RecordingDetails }) {
  const summary = details.summary;

  if (summary === null || summary === undefined) {
    return <p className="text-muted-foreground text-sm">No summary available yet.</p>;
  }

  const summaryText = summaryMarkdown(details.recording, summary);

  return (
    <div className="mx-auto max-w-3xl rounded border border-[#c5c6cd] bg-white shadow-sm">
      <div className="flex items-center justify-end border-b border-[#c5c6cd] bg-[#f7f9fb] p-2">
        <ResultActions
          copyLabel="Copy summary"
          exportLabel="Export Markdown"
          filename={`${fileSafeName(details.recording.title)}-summary.md`}
          content={summaryText}
          compact
        />
      </div>
      <div className="flex flex-col gap-6 p-4 text-[14px] leading-6 text-[#191c1e] md:p-8">
        <section>
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <h3 className="text-[20px] font-semibold leading-7 text-[#091426]">{summary.title}</h3>
            <Badge variant="secondary" className="rounded bg-[#eceef0] font-mono text-[11px] text-[#45474c]">
              {summary.provider}
            </Badge>
          </div>
          <p className="text-[#45474c]">{summary.overview}</p>
        </section>
        <hr className="border-[#c5c6cd]" />
        <section>
          <h3 className="mb-3 text-[20px] font-semibold leading-7 text-[#091426]">Generated Notes</h3>
          <p className="whitespace-pre-wrap text-[#45474c]">{summary.content_markdown}</p>
        </section>
      </div>
    </div>
  );
}

function MindMapTab({ details }: { details: RecordingDetails }) {
  const mindMap = details.mind_map;

  if (mindMap === null || mindMap === undefined) {
    return <p className="text-muted-foreground text-sm">No mind map available yet.</p>;
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-lg font-semibold leading-snug">{mindMap.title}</h3>
            <Badge variant="secondary">{mindMap.provider}</Badge>
          </div>
          <p className="text-muted-foreground text-sm">{mindMap.model}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <ResultActions
            copyLabel="Copy mind map"
            exportLabel="Export Markdown"
            filename={`${fileSafeName(details.recording.title)}-mind-map.md`}
            content={mindMap.content_markdown}
          />
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => downloadTextFile(
              `${fileSafeName(details.recording.title)}-mind-map.json`,
              JSON.stringify(mindMap.root, null, 2),
              'application/json;charset=utf-8',
            )}
          >
            <Download className="size-4" aria-hidden="true" />
            JSON
          </Button>
        </div>
      </div>
      <MindMapTree root={mindMap.root} />
    </div>
  );
}

function TranscriptTab({
  details,
  segments,
}: {
  details: RecordingDetails;
  segments: RecordingTranscriptSegment[];
}) {
  const transcript = details.transcript;
  const transcriptText = transcriptMarkdown(details.recording, segments, transcript?.text ?? '');

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-lg font-semibold leading-snug">Transcript</h3>
            {transcript !== null && transcript !== undefined && <Badge variant="secondary">{transcript.provider}</Badge>}
          </div>
          <p className="text-muted-foreground text-sm">
            {segments.length > 0 ? `${segments.length} segments` : 'No segments'}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <ResultActions
            copyLabel="Copy transcript"
            exportLabel="Export Markdown"
            filename={`${fileSafeName(details.recording.title)}-transcript.md`}
            content={transcriptText}
            disabled={transcriptText.length === 0}
          />
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={transcriptText.length === 0}
            onClick={() => downloadTextFile(
              `${fileSafeName(details.recording.title)}-transcript.txt`,
              plainTranscript(segments, transcript?.text ?? ''),
            )}
          >
            <Download className="size-4" aria-hidden="true" />
            TXT
          </Button>
        </div>
      </div>

      {segments.length > 0 ? (
        <ol className="space-y-3">
          {segments.map((segment) => (
            <li key={segment.id} className="rounded-md border bg-background px-3 py-3" data-testid="transcript-segment">
              <div className="mb-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                {segment.speaker_label.length > 0 && (
                  <Badge variant="outline">{segment.speaker_label}</Badge>
                )}
                <span>{formatTimeRange(segment)}</span>
              </div>
              <p className="text-sm leading-6">{segment.text}</p>
            </li>
          ))}
        </ol>
      ) : transcript !== null && transcript !== undefined && transcript.text.length > 0 ? (
        <div className="rounded-md border bg-muted/20 p-4">
          <p className="whitespace-pre-wrap text-sm leading-6">{transcript.text}</p>
        </div>
      ) : (
        <p className="text-muted-foreground text-sm">No transcript available yet.</p>
      )}
    </div>
  );
}

function MetadataTab({
  details,
  segments,
}: {
  details: RecordingDetails;
  segments: RecordingTranscriptSegment[];
}) {
  const recording = details.recording;
  const rows: Array<[string, string | number | undefined]> = [
    ['Recording ID', recording.id],
    ['Workspace ID', recording.workspace_id],
    ['Status', recording.status],
    ['Workflow type', recording.workflow_type],
    ['Language', recording.language],
    ['Audio content type', recording.audio_content_type],
    ['Audio size', formatBytes(recording.audio_size_bytes)],
    ['Transcript provider', details.transcript?.provider],
    ['Transcript model', details.transcript?.model],
    ['Summary provider', details.summary?.provider],
    ['Summary model', details.summary?.model],
    ['Mind map provider', details.mind_map?.provider],
    ['Mind map model', details.mind_map?.model],
    ['Segments', segments.length],
    ['Created', formatDateTime(recording.created_at)],
    ['Updated', formatDateTime(recording.updated_at)],
    ['Completed', recording.completed_at ? formatDateTime(recording.completed_at) : undefined],
  ];

  return (
    <dl className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
      {rows.map(([label, value]) => (
        <div key={label} className="rounded-md border bg-muted/20 px-3 py-2">
          <dt className="text-xs text-muted-foreground">{label}</dt>
          <dd className="mt-1 min-w-0 break-words font-mono text-xs">{value ?? 'n/a'}</dd>
        </div>
      ))}
    </dl>
  );
}

function ResultActions({
  copyLabel,
  exportLabel,
  filename,
  content,
  disabled = false,
  compact = false,
}: {
  copyLabel: string;
  exportLabel: string;
  filename: string;
  content: string;
  disabled?: boolean;
  compact?: boolean;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        type="button"
        variant="outline"
        size="sm"
        className={cn(
          'rounded border-[#c5c6cd] bg-white text-[#191c1e] shadow-none hover:bg-[#f2f4f6]',
          compact && 'h-8 border-transparent px-2 text-[13px] font-normal',
        )}
        disabled={disabled || content.length === 0}
        onClick={() => void copyText(content)}
      >
        <Copy className="size-4" aria-hidden="true" />
        {copyLabel}
      </Button>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className={cn(
          'rounded border-[#c5c6cd] bg-white text-[#191c1e] shadow-none hover:bg-[#f2f4f6]',
          compact && 'h-8 px-2 text-[13px] font-normal',
        )}
        disabled={disabled || content.length === 0}
        onClick={() => downloadTextFile(filename, content)}
      >
        <Download className="size-4" aria-hidden="true" />
        {exportLabel}
      </Button>
    </div>
  );
}

function MindMapTree({ root }: { root: RecordingMindMapNode }) {
  return (
    <div className="overflow-x-auto rounded-md border bg-muted/20 p-4">
      <MindMapNodeView node={root} depth={0} />
    </div>
  );
}

function MindMapNodeView({ node, depth }: { node: RecordingMindMapNode; depth: number }) {
  const children = node.children ?? [];

  if (children.length > 0) {
    return (
      <details className={depth === 0 ? 'min-w-[260px]' : 'border-l pl-4'} open={depth < 2} data-testid="mind-map-node">
        <summary className="flex cursor-pointer list-none items-start gap-2 py-1.5">
          <span className="mt-2 h-2 w-2 shrink-0 rounded-full bg-sky-600" aria-hidden="true" />
          <span className={depth === 0 ? 'text-base font-semibold leading-6' : 'text-sm leading-6'}>
            {node.label}
          </span>
        </summary>
        <div className="ml-1.5 space-y-1">
          {children.map((child, index) => (
            <MindMapNodeView key={`${depth}-${index}-${child.label}`} node={child} depth={depth + 1} />
          ))}
        </div>
      </details>
    );
  }

  return (
    <div className={depth === 0 ? 'min-w-[260px]' : 'border-l pl-4'} data-testid="mind-map-node">
      <div className="flex items-start gap-2 py-1.5">
        <span className="mt-2 h-2 w-2 shrink-0 rounded-full bg-emerald-600" aria-hidden="true" />
        <span className={depth === 0 ? 'text-base font-semibold leading-6' : 'text-sm leading-6'}>
          {node.label}
        </span>
      </div>
    </div>
  );
}

async function copyText(value: string) {
  if (navigator.clipboard?.writeText === undefined) {
    return;
  }

  await navigator.clipboard.writeText(value);
}

function downloadTextFile(filename: string, content: string, contentType = 'text/plain;charset=utf-8') {
  const blob = new Blob([content], { type: contentType });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(url);
}

function transcriptMarkdown(recording: Recording, segments: RecordingTranscriptSegment[], fallbackText: string) {
  if (segments.length === 0) {
    return fallbackText;
  }

  return [
    `# ${recording.title}`,
    '',
    ...segments.map((segment) => `- [${formatTimeRange(segment)}] ${segment.speaker_label}: ${segment.text}`),
  ].join('\n');
}

function summaryMarkdown(recording: Recording, summary: RecordingSummary) {
  return [
    `# ${summary.title || recording.title}`,
    summary.overview,
    summary.content_markdown,
  ].filter((section) => section.length > 0).join('\n\n');
}

function plainTranscript(segments: RecordingTranscriptSegment[], fallbackText: string) {
  if (segments.length === 0) {
    return fallbackText;
  }

  return segments.map((segment) => `${formatTimeRange(segment)} ${segment.speaker_label}: ${segment.text}`).join('\n');
}

function fileSafeName(value: string) {
  const safeName = value.trim().toLocaleLowerCase().replace(/[^a-z0-9._-]+/g, '-').replace(/^-+|-+$/g, '');
  return safeName.length > 0 ? safeName : 'recording';
}

function formatBytes(value: number | undefined) {
  if (value === undefined) {
    return undefined;
  }

  if (value < 1024) {
    return `${value} B`;
  }

  if (value < 1024 * 1024) {
    return `${(value / 1024).toFixed(1)} KB`;
  }

  return `${(value / (1024 * 1024)).toFixed(1)} MB`;
}

function formatDateTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function formatTimeRange(segment: RecordingTranscriptSegment) {
  return `${formatMilliseconds(segment.start_ms)} - ${formatMilliseconds(segment.end_ms)}`;
}

function formatMilliseconds(milliseconds: number) {
  const safeMilliseconds = Math.max(0, milliseconds);
  const totalSeconds = Math.floor(safeMilliseconds / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;

  return `${minutes}:${seconds.toString().padStart(2, '0')}`;
}
