import {
  deleteRecording,
  getMe,
  getRecordingDetails,
  getRecordingStatus,
  listDeletedRecordings,
  listRecordings,
  listWorkspaces,
  purgeRecording,
  retryRecording,
  restoreRecording,
  signIn,
  signOut,
  signUp,
  SoniqApiError,
  type ListRecordingsResponse,
  type Recording,
  type RecordingStatus,
  type SignInInput,
  type SignUpInput,
  type UploadRecordingInput,
  uploadRecording,
} from '@soniq/api-client';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

export const RECORDING_STATUS_POLL_INTERVAL_MS = 1500;

export function useMe(enabled = true) {
  return useQuery({
    queryKey: ['me'],
    queryFn: () => getMe(),
    enabled,
    retry: false,
  });
}

export function useWorkspaces(enabled = true) {
  return useQuery({
    queryKey: ['workspaces'],
    queryFn: () => listWorkspaces(),
    enabled,
    retry: retryUnlessUnauthorized,
  });
}

export function useSignIn() {
  return useAuthMutation((input: SignInInput) => signIn(input));
}

export function useSignUp() {
  return useAuthMutation((input: SignUpInput) => signUp(input));
}

export function useSignOut() {
  return useMutation({
    mutationFn: () => signOut(),
  });
}

export function useRecordings(workspaceId: string | null | undefined, enabled = true) {
  return useQuery({
    queryKey: ['workspaces', workspaceId, 'recordings'],
    queryFn: () => listRecordings(requireWorkspaceId(workspaceId)),
    enabled: enabled && hasWorkspaceId(workspaceId),
    retry: retryUnlessUnauthorized,
  });
}

export function useDeletedRecordings(workspaceId: string | null | undefined, enabled = true) {
  return useQuery({
    queryKey: ['workspaces', workspaceId, 'recordings', 'trash'],
    queryFn: () => listDeletedRecordings(requireWorkspaceId(workspaceId)),
    enabled: enabled && hasWorkspaceId(workspaceId),
    retry: retryUnlessUnauthorized,
  });
}

export function useUploadRecording(workspaceId: string | null | undefined) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (input: UploadRecordingInput) => uploadRecording(requireWorkspaceId(workspaceId), input),
    onSuccess: async () => {
      if (!hasWorkspaceId(workspaceId)) {
        return;
      }
      await queryClient.invalidateQueries({ queryKey: ['workspaces', workspaceId, 'recordings'] });
    },
  });
}

export function useDeleteRecording(workspaceId: string | null | undefined) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (recordingId: string) => deleteRecording(requireWorkspaceId(workspaceId), requireRecordingId(recordingId)),
    onSuccess: async (_, recordingId) => {
      if (!hasWorkspaceId(workspaceId)) {
        return;
      }
      queryClient.setQueryData<ListRecordingsResponse>(
        ['workspaces', workspaceId, 'recordings'],
        (current) => current === undefined
          ? current
          : {
              ...current,
              recordings: current.recordings.filter((recording) => recording.id !== recordingId),
            },
      );
      queryClient.removeQueries({ queryKey: ['workspaces', workspaceId, 'recordings', recordingId] });
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['workspaces', workspaceId, 'recordings'], exact: true }),
        queryClient.invalidateQueries({ queryKey: ['workspaces', workspaceId, 'recordings', 'trash'], exact: true }),
      ]);
    },
  });
}

export function useRestoreRecording(workspaceId: string | null | undefined) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (recordingId: string) => restoreRecording(requireWorkspaceId(workspaceId), requireRecordingId(recordingId)),
    onSuccess: async (recording) => {
      if (!hasWorkspaceId(workspaceId)) {
        return;
      }
      queryClient.setQueryData<ListRecordingsResponse>(
        ['workspaces', workspaceId, 'recordings', 'trash'],
        (current) => current === undefined
          ? current
          : {
              ...current,
              recordings: current.recordings.filter((item) => item.id !== recording.id),
            },
      );
      queryClient.setQueryData<ListRecordingsResponse>(
        ['workspaces', workspaceId, 'recordings'],
        (current) => addRecordingToList(current, recording),
      );
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['workspaces', workspaceId, 'recordings'], exact: true }),
        queryClient.invalidateQueries({ queryKey: ['workspaces', workspaceId, 'recordings', 'trash'], exact: true }),
      ]);
    },
  });
}

