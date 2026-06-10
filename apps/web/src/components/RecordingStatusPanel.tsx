import { type RecordingStatus } from '@soniq/api-client';

import { isTerminalRecordingStatus, recordingStatusBadgeVariant } from '@/api/queries';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';

export type RecordingStatusPanelProps = {
  recordingId: string | null;
  initialStatus?: RecordingStatus;
  currentStatus?: RecordingStatus;
  isPending?: boolean;
  isFetching?: boolean;
  error?: string | null;
  processingEnqueued?: boolean;
};

const statusLabels: Record<RecordingStatus, string> = {
  uploaded: 'Uploaded',
  processing: 'Processing',
  transcribing: 'Transcribing',
  summarizing: 'Summarizing',
  completed: 'Completed',
  failed: 'Failed',
  cancelled: 'Cancelled',
};

export function RecordingStatusPanel({
  recordingId,
  initialStatus = 'uploaded',
  currentStatus,
  isPending = false,
  isFetching = false,
  error = null,
  processingEnqueued,
}: RecordingStatusPanelProps) {
  if (recordingId === null) {
    return null;
  }

  const status = currentStatus ?? initialStatus;
  const isTerminal = isTerminalRecordingStatus(status);

  return (
    <Card aria-label="Recording processing status">
      <CardHeader>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="space-y-1.5">
            <CardTitle>Processing status</CardTitle>
            <CardDescription>
              Recording <span className="font-mono text-foreground">{recordingId}</span>
            </CardDescription>
          </div>
          <Badge variant={recordingStatusBadgeVariant(status)}>{statusLabels[status]}</Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-3 text-sm" aria-live="polite">
        {processingEnqueued !== undefined && (
          <div className="flex items-center justify-between gap-4 rounded-md border px-3 py-2">
            <span className="text-muted-foreground">Processing enqueued</span>
            <Badge variant={processingEnqueued ? 'secondary' : 'destructive'}>
              {processingEnqueued ? 'yes' : 'no'}
            </Badge>
          </div>
        )}

        <div className="flex items-center justify-between gap-4 rounded-md border px-3 py-2">
          <span className="text-muted-foreground">Current step</span>
          <span className="font-medium">{statusLabels[status]}</span>
        </div>

        {isPending && (
          <p className="text-muted-foreground">Checking status...</p>
        )}

        {isFetching && !isPending && !isTerminal && (
          <p className="text-muted-foreground">Refreshing status...</p>
        )}

        {isTerminal && status === 'completed' && (
          <p className="text-muted-foreground">Processing completed.</p>
        )}

        {isTerminal && status !== 'completed' && (
          <p className="text-destructive">Processing ended with {statusLabels[status].toLowerCase()} status.</p>
        )}

        {error !== null && (
          <p className="text-destructive" role="alert">
            {error}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
