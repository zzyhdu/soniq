import { useEffect, useState } from 'react';

import { type AuthUserResponse, type SignInInput, type SignUpInput, type UploadRecordingResponse } from '@soniq/api-client';
import { useQueryClient } from '@tanstack/react-query';

import {
  isUnauthorizedApiError,
  useMe,
  useRecordingStatus,
  useRecordings,
  useRetryRecording,
  useSignIn,
  useSignOut,
  useSignUp,
  useUploadRecording,
  useWorkspaces,
} from '@/api/queries';
import { RecordingList } from '@/components/RecordingList';
import { RecordingResults } from '@/components/RecordingResults';
import { RecordingStatusPanel } from '@/components/RecordingStatusPanel';
import { RecordingUploadForm } from '@/components/RecordingUploadForm';
import { AuthGate } from '@/components/AuthGate';
import { UserMenu } from '@/components/UserMenu';
import { WorkspaceSwitcher } from '@/components/WorkspaceSwitcher';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';

export function App() {
  const initialRoute = parseAppRoute();
  const queryClient = useQueryClient();
  const [authState, setAuthState] = useState<AuthState>('checking');
  const shouldResolveCurrentUser = authState !== 'signed_out';
  const meQuery = useMe(shouldResolveCurrentUser);
  const isAuthenticated = authState === 'authenticated';
  const signInMutation = useSignIn();
  const signUpMutation = useSignUp();
  const signOutMutation = useSignOut();
  const workspacesQuery = useWorkspaces(isAuthenticated && meQuery.data !== undefined);
  const workspaces = workspacesQuery.data?.workspaces ?? [];
  const [selectedWorkspaceId, setSelectedWorkspaceId] = useState<string | null>(initialRoute.workspaceId);
  const [selectedRecordingId, setSelectedRecordingId] = useState<string | null>(initialRoute.recordingId);
  const [latestProcessingRequest, setLatestProcessingRequest] = useState<LatestProcessingRequest | null>(null);

  useEffect(() => {
    if (!shouldResolveCurrentUser) {
      return;
    }
    if (meQuery.data !== undefined) {
      setAuthState('authenticated');
      return;
    }
    if (isUnauthorizedApiError(meQuery.error)) {
      setAuthState('signed_out');
    }
  }, [meQuery.data, meQuery.error, shouldResolveCurrentUser]);

  useEffect(() => {
    function syncRoute() {
      const route = parseAppRoute();
      setSelectedWorkspaceId(route.workspaceId);
      setSelectedRecordingId(route.recordingId);
      setLatestProcessingRequest(null);
    }

    window.addEventListener('hashchange', syncRoute);
    window.addEventListener('popstate', syncRoute);
    return () => {
      window.removeEventListener('hashchange', syncRoute);
      window.removeEventListener('popstate', syncRoute);
    };
  }, []);

  useEffect(() => {
    if (workspaces.length === 0) {
      return;
    }
    if (selectedWorkspaceId !== null && workspaces.some((workspace) => workspace.id === selectedWorkspaceId)) {
      return;
    }
    const fallbackWorkspaceId = workspaces[0].id;
    setSelectedWorkspaceId(fallbackWorkspaceId);
    setSelectedRecordingId(null);
    setLatestProcessingRequest(null);
    replaceAppRoute({ workspaceId: fallbackWorkspaceId, recordingId: null });
  }, [selectedWorkspaceId, workspaces]);

  const recordingsQuery = useRecordings(selectedWorkspaceId, isAuthenticated);
  const recordings = recordingsQuery.data?.recordings ?? [];
  const uploadRecordingMutation = useUploadRecording(selectedWorkspaceId);
  const retryRecordingMutation = useRetryRecording(selectedWorkspaceId, selectedRecordingId);
  const selectedRecording = recordings.find((recording) => recording.id === selectedRecordingId) ??
    (latestProcessingRequest?.recording.id === selectedRecordingId ? latestProcessingRequest.recording : undefined);
  const statusQuery = useRecordingStatus(selectedWorkspaceId, selectedRecordingId, isAuthenticated);
  const currentStatus = statusQuery.data?.status ?? selectedRecording?.status;
  const currentFailureReason = statusQuery.data?.failure_reason ?? selectedRecording?.failure_reason ?? null;
  const statusError = statusQuery.error instanceof Error ? statusQuery.error.message : null;
  const uploadError = uploadRecordingMutation.error instanceof Error ? uploadRecordingMutation.error.message : null;
  const retryError = retryRecordingMutation.error instanceof Error ? retryRecordingMutation.error.message : null;
  const meError = meQuery.error instanceof Error ? meQuery.error.message : null;
  const workspacesError = workspacesQuery.error instanceof Error ? workspacesQuery.error.message : null;
  const recordingsError = recordingsQuery.error instanceof Error ? recordingsQuery.error.message : null;
  const selectedProcessingEnqueued = latestProcessingRequest?.recording.id === selectedRecordingId
    ? latestProcessingRequest.processing_enqueued
    : undefined;

  function resetSessionState() {
    setSelectedWorkspaceId(null);
    setSelectedRecordingId(null);
    setLatestProcessingRequest(null);
    replaceAppRoute({ workspaceId: null, recordingId: null });
  }

  function handleAuthenticated(response: AuthUserResponse) {
    queryClient.clear();
    queryClient.setQueryData(['me'], response.user);
    resetSessionState();
    setAuthState('authenticated');
  }

  async function handleSignIn(input: SignInInput) {
    const response = await signInMutation.mutateAsync(input);
    handleAuthenticated(response);
  }

  async function handleSignUp(input: SignUpInput) {
    const response = await signUpMutation.mutateAsync(input);
    handleAuthenticated(response);
  }

  function handleSelectWorkspace(workspaceId: string) {
    setSelectedWorkspaceId(workspaceId);
    setSelectedRecordingId(null);
    setLatestProcessingRequest(null);
    pushAppRoute({ workspaceId, recordingId: null });
  }

  function handleSelectRecording(recordingId: string) {
    setSelectedRecordingId(recordingId);
    setLatestProcessingRequest(null);
    if (selectedWorkspaceId !== null) {
      pushAppRoute({ workspaceId: selectedWorkspaceId, recordingId });
    }
  }

  function handleUploaded(response: UploadRecordingResponse) {
    setLatestProcessingRequest({ kind: 'upload', ...response });
    setSelectedWorkspaceId(response.recording.workspace_id);
    setSelectedRecordingId(response.recording.id);
    pushAppRoute({ workspaceId: response.recording.workspace_id, recordingId: response.recording.id });
  }

  async function handleRetryRecording() {
    const response = await retryRecordingMutation.mutateAsync();
    setLatestProcessingRequest({ kind: 'retry', ...response });
    setSelectedWorkspaceId(response.recording.workspace_id);
    setSelectedRecordingId(response.recording.id);
    pushAppRoute({ workspaceId: response.recording.workspace_id, recordingId: response.recording.id });
  }

  async function handleLogout() {
    await signOutMutation.mutateAsync();
    queryClient.clear();
    resetSessionState();
    setAuthState('signed_out');
  }

  if (authState === 'signed_out') {
    return (
      <AuthGate
        isSubmitting={signInMutation.isPending || signUpMutation.isPending}
        error={authErrorMessage(signInMutation.error ?? signUpMutation.error)}
        onSignIn={handleSignIn}
        onSignUp={handleSignUp}
      />
    );
  }

  return (
    <main className="min-h-screen bg-muted/30 px-4 py-6 sm:px-6 lg:px-8">
      <div className="mx-auto flex max-w-7xl flex-col gap-6">
        <header className="flex flex-col gap-4 border-b pb-5 sm:flex-row sm:items-center sm:justify-between">
          <div className="space-y-2">
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-3xl font-semibold tracking-tight">Soniq</h1>
              <Badge variant="secondary">Local</Badge>
            </div>
            <p className="text-muted-foreground text-sm">Audio intelligence workspace</p>
          </div>
          <UserMenu
            user={meQuery.data}
            isLoading={meQuery.isPending}
            error={meError}
            onLogout={handleLogout}
            isLoggingOut={signOutMutation.isPending}
          />
        </header>

        <div className="grid gap-6 lg:grid-cols-[320px_minmax(0,1fr)]">
          <aside className="space-y-6">
            <WorkspaceSwitcher
              workspaces={workspaces}
              selectedWorkspaceId={selectedWorkspaceId}
              onSelectWorkspace={handleSelectWorkspace}
              isLoading={workspacesQuery.isPending}
              error={workspacesError}
            />
            <RecordingList
              recordings={recordings}
              selectedRecordingId={selectedRecordingId}
              onSelectRecording={handleSelectRecording}
              isLoading={selectedWorkspaceId !== null && recordingsQuery.isPending}
              error={recordingsError}
            />
          </aside>

          <section className="space-y-6">
            {selectedWorkspaceId !== null ? (
              <RecordingUploadForm
                onUpload={(input) => uploadRecordingMutation.mutateAsync(input)}
                onUploaded={handleUploaded}
                isUploading={uploadRecordingMutation.isPending}
                error={uploadError}
              />
            ) : (
              <Card>
                <CardHeader>
                  <CardTitle>Upload recording</CardTitle>
                  <CardDescription>No workspace selected.</CardDescription>
                </CardHeader>
              </Card>
            )}

            {latestProcessingRequest !== null && latestProcessingRequest.recording.id === selectedRecordingId && (
              <Card>
                <CardHeader>
                  <CardTitle>{latestProcessingRequest.kind === 'upload' ? 'Upload created' : 'Retry requested'}</CardTitle>
                </CardHeader>
                <CardContent className="space-y-3 text-sm">
                  <div>
                    <span className="text-muted-foreground">Recording ID: </span>
                    <span className="font-mono">{latestProcessingRequest.recording.id}</span>
                  </div>
                  <div>
                    <span className="text-muted-foreground">Processing enqueued: </span>
                    <Badge variant={latestProcessingRequest.processing_enqueued ? 'default' : 'destructive'}>
                      {latestProcessingRequest.processing_enqueued ? 'yes' : 'no'}
                    </Badge>
                  </div>
                </CardContent>
              </Card>
            )}

            <RecordingStatusPanel
              recordingId={selectedRecordingId}
              initialStatus={selectedRecording?.status}
              currentStatus={currentStatus}
              isPending={statusQuery.isPending}
              isFetching={statusQuery.isFetching}
              error={statusError}
              processingEnqueued={selectedProcessingEnqueued}
              failureReason={currentFailureReason}
              onRetry={currentStatus === 'failed' ? handleRetryRecording : undefined}
              isRetrying={retryRecordingMutation.isPending}
              retryError={retryError}
            />

            <RecordingResults
              workspaceId={selectedWorkspaceId}
              recordingId={selectedRecordingId}
              enabled={currentStatus === 'completed'}
            />
          </section>
        </div>
      </div>
    </main>
  );
}

