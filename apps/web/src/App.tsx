import { useEffect, useState } from 'react';

import { type UploadRecordingResponse } from '@soniq/api-client';

import {
  useMe,
  useRecordingStatus,
  useRecordings,
  useUploadRecording,
  useWorkspaces,
} from '@/api/queries';
import { RecordingList } from '@/components/RecordingList';
import { RecordingResults } from '@/components/RecordingResults';
import { RecordingStatusPanel } from '@/components/RecordingStatusPanel';
import { RecordingUploadForm } from '@/components/RecordingUploadForm';
import { UserMenu } from '@/components/UserMenu';
import { WorkspaceSwitcher } from '@/components/WorkspaceSwitcher';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';

export function App() {
  const meQuery = useMe();
  const workspacesQuery = useWorkspaces();
  const workspaces = workspacesQuery.data?.workspaces ?? [];
  const [selectedWorkspaceId, setSelectedWorkspaceId] = useState<string | null>(null);
  const [selectedRecordingId, setSelectedRecordingId] = useState<string | null>(null);
  const [latestUpload, setLatestUpload] = useState<UploadRecordingResponse | null>(null);

  useEffect(() => {
    if (workspaces.length === 0) {
      return;
    }
    if (selectedWorkspaceId !== null && workspaces.some((workspace) => workspace.id === selectedWorkspaceId)) {
      return;
    }
    setSelectedWorkspaceId(workspaces[0].id);
    setSelectedRecordingId(null);
    setLatestUpload(null);
  }, [selectedWorkspaceId, workspaces]);

  const recordingsQuery = useRecordings(selectedWorkspaceId);
  const recordings = recordingsQuery.data?.recordings ?? [];
  const uploadRecordingMutation = useUploadRecording(selectedWorkspaceId);
  const selectedRecording = recordings.find((recording) => recording.id === selectedRecordingId) ??
    (latestUpload?.recording.id === selectedRecordingId ? latestUpload.recording : undefined);
  const statusQuery = useRecordingStatus(selectedWorkspaceId, selectedRecordingId);
  const currentStatus = statusQuery.data?.status ?? selectedRecording?.status;
  const statusError = statusQuery.error instanceof Error ? statusQuery.error.message : null;
  const uploadError = uploadRecordingMutation.error instanceof Error ? uploadRecordingMutation.error.message : null;
  const meError = meQuery.error instanceof Error ? meQuery.error.message : null;
  const workspacesError = workspacesQuery.error instanceof Error ? workspacesQuery.error.message : null;
  const recordingsError = recordingsQuery.error instanceof Error ? recordingsQuery.error.message : null;
  const selectedProcessingEnqueued = latestUpload?.recording.id === selectedRecordingId
    ? latestUpload.processing_enqueued
    : undefined;

  function handleSelectWorkspace(workspaceId: string) {
    setSelectedWorkspaceId(workspaceId);
    setSelectedRecordingId(null);
    setLatestUpload(null);
  }

  function handleUploaded(response: UploadRecordingResponse) {
    setLatestUpload(response);
    setSelectedRecordingId(response.recording.id);
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
              onSelectRecording={setSelectedRecordingId}
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

            {latestUpload !== null && latestUpload.recording.id === selectedRecordingId && (
              <Card>
                <CardHeader>
                  <CardTitle>Upload created</CardTitle>
                </CardHeader>
                <CardContent className="space-y-3 text-sm">
                  <div>
                    <span className="text-muted-foreground">Recording ID: </span>
                    <span className="font-mono">{latestUpload.recording.id}</span>
                  </div>
                  <div>
                    <span className="text-muted-foreground">Processing enqueued: </span>
                    <Badge variant={latestUpload.processing_enqueued ? 'default' : 'destructive'}>
                      {latestUpload.processing_enqueued ? 'yes' : 'no'}
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
