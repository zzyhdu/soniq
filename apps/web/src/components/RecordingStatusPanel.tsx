import { type RecordingStatus } from '@soniq/api-client';

import { isTerminalRecordingStatus, recordingStatusBadgeVariant } from '@/api/queries';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';

export type RecordingStatusPanelProps = {
  recordingId: string | null;
  initialStatus?: RecordingStatus;
  currentStatus?: RecordingStatus;
  isPending?: boolean;
  isFetching?: boolean;
  error?: string | null;
  processingEnqueued?: boolean;
  failureReason?: string | null;
  onRetry?: () => void;
  isRetrying?: boolean;
  retryError?: string | null;
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
  failureReason = null,
  onRetry,
  isRetrying = false,
  retryError = null,
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

        {isTerminal && status === 'failed' && (
          <div className="space-y-3 rounded-md border border-destructive/40 bg-destructive/5 px-3 py-3">
            <div className="space-y-1">
              <p className="font-medium text-destructive">Processing failed.</p>
              {failureReason !== null && failureReason.trim().length > 0 && (
                <p className="text-sm text-destructive" role="alert">{failureReason}</p>
              )}
            </div>
            {onRetry !== undefined && (
              <Button type="button" variant="outline" size="sm" onClick={onRetry} disabled={isRetrying}>
                {isRetrying ? 'Retrying...' : 'Retry'}
              </Button>
            )}
          </div>
        )}

        {isTerminal && status === 'cancelled' && (
          <p className="text-destructive">Processing ended with cancelled status.</p>
        )}

        {error !== null && (
          <p className="text-destructive" role="alert">
            {error}
          </p>
        )}

        {retryError !== null && (
          <p className="text-destructive" role="alert">
            {retryError}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
