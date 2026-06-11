import { type RecordingDetails, type RecordingTranscriptSegment } from '@soniq/api-client';

import { useRecordingDetails } from '@/api/queries';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';

export type RecordingResultsProps = {
  workspaceId: string | null;
  recordingId: string | null;
  enabled: boolean;
};

export function RecordingResults({ workspaceId, recordingId, enabled }: RecordingResultsProps) {
  const detailsQuery = useRecordingDetails(workspaceId, recordingId, enabled);

  if (workspaceId === null || recordingId === null || !enabled) {
    return null;
  }

  if (detailsQuery.isPending) {
    return (
      <Card aria-label="Recording results">
        <CardHeader>
          <CardTitle>Results</CardTitle>
          <CardDescription>Loading transcript and summary.</CardDescription>
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

  return <RecordingResultsView details={detailsQuery.data} />;
}

function RecordingResultsView({ details }: { details: RecordingDetails }) {
  const segments = [...details.segments].sort((a, b) => a.segment_index - b.segment_index);

  return (
    <section className="grid gap-6 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]" aria-label="Recording results">
      <Card>
        <CardHeader>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="space-y-1.5">
              <CardTitle>Summary</CardTitle>
              <CardDescription>{details.recording.title}</CardDescription>
            </div>
            {details.summary !== null && details.summary !== undefined && (
              <Badge variant="secondary">{details.summary.provider}</Badge>
            )}
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {details.summary !== null && details.summary !== undefined ? (
            <div className="space-y-4">
              <div className="space-y-1">
                <h3 className="text-lg font-semibold leading-snug">{details.summary.title}</h3>
                <p className="text-muted-foreground text-sm">{details.summary.overview}</p>
              </div>
              <p className="whitespace-pre-wrap text-sm leading-6">{details.summary.content_markdown}</p>
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">No summary available yet.</p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="space-y-1.5">
              <CardTitle>Transcript</CardTitle>
              <CardDescription>
                {segments.length > 0 ? `${segments.length} segments` : 'No segments'}
              </CardDescription>
            </div>
            {details.transcript !== null && details.transcript !== undefined && (
              <Badge variant="secondary">{details.transcript.provider}</Badge>
            )}
          </div>
        </CardHeader>
        <CardContent>
          {segments.length > 0 ? (
            <ol className="space-y-3">
              {segments.map((segment) => (
                <li key={segment.id} className="rounded-md border px-3 py-3" data-testid="transcript-segment">
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
          ) : details.transcript !== null && details.transcript !== undefined && details.transcript.text.length > 0 ? (
            <p className="whitespace-pre-wrap text-sm leading-6">{details.transcript.text}</p>
          ) : (
            <p className="text-muted-foreground text-sm">No transcript available yet.</p>
          )}
        </CardContent>
      </Card>
    </section>
  );
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
