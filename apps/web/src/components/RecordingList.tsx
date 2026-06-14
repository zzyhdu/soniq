import { type Recording, type RecordingStatus, type WorkflowType } from '@soniq/api-client';
import { Search } from 'lucide-react';

import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';

export type RecordingListProps = {
  recordings: Recording[];
  selectedRecordingId: string | null;
  onSelectRecording: (recordingId: string) => void;
  isLoading: boolean;
  error: string | null;
  searchQuery?: string;
  onSearchQueryChange?: (query: string) => void;
  statusFilter?: RecordingStatusFilter;
  onStatusFilterChange?: (status: RecordingStatusFilter) => void;
  workflowTypeFilter?: WorkflowType | 'all';
  onWorkflowTypeFilterChange?: (workflowType: WorkflowType | 'all') => void;
  onUploadClick?: () => void;
};

export function RecordingList({
  recordings,
  selectedRecordingId,
  onSelectRecording,
  isLoading,
  error,
  searchQuery = '',
  onSearchQueryChange,
  statusFilter = 'all',
  onStatusFilterChange,
  workflowTypeFilter = 'all',
  onUploadClick,
}: RecordingListProps) {
  const filteredRecordings = recordings.filter((recording) => (
    matchesSearch(recording, searchQuery) &&
    matchesStatusFilter(recording.status, statusFilter) &&
    (workflowTypeFilter === 'all' || recording.workflow_type === workflowTypeFilter)
  ));

  return (
    <section className="flex h-full min-h-0 w-full flex-shrink-0 flex-col border-r border-[#c5c6cd] bg-white md:w-[360px]" aria-label="Recording library">
      <div className="shrink-0 space-y-3 border-b border-[#c5c6cd] bg-[#f7f9fb] p-4">
        <h2 className="text-[20px] font-medium leading-7 text-[#091426]">Recordings</h2>

        <div className="relative">
          <Label htmlFor="recording-search" className="sr-only">Search recordings</Label>
          <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-[#45474c]" aria-hidden="true" />
          <Input
            id="recording-search"
            value={searchQuery}
            onChange={(event) => onSearchQueryChange?.(event.target.value)}
            placeholder="Search recordings..."
            className="h-10 rounded border-[#c5c6cd] bg-white pl-9 text-[13px] shadow-none focus-visible:ring-[#3b82f6]"
          />
        </div>

        <div className="flex gap-2 overflow-x-auto pb-1" aria-label="Recording status filters">
          {recordingStatusFilters.map((filter) => (
            <button
              key={filter.value}
              type="button"
              className={cn(
                'shrink-0 rounded-full border px-3 py-1 font-mono text-[11px] font-medium leading-4 tracking-[0.02em] transition-colors',
                statusFilter === filter.value
                  ? 'border-transparent bg-[#091426] text-white'
                  : 'border-[#c5c6cd] bg-[#eceef0] text-[#191c1e] hover:bg-[#e0e3e5]',
              )}
              onClick={() => onStatusFilterChange?.(filter.value)}
            >
              {filter.label}
            </button>
          ))}
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {isLoading && <p className="px-4 py-3 text-[13px] text-[#45474c]">Loading recordings...</p>}
        {error !== null && <p className="px-4 py-3 text-[13px] text-[#ba1a1a]" role="alert">{error}</p>}

        {!isLoading && error === null && recordings.length === 0 && (
          <div className="m-4 rounded border border-dashed border-[#c5c6cd] p-4 text-sm">
            <p className="font-medium">No recordings yet</p>
            <p className="mt-1 text-[#45474c]">Upload an audio file to start processing.</p>
            {onUploadClick !== undefined && <button type="button" className="mt-4 rounded bg-[#091426] px-3 py-2 text-xs font-medium text-white" onClick={onUploadClick}>Upload recording</button>}
          </div>
        )}

        {!isLoading && error === null && recordings.length > 0 && filteredRecordings.length === 0 && (
          <p className="px-4 py-3 text-[13px] text-[#45474c]">No recordings match the current filters.</p>
        )}

        {filteredRecordings.length > 0 && (
          <ol aria-label="Recording history">
            {filteredRecordings.map((recording) => (
              <li key={recording.id}>
                <button
                  type="button"
                  className={cn(
                    'relative flex w-full flex-col gap-1.5 border-b border-[#c5c6cd] bg-white p-4 text-left transition-colors hover:bg-[#f2f4f6] focus-visible:ring-2 focus-visible:ring-[#3b82f6] focus-visible:outline-none',
                    selectedRecordingId === recording.id && 'bg-[#eceef0]',
                  )}
                  aria-pressed={selectedRecordingId === recording.id}
                  onClick={() => onSelectRecording(recording.id)}
                >
                  {selectedRecordingId === recording.id && (
                    <span className="absolute top-0 bottom-0 left-0 w-1 bg-[#3b82f6]" aria-hidden="true" />
                  )}
                  <div className="flex items-start justify-between gap-2">
                    <span className="min-w-0 truncate text-[14px] font-medium leading-5 text-[#091426]">{recording.title}</span>
                    <StatusLabel status={recording.status} />
                  </div>
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="rounded bg-[#e0e3e5] px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wider text-[#45474c]">
                      {workflowTypeLabels[recording.workflow_type]}
                    </span>
                    <span className="text-[13px] leading-[18px] text-[#45474c]">{recording.language || 'unknown'}</span>
                    <span className="text-[#75777d]" aria-hidden="true">•</span>
                    <time className="truncate text-[13px] leading-[18px] text-[#45474c]" dateTime={recording.updated_at}>
                      {formatDateTime(recording.updated_at)}
                    </time>
                  </div>
                  <p className="mt-1 w-full truncate text-[13px] leading-[18px] text-[#45474c]">
                    {recording.failure_reason ?? recording.id}
                  </p>
                </button>
              </li>
            ))}
          </ol>
        )}
      </div>
    </section>
  );
}

export type RecordingStatusFilter = RecordingStatus | 'all' | 'active';

const recordingStatusFilters: Array<{ value: RecordingStatusFilter; label: string }> = [
  { value: 'all', label: 'All' },
  { value: 'active', label: 'Processing' },
  { value: 'completed', label: 'Completed' },
  { value: 'failed', label: 'Failed' },
];

const workflowTypeLabels: Record<WorkflowType, string> = {
  meeting: 'Meeting',
  lecture: 'Lecture',
  interview: 'Interview',
  memo: 'Memo',
};

const activeStatuses: RecordingStatus[] = ['uploaded', 'processing', 'transcribing', 'summarizing'];

function matchesStatusFilter(status: RecordingStatus, filter: RecordingStatusFilter) {
  if (filter === 'all') {
    return true;
  }

  if (filter === 'active') {
    return activeStatuses.includes(status);
  }

  return status === filter;
}

function StatusLabel({ status }: { status: RecordingStatus }) {
  const state = statusState[status];

  return (
    <div className="flex shrink-0 items-center gap-1">
      <span className={cn('size-2 rounded-full', state.dotClass)} aria-hidden="true" />
      <span className="font-mono text-[11px] font-medium leading-[14px] tracking-[0.02em] text-[#45474c]">
        {state.label}
      </span>
    </div>
  );
}

const statusState: Record<RecordingStatus, { label: string; dotClass: string }> = {
  uploaded: { label: 'Uploaded', dotClass: 'bg-[#75777d]' },
  processing: { label: 'Processing', dotClass: 'bg-amber-500' },
  transcribing: { label: 'Transcribing', dotClass: 'bg-amber-500' },
  summarizing: { label: 'Summarizing', dotClass: 'bg-amber-500' },
  completed: { label: 'Completed', dotClass: 'bg-emerald-500' },
  failed: { label: 'Failed', dotClass: 'bg-[#ba1a1a]' },
  cancelled: { label: 'Cancelled', dotClass: 'bg-[#75777d]' },
};

function matchesSearch(recording: Recording, searchQuery: string) {
  const query = searchQuery.trim().toLocaleLowerCase();
  if (query.length === 0) {
    return true;
  }

  return [
    recording.title,
    recording.id,
    recording.workflow_type,
    recording.language,
    recording.status,
  ].some((value) => value.toLocaleLowerCase().includes(query));
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
