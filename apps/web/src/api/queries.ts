import {
  type RecordingStatus,
  type UploadRecordingInput,
  getRecordingDetails,
  getRecordingStatus,
  uploadRecording,
} from '@soniq/api-client';
import { useMutation, useQuery } from '@tanstack/react-query';

export const RECORDING_STATUS_POLL_INTERVAL_MS = 1500;

export function useUploadRecording() {
  return useMutation({
    mutationFn: (input: UploadRecordingInput) => uploadRecording(input),
  });
}

export function useRecordingStatus(recordingId: string | null | undefined) {
  return useQuery({
    queryKey: ['recordings', recordingId, 'status'],
    queryFn: () => getRecordingStatus(requireRecordingId(recordingId)),
    enabled: hasRecordingId(recordingId),
    refetchInterval: (query) => recordingStatusRefetchInterval(query.state.data?.status),
  });
}

export function useRecordingDetails(recordingId: string | null | undefined, enabled: boolean) {
  return useQuery({
    queryKey: ['recordings', recordingId, 'details'],
    queryFn: () => getRecordingDetails(requireRecordingId(recordingId)),
    enabled: enabled && hasRecordingId(recordingId),
  });
}

export function recordingStatusRefetchInterval(status: RecordingStatus | undefined) {
  if (status !== undefined && isTerminalRecordingStatus(status)) {
    return false;
  }

  return RECORDING_STATUS_POLL_INTERVAL_MS;
}

export function isTerminalRecordingStatus(status: RecordingStatus) {
  return status === 'completed' || status === 'failed' || status === 'cancelled';
}

export function recordingStatusBadgeVariant(status: RecordingStatus): 'default' | 'secondary' | 'outline' | 'destructive' {
  if (status === 'failed' || status === 'cancelled') {
    return 'destructive';
  }

  if (status === 'completed') {
    return 'default';
  }

  if (status === 'uploaded') {
    return 'outline';
  }

  return 'secondary';
}

function hasRecordingId(recordingId: string | null | undefined): recordingId is string {
  return typeof recordingId === 'string' && recordingId.length > 0;
}

function requireRecordingId(recordingId: string | null | undefined) {
  if (!hasRecordingId(recordingId)) {
    throw new Error('recording id is required');
  }

  return recordingId;
}