export function usePurgeRecording(workspaceId: string | null | undefined) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (recordingId: string) => purgeRecording(requireWorkspaceId(workspaceId), requireRecordingId(recordingId)),
    onSuccess: async (_, recordingId) => {
      if (!hasWorkspaceId(workspaceId)) {
        return;
      }
      queryClient.setQueryData<ListRecordingsResponse>(
        ['workspaces', workspaceId, 'recordings', 'trash'],
        (current) => current === undefined
          ? current
          : {
              ...current,
              recordings: current.recordings.filter((item) => item.id !== recordingId),
            },
      );
      queryClient.removeQueries({ queryKey: ['workspaces', workspaceId, 'recordings', recordingId] });
      await queryClient.invalidateQueries({ queryKey: ['workspaces', workspaceId, 'recordings', 'trash'], exact: true });
    },
  });
}

export function useRecordingStatus(
  workspaceId: string | null | undefined,
  recordingId: string | null | undefined,
  enabled = true,
) {
  return useQuery({
    queryKey: ['workspaces', workspaceId, 'recordings', recordingId, 'status'],
    queryFn: () => getRecordingStatus(requireWorkspaceId(workspaceId), requireRecordingId(recordingId)),
    enabled: enabled && hasWorkspaceId(workspaceId) && hasRecordingId(recordingId),
    refetchInterval: (query) => recordingStatusRefetchInterval(query.state.data?.status),
    retry: retryUnlessUnauthorized,
  });
}

export function useRetryRecording(workspaceId: string | null | undefined, recordingId: string | null | undefined) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => retryRecording(requireWorkspaceId(workspaceId), requireRecordingId(recordingId)),
    onSuccess: async (response) => {
      if (!hasWorkspaceId(workspaceId) || !hasRecordingId(recordingId)) {
        return;
      }
      queryClient.setQueryData(['workspaces', workspaceId, 'recordings', recordingId, 'status'], {
        id: response.recording.id,
        workspace_id: response.recording.workspace_id,
        status: response.recording.status,
        failure_reason: response.recording.failure_reason,
        completed_at: response.recording.completed_at,
        failed_at: response.recording.failed_at,
      });
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['workspaces', workspaceId, 'recordings'] }),
        queryClient.invalidateQueries({ queryKey: ['workspaces', workspaceId, 'recordings', recordingId, 'status'] }),
        queryClient.invalidateQueries({ queryKey: ['workspaces', workspaceId, 'recordings', recordingId, 'details'] }),
      ]);
    },
  });
}

export function useRecordingDetails(
  workspaceId: string | null | undefined,
  recordingId: string | null | undefined,
  enabled: boolean,
) {
  return useQuery({
    queryKey: ['workspaces', workspaceId, 'recordings', recordingId, 'details'],
    queryFn: () => getRecordingDetails(requireWorkspaceId(workspaceId), requireRecordingId(recordingId)),
    enabled: enabled && hasWorkspaceId(workspaceId) && hasRecordingId(recordingId),
    retry: retryUnlessUnauthorized,
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

export function isUnauthorizedApiError(error: unknown) {
  return error instanceof SoniqApiError && error.code === 'unauthenticated';
}

function useAuthMutation<TInput, TResponse>(mutationFn: (input: TInput) => Promise<TResponse>) {
  return useMutation({
    mutationFn,
  });
}

function retryUnlessUnauthorized(failureCount: number, error: unknown) {
  return !isUnauthorizedApiError(error) && failureCount < 3;
}

function addRecordingToList(
  current: ListRecordingsResponse | undefined,
  recording: Recording,
): ListRecordingsResponse | undefined {
  if (current === undefined) {
    return current;
  }

  return {
    ...current,
    recordings: [
      recording,
      ...current.recordings.filter((item) => item.id !== recording.id),
    ],
  };
}

function hasRecordingId(recordingId: string | null | undefined): recordingId is string {
  return typeof recordingId === 'string' && recordingId.length > 0;
}

function hasWorkspaceId(workspaceId: string | null | undefined): workspaceId is string {
  return typeof workspaceId === 'string' && workspaceId.length > 0;
}

function requireRecordingId(recordingId: string | null | undefined) {
  if (!hasRecordingId(recordingId)) {
    throw new Error('recording id is required');
  }

  return recordingId;
}

function requireWorkspaceId(workspaceId: string | null | undefined) {
  if (!hasWorkspaceId(workspaceId)) {
    throw new Error('workspace id is required');
  }

  return workspaceId;
}
