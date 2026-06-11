import { type Recording, type RecordingStatus } from '@soniq/api-client';

import { recordingStatusBadgeVariant } from '@/api/queries';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { cn } from '@/lib/utils';

export type RecordingListProps = {
  recordings: Recording[];
  selectedRecordingId: string | null;
  onSelectRecording: (recordingId: string) => void;
  isLoading: boolean;
  error: string | null;
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

export function RecordingList({
  recordings,
  selectedRecordingId,
  onSelectRecording,
  isLoading,
  error,
}: RecordingListProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Recordings</CardTitle>
        <CardDescription>{recordings.length} in current workspace</CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading && <p className="text-muted-foreground text-sm">Loading recordings...</p>}
        {error !== null && <p className="text-destructive text-sm" role="alert">{error}</p>}

        {!isLoading && error === null && recordings.length === 0 && (
          <p className="text-muted-foreground text-sm">No recordings yet.</p>
        )}

        {recordings.length > 0 && (
          <ol className="space-y-2" aria-label="Recording history">
            {recordings.map((recording) => (
              <li key={recording.id}>
                <button
                  type="button"
                  className={cn(
                    'hover:bg-accent focus-visible:ring-ring flex w-full flex-col gap-2 rounded-md border px-3 py-3 text-left text-sm transition-colors focus-visible:ring-2 focus-visible:outline-none',
                    selectedRecordingId === recording.id && 'border-primary bg-accent',
                  )}
                  aria-pressed={selectedRecordingId === recording.id}
                  onClick={() => onSelectRecording(recording.id)}
                >
                  <div className="flex items-start justify-between gap-3">
                    <span className="line-clamp-2 font-medium">{recording.title}</span>
                    <Badge variant={recordingStatusBadgeVariant(recording.status)}>
                      {statusLabels[recording.status]}
                    </Badge>
                  </div>
                  <div className="text-muted-foreground flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                    <span className="font-mono">{recording.id}</span>
                    <time dateTime={recording.created_at}>{formatDateTime(recording.created_at)}</time>
                  </div>
                </button>
              </li>
            ))}
          </ol>
        )}
      </CardContent>
    </Card>
  );
}

function formatDateTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}