type AuthState = 'checking' | 'authenticated' | 'signed_out';

type AppRoute = {
  workspaceId: string | null;
  recordingId: string | null;
};

type LatestProcessingRequest = UploadRecordingResponse & {
  kind: 'upload' | 'retry';
};

function parseAppRoute(): AppRoute {
  const hash = window.location.hash.replace(/^#/, '');
  const parts = hash.split('/').filter(Boolean);
  if (parts[0] !== 'workspaces' || parts[1] === undefined) {
    return { workspaceId: null, recordingId: null };
  }
  const workspaceId = decodeURIComponent(parts[1]);
  if (parts[2] === 'recordings' && parts[3] !== undefined) {
    return { workspaceId, recordingId: decodeURIComponent(parts[3]) };
  }
  return { workspaceId, recordingId: null };
}

function pushAppRoute(route: AppRoute) {
  writeAppRoute(route, false);
}

function replaceAppRoute(route: AppRoute) {
  writeAppRoute(route, true);
}

function writeAppRoute(route: AppRoute, replace: boolean) {
  const hash = route.workspaceId === null
    ? ''
    : route.recordingId !== null
    ? `#/workspaces/${encodeURIComponent(route.workspaceId ?? '')}/recordings/${encodeURIComponent(route.recordingId)}`
    : `#/workspaces/${encodeURIComponent(route.workspaceId ?? '')}`;
  const url = `${window.location.pathname}${window.location.search}${hash}`;
  if (replace) {
    window.history.replaceState(null, '', url);
    return;
  }
  window.history.pushState(null, '', url);
}

function authErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : null;
}
