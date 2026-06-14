import { useCallback, useEffect, useState } from 'react';

import {
  setUnauthorizedHandler,
  type AuthUserResponse,
  type ListRecordingsResponse,
  type Recording,
  type RecordingStatus,
  type RecordingStatusResponse,
  type SignInInput,
  type SignUpInput,
  type UploadRecordingResponse,
  type WorkflowType,
} from '@soniq/api-client';
import { useQueryClient } from '@tanstack/react-query';
import {
  ArrowLeft,
  BarChart3,
  Building2,
  Download,
  FolderOpen,
  HelpCircle,
  MessageSquare,
  MoreHorizontal,
  Mic,
  Plus,
  RefreshCw,
  Search,
  Settings,
  Trash2,
  Upload,
  type LucideIcon,
  Workflow,
  X,
} from 'lucide-react';

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
import { AuthGate } from '@/components/AuthGate';
import { RecordingList, type RecordingStatusFilter } from '@/components/RecordingList';
import { RecordingResults, type RecordingResultsTab } from '@/components/RecordingResults';
import { RecordingStatusPanel } from '@/components/RecordingStatusPanel';
import { RecordingUploadForm } from '@/components/RecordingUploadForm';
import { UserMenu } from '@/components/UserMenu';
import { WorkspaceSwitcher } from '@/components/WorkspaceSwitcher';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

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
  const [isUploadOpen, setIsUploadOpen] = useState(false);
  const [recordingSearch, setRecordingSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState<RecordingStatusFilter>('all');
  const [workflowTypeFilter, setWorkflowTypeFilter] = useState<WorkflowType | 'all'>('all');
  const [activeView, setActiveView] = useState<AppView>('recordings');

  const resetSessionState = useCallback(() => {
    setSelectedWorkspaceId(null);
    setSelectedRecordingId(null);
    setLatestProcessingRequest(null);
    setIsUploadOpen(false);
    setRecordingSearch('');
    setStatusFilter('all');
    setWorkflowTypeFilter('all');
    setActiveView('recordings');
    replaceAppRoute({ workspaceId: null, recordingId: null });
  }, []);

  const handleUnauthorized = useCallback(() => {
    queryClient.clear();
    resetSessionState();
    setAuthState('signed_out');
  }, [queryClient, resetSessionState]);

  const handleAuthenticated = useCallback((response: AuthUserResponse) => {
    queryClient.clear();
    queryClient.setQueryData(['me'], response.user);
    resetSessionState();
    setAuthState('authenticated');
  }, [queryClient, resetSessionState]);

  useEffect(() => setUnauthorizedHandler(handleUnauthorized), [handleUnauthorized]);

  useEffect(() => {
    if (!shouldResolveCurrentUser) {
      return;
    }
    if (meQuery.data !== undefined) {
      setAuthState('authenticated');
      return;
    }
    if (isUnauthorizedApiError(meQuery.error)) {
      handleUnauthorized();
    }
  }, [handleUnauthorized, meQuery.data, meQuery.error, shouldResolveCurrentUser]);

  useEffect(() => {
    function syncRoute() {
      const route = parseAppRoute();
      setSelectedWorkspaceId(route.workspaceId);
      setSelectedRecordingId(route.recordingId);
      setLatestProcessingRequest(null);
      setActiveView('recordings');
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

  const selectedWorkspace = workspaces.find((workspace) => workspace.id === selectedWorkspaceId);
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

  useEffect(() => {
    if (selectedWorkspaceId === null || statusQuery.data === undefined) {
      return;
    }

    queryClient.setQueryData<ListRecordingsResponse>(
      ['workspaces', selectedWorkspaceId, 'recordings'],
      (current) => syncRecordingListStatus(current, statusQuery.data, selectedRecording),
    );
  }, [queryClient, selectedRecording, selectedWorkspaceId, statusQuery.data]);

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
    setActiveView('recordings');
    pushAppRoute({ workspaceId, recordingId: null });
  }

  function handleSelectRecording(recordingId: string) {
    setSelectedRecordingId(recordingId);
    setLatestProcessingRequest(null);
    setActiveView('recordings');
    if (selectedWorkspaceId !== null) {
      pushAppRoute({ workspaceId: selectedWorkspaceId, recordingId });
    }
  }

  function handleBackToLibrary() {
    setSelectedRecordingId(null);
    setLatestProcessingRequest(null);
    setActiveView('recordings');
    if (selectedWorkspaceId !== null) {
      pushAppRoute({ workspaceId: selectedWorkspaceId, recordingId: null });
    }
  }

  function handleUploaded(response: UploadRecordingResponse) {
    setLatestProcessingRequest({ kind: 'upload', ...response });
    setSelectedWorkspaceId(response.recording.workspace_id);
    setSelectedRecordingId(response.recording.id);
    setIsUploadOpen(false);
    setActiveView('recordings');
    pushAppRoute({ workspaceId: response.recording.workspace_id, recordingId: response.recording.id });
  }

  async function handleRetryRecording() {
    const response = await retryRecordingMutation.mutateAsync();
    setLatestProcessingRequest({ kind: 'retry', ...response });
    setSelectedWorkspaceId(response.recording.workspace_id);
    setSelectedRecordingId(response.recording.id);
    setActiveView('recordings');
    pushAppRoute({ workspaceId: response.recording.workspace_id, recordingId: response.recording.id });
  }

  function handleSelectView(view: AppView) {
    setActiveView(view);
  }

  async function handleLogout() {
    try {
      await signOutMutation.mutateAsync();
    } catch {
      // Local sign-out should still clear browser state even if server revoke fails.
    }
    handleUnauthorized();
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

  if (authState === 'checking' && meQuery.isPending) {
    return (
      <StartupState
        title="Checking session"
        description="Connecting to the local Soniq API."
      />
    );
  }

  if (authState === 'checking' && meQuery.error !== null && !isUnauthorizedApiError(meQuery.error)) {
    return (
      <StartupState
        title="API unavailable"
        description="The Web UI is running, but the local API is not reachable. Start the API on localhost:8080 and refresh this page."
        error={authErrorMessage(meQuery.error)}
      />
    );
  }

  return (
    <main className="h-screen overflow-hidden bg-[#f7f9fb] text-[#191c1e]">
      <header className="fixed top-0 left-0 z-50 flex h-16 w-full items-center justify-between border-b border-[#c5c6cd] bg-[#f7f9fb] px-5 md:px-8">
        <div className="flex items-center gap-4">
          <h1 className="text-[20px] font-bold leading-7 text-[#091426]">Soniq</h1>
        </div>

        <div className="flex min-w-0 flex-1 items-center justify-end gap-4">
          <div className="relative hidden w-64 md:block lg:w-80">
            <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-[#45474c]" aria-hidden="true" />
            <input
              type="search"
              value={recordingSearch}
              onChange={(event) => setRecordingSearch(event.target.value)}
              placeholder="Search"
              aria-label="Search recordings"
              className="h-10 w-full rounded border border-[#c5c6cd] bg-white px-9 text-[14px] shadow-none outline-none transition-colors focus:border-transparent focus:ring-2 focus:ring-[#3b82f6]"
            />
          </div>
          <Button
            type="button"
            aria-label="Upload recording"
            className="h-10 rounded bg-[#091426] px-4 font-mono text-[12px] font-medium leading-4 tracking-[0.02em] text-white hover:bg-[#1e293b]"
            onClick={() => setIsUploadOpen(true)}
            disabled={selectedWorkspaceId === null}
          >
            Upload
          </Button>
          <UserMenu
            user={meQuery.data}
            isLoading={meQuery.isPending}
            error={meError}
            onLogout={handleLogout}
            isLoggingOut={signOutMutation.isPending}
          />
        </div>
      </header>

      <SideNav
        workspaces={workspaces}
        selectedWorkspaceId={selectedWorkspaceId}
        onSelectWorkspace={handleSelectWorkspace}
        workspacesIsLoading={workspacesQuery.isPending}
        workspacesError={workspacesError}
        onUploadClick={() => setIsUploadOpen(true)}
        activeView={activeView}
        onSelectView={handleSelectView}
      />

      <div className="flex h-[calc(100vh-64px)] pt-16 md:pl-[280px]">
        {activeView === 'recordings' ? (
          <>
          <RecordingList
            recordings={recordings}
            selectedRecordingId={selectedRecordingId}
            onSelectRecording={handleSelectRecording}
            isLoading={selectedWorkspaceId !== null && recordingsQuery.isPending}
            error={recordingsError}
            searchQuery={recordingSearch}
            onSearchQueryChange={setRecordingSearch}
            statusFilter={statusFilter}
            onStatusFilterChange={setStatusFilter}
            workflowTypeFilter={workflowTypeFilter}
            onWorkflowTypeFilterChange={setWorkflowTypeFilter}
            onUploadClick={() => setIsUploadOpen(true)}
          />

          <RecordingDetailPanel
            workspaceId={selectedWorkspaceId}
            workspaceName={selectedWorkspace?.name ?? null}
            recording={selectedRecording}
            currentStatus={currentStatus}
            statusIsPending={statusQuery.isPending}
            statusIsFetching={statusQuery.isFetching}
            statusError={statusError}
            processingEnqueued={selectedProcessingEnqueued}
            failureReason={currentFailureReason}
            onRetry={currentStatus === 'failed' ? handleRetryRecording : undefined}
            isRetrying={retryRecordingMutation.isPending}
            retryError={retryError}
            onUploadClick={() => setIsUploadOpen(true)}
            onBack={handleBackToLibrary}
          />
          </>
        ) : (
          <ConstructionWorkspaceView view={activeView} onBackToRecordings={() => setActiveView('recordings')} />
        )}
      </div>

      {isUploadOpen && selectedWorkspaceId !== null && (
        <UploadDrawer
          workspaceName={selectedWorkspace?.name ?? selectedWorkspaceId}
          isUploading={uploadRecordingMutation.isPending}
          error={uploadError}
          onClose={() => setIsUploadOpen(false)}
          onUpload={(input) => uploadRecordingMutation.mutateAsync(input)}
          onUploaded={handleUploaded}
        />
      )}
    </main>
  );
}

type AuthState = 'checking' | 'authenticated' | 'signed_out';

type AppView = 'recordings' | ConstructionView;

type ConstructionView = 'analytics' | 'workflows' | 'library' | 'settings';

type AppRoute = {
  workspaceId: string | null;
  recordingId: string | null;
};

type LatestProcessingRequest = UploadRecordingResponse & {
  kind: 'upload' | 'retry';
};

function StartupState({
  title,
  description,
  error,
}: {
  title: string;
  description: string;
  error?: string | null;
}) {
  return (
    <main className="flex min-h-screen items-center justify-center bg-[#f7f5ef] px-4 py-8">
      <section className="w-full max-w-md rounded-md border bg-background p-6 shadow-sm" aria-label={title}>
        <div className="space-y-2">
          <div className="flex size-9 items-center justify-center rounded-md bg-primary text-sm font-semibold text-primary-foreground">
            S
          </div>
          <h1 className="text-xl font-semibold">{title}</h1>
          <p className="text-sm text-muted-foreground">{description}</p>
        </div>
        {error !== null && error !== undefined && (
          <p className="mt-4 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive" role="alert">
            {error}
          </p>
        )}
      </section>
    </main>
  );
}

type SideNavProps = {
  workspaces: React.ComponentProps<typeof WorkspaceSwitcher>['workspaces'];
  selectedWorkspaceId: string | null;
  onSelectWorkspace: (workspaceId: string) => void;
  workspacesIsLoading: boolean;
  workspacesError: string | null;
  onUploadClick: () => void;
  activeView: AppView;
  onSelectView: (view: AppView) => void;
};

function SideNav({
  workspaces,
  selectedWorkspaceId,
  onSelectWorkspace,
  workspacesIsLoading,
  workspacesError,
  onUploadClick,
  activeView,
  onSelectView,
}: SideNavProps) {
  return (
    <nav className="fixed top-16 bottom-0 left-0 z-40 hidden w-[280px] flex-col border-r border-[#c5c6cd] bg-[#f2f4f6] p-4 md:flex" aria-label="Workspace navigation">
      <div className="mb-6 flex items-center gap-3 px-2 py-3">
        <div className="flex size-10 shrink-0 items-center justify-center rounded border border-[#c5c6cd] bg-[#e6e8ea]">
          <Building2 className="size-5 text-[#091426]" aria-hidden="true" />
        </div>
        <WorkspaceSwitcher
          workspaces={workspaces}
          selectedWorkspaceId={selectedWorkspaceId}
          onSelectWorkspace={onSelectWorkspace}
          isLoading={workspacesIsLoading}
          error={workspacesError}
        />
      </div>

      <Button
        type="button"
        className="mb-4 h-[42px] w-full rounded bg-[#091426] font-mono text-[12px] font-medium leading-4 tracking-[0.02em] text-white hover:bg-[#1e293b]"
        onClick={onUploadClick}
      >
        <Plus className="size-4" aria-hidden="true" />
        New Recording
      </Button>

      <div className="flex flex-1 flex-col gap-1">
        <SideNavItem icon={Mic} label="Recordings" active={activeView === 'recordings'} onClick={() => onSelectView('recordings')} />
        <SideNavItem icon={BarChart3} label="Analytics" active={activeView === 'analytics'} onClick={() => onSelectView('analytics')} />
        <SideNavItem icon={Workflow} label="Workflows" active={activeView === 'workflows'} onClick={() => onSelectView('workflows')} />
        <SideNavItem icon={FolderOpen} label="Library" active={activeView === 'library'} onClick={() => onSelectView('library')} />
        <SideNavItem icon={Settings} label="Settings" active={activeView === 'settings'} onClick={() => onSelectView('settings')} />
      </div>

      <div className="mt-auto flex flex-col gap-1 border-t border-[#c5c6cd] pt-4">
        <SideNavItem icon={HelpCircle} label="Help" disabled />
        <SideNavItem icon={MessageSquare} label="Feedback" disabled />
      </div>
    </nav>
  );
}

function SideNavItem({
  icon: Icon,
  label,
  active = false,
  disabled = false,
  onClick,
}: {
  icon: LucideIcon;
  label: string;
  active?: boolean;
  disabled?: boolean;
  onClick?: () => void;
}) {
  return (
    <button
      type="button"
      className={cn(
        'flex w-full items-center gap-3 rounded px-3 py-2 text-left font-mono text-[12px] font-medium leading-4 tracking-[0.02em] transition-colors',
        active ? 'bg-[#dae2fd] text-[#3f465c]' : 'text-[#565e74] hover:bg-[#e6e8ea]',
        disabled && 'cursor-not-allowed opacity-50 hover:bg-transparent',
      )}
      aria-current={active ? 'page' : undefined}
      disabled={disabled}
      onClick={onClick}
    >
      <Icon className="size-5" aria-hidden="true" />
      {label}
    </button>
  );
}

const constructionViewMeta: Record<ConstructionView, { title: string; description: string; icon: LucideIcon }> = {
  analytics: {
    title: 'Analytics',
    description: 'Workspace reporting is planned, but not available in this build yet.',
    icon: BarChart3,
  },
  workflows: {
    title: 'Workflows',
    description: 'Workflow configuration is planned, but not available in this build yet.',
    icon: Workflow,
  },
  library: {
    title: 'Library',
    description: 'Knowledge library features are planned, but not available in this build yet.',
    icon: FolderOpen,
  },
  settings: {
    title: 'Settings',
    description: 'Workspace settings are planned, but not available in this build yet.',
    icon: Settings,
  },
};

function ConstructionWorkspaceView({
  view,
  onBackToRecordings,
}: {
  view: ConstructionView;
  onBackToRecordings: () => void;
}) {
  const meta = constructionViewMeta[view];
  const Icon = meta.icon;

  return (
    <section className="flex h-full min-w-0 flex-1 items-center justify-center bg-[#f7f9fb] p-8" aria-label={`${meta.title} workspace`}>
      <div className="w-full max-w-lg rounded border border-[#c5c6cd] bg-white p-8 text-center shadow-sm">
        <div className="mx-auto mb-5 flex size-12 items-center justify-center rounded border border-[#c5c6cd] bg-[#f2f4f6]">
          <Icon className="size-6 text-[#091426]" aria-hidden="true" />
        </div>
        <p className="mb-2 font-mono text-[12px] font-medium uppercase leading-4 tracking-[0.08em] text-[#45474c]">
          Under construction
        </p>
        <h2 className="text-[24px] font-semibold leading-8 text-[#091426]">{meta.title}</h2>
        <p className="mx-auto mt-3 max-w-sm text-[14px] leading-6 text-[#45474c]">{meta.description}</p>
        <Button
          type="button"
          className="mt-6 h-10 rounded bg-[#091426] px-4 font-mono text-[12px] font-medium leading-4 tracking-[0.02em] text-white hover:bg-[#1e293b]"
          onClick={onBackToRecordings}
        >
          Back to Recordings
        </Button>
      </div>
    </section>
  );
}

type RecordingDetailPanelProps = {
  workspaceId: string | null;
  workspaceName: string | null;
  recording: Recording | undefined;
  currentStatus: RecordingStatus | undefined;
  statusIsPending: boolean;
  statusIsFetching: boolean;
  statusError: string | null;
  processingEnqueued: boolean | undefined;
  failureReason: string | null;
  onRetry?: () => void;
  isRetrying: boolean;
  retryError: string | null;
  onUploadClick: () => void;
  onBack: () => void;
};

function RecordingDetailPanel({
  workspaceId,
  workspaceName,
  recording,
  currentStatus,
  statusIsPending,
  statusIsFetching,
  statusError,
  processingEnqueued,
  failureReason,
  onRetry,
  isRetrying,
  retryError,
  onUploadClick,
  onBack,
}: RecordingDetailPanelProps) {
  const [activeTab, setActiveTab] = useState<RecordingResultsTab>('summary');

  if (workspaceId === null) {
    return (
      <EmptyDetail
        title="No workspace selected"
        description="Choose a workspace before uploading or reviewing recordings."
        onUploadClick={undefined}
      />
    );
  }

  if (recording === undefined) {
    return (
      <EmptyDetail
        title={workspaceName ?? 'Workspace'}
        description="Select a recording from the library or upload a new audio file."
        onUploadClick={onUploadClick}
      />
    );
  }

  const status = currentStatus ?? recording.status;
  const isCompleted = status === 'completed';
  const canRetry = status === 'failed' && onRetry !== undefined;

  return (
    <section className="flex h-full min-w-0 flex-1 flex-col overflow-hidden bg-[#f7f9fb]" aria-label="Recording detail">
      <header className="shrink-0 border-b border-[#c5c6cd] bg-white px-5 py-4 shadow-sm md:px-8">
        <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
          <div className="flex min-w-0 gap-3">
            <Button type="button" variant="ghost" size="icon" className="shrink-0 md:hidden" onClick={onBack} aria-label="Back to recordings">
              <ArrowLeft className="size-4" aria-hidden="true" />
            </Button>
            <div className="min-w-0 space-y-2">
              <div className="flex min-w-0 items-center gap-3">
                <h2 className="min-w-0 truncate text-[24px] font-semibold leading-8 text-[#091426]">{recording.title}</h2>
                <StatusTag status={status} />
              </div>
              <div className="flex flex-wrap items-center gap-2 text-[13px] leading-[18px] text-[#45474c]">
                <span>{formatDateTime(recording.updated_at)}</span>
                <span className="text-[#75777d]" aria-hidden="true">•</span>
                <span>{recording.language || 'unknown language'}</span>
                <span className="text-[#75777d]" aria-hidden="true">•</span>
                <span>Workspace: {workspaceName ?? recording.workspace_id}</span>
              </div>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Button type="button" variant="outline" size="sm" className="h-10 rounded border-[#c5c6cd] bg-white px-3 text-[13px] font-normal text-[#191c1e] shadow-none hover:bg-[#f2f4f6]" disabled={!isCompleted} title="Available from result tabs">
              <Download className="size-4" aria-hidden="true" />
              Export
            </Button>
            <Button type="button" variant="outline" size="sm" className="h-10 rounded border-[#c5c6cd] bg-white px-3 text-[13px] font-normal text-[#191c1e] shadow-none hover:bg-[#f2f4f6]" disabled title="Regenerate API is planned">
              <RefreshCw className="size-4" aria-hidden="true" />
              Regenerate
            </Button>
            <div className="mx-1 h-6 w-px bg-[#c5c6cd]" aria-hidden="true" />
            <Button type="button" variant="ghost" size="icon" className="size-8 rounded text-[#45474c] hover:bg-[#f2f4f6]" disabled title="More actions are planned" aria-label="More actions">
              <MoreHorizontal className="size-4" aria-hidden="true" />
            </Button>
            <Button type="button" variant="ghost" size="icon" className="size-8 rounded text-[#ba1a1a] hover:bg-[#ffdad6]" disabled title="Delete API is planned" aria-label="Delete recording">
              <Trash2 className="size-4" aria-hidden="true" />
            </Button>
          </div>
        </div>

        {isCompleted && (
          <div className="mt-6 flex gap-6 border-b border-[#c5c6cd]" role="tablist" aria-label="Recording result views">
            {detailTabs.map((tab) => (
              <button
                key={tab.value}
                type="button"
                role="tab"
                aria-selected={activeTab === tab.value}
                className={cn(
                  'border-b-2 pb-2 font-mono text-[12px] font-medium leading-4 tracking-[0.02em] transition-colors',
                  activeTab === tab.value
                    ? 'border-[#091426] text-[#091426]'
                    : 'border-transparent text-[#45474c] hover:text-[#191c1e]',
                )}
                onClick={() => setActiveTab(tab.value)}
              >
                {tab.label}
              </button>
            ))}
          </div>
        )}
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto bg-[#f7f9fb] p-5 md:p-8">
        {!isCompleted && (
          <RecordingStatusPanel
            recordingId={recording.id}
            initialStatus={recording.status}
            currentStatus={status}
            isPending={statusIsPending}
            isFetching={statusIsFetching}
            error={statusError}
            processingEnqueued={processingEnqueued}
            failureReason={failureReason}
            onRetry={canRetry ? onRetry : undefined}
            isRetrying={isRetrying}
            retryError={retryError}
          />
        )}

        {isCompleted ? (
          <RecordingResults workspaceId={workspaceId} recordingId={recording.id} enabled activeTab={activeTab} />
        ) : (
          <DeferredResultsPlaceholder status={status} />
        )}
      </div>
    </section>
  );
}

const detailTabs: Array<{ value: RecordingResultsTab; label: string }> = [
  { value: 'summary', label: 'Summary' },
  { value: 'transcript', label: 'Transcript' },
  { value: 'mind-map', label: 'Mind Map' },
  { value: 'metadata', label: 'Metadata' },
];

function StatusTag({ status }: { status: RecordingStatus }) {
  const state = statusTagState[status];

  return (
    <span className={cn('flex shrink-0 items-center gap-1 rounded border px-2 py-0.5 font-mono text-[11px] font-medium uppercase leading-[14px] tracking-[0.08em]', state.className)}>
      <span className={cn('size-1.5 rounded-full', state.dotClass)} aria-hidden="true" />
      {state.label}
    </span>
  );
}

const statusTagState: Record<RecordingStatus, { label: string; className: string; dotClass: string }> = {
  uploaded: { label: 'Uploaded', className: 'border-[#c5c6cd] bg-[#eceef0] text-[#45474c]', dotClass: 'bg-[#75777d]' },
  processing: { label: 'Processing', className: 'border-amber-200 bg-amber-50 text-amber-700', dotClass: 'bg-amber-500' },
  transcribing: { label: 'Transcribing', className: 'border-amber-200 bg-amber-50 text-amber-700', dotClass: 'bg-amber-500' },
  summarizing: { label: 'Summarizing', className: 'border-amber-200 bg-amber-50 text-amber-700', dotClass: 'bg-amber-500' },
  completed: { label: 'Completed', className: 'border-emerald-200 bg-emerald-50 text-emerald-700', dotClass: 'bg-emerald-500' },
  failed: { label: 'Failed', className: 'border-[#ffdad6] bg-[#ffdad6] text-[#93000a]', dotClass: 'bg-[#ba1a1a]' },
  cancelled: { label: 'Cancelled', className: 'border-[#c5c6cd] bg-[#eceef0] text-[#45474c]', dotClass: 'bg-[#75777d]' },
};

function EmptyDetail({
  title,
  description,
  onUploadClick,
}: {
  title: string;
  description: string;
  onUploadClick?: () => void;
}) {
  return (
    <section className="flex min-h-[520px] items-center justify-center rounded-md border bg-background p-6 text-center shadow-sm" aria-label="Recording detail">
      <div className="max-w-sm space-y-4">
        <div className="mx-auto flex size-12 items-center justify-center rounded-md bg-[#e9f3ee] text-emerald-700">
          <Upload className="size-5" aria-hidden="true" />
        </div>
        <div className="space-y-1">
          <h2 className="text-lg font-semibold">{title}</h2>
          <p className="text-sm text-muted-foreground">{description}</p>
        </div>
        {onUploadClick !== undefined && (
          <Button type="button" onClick={onUploadClick}>
            <Upload className="size-4" aria-hidden="true" />
            Upload recording
          </Button>
        )}
      </div>
    </section>
  );
}

function DeferredResultsPlaceholder({ status }: { status: RecordingStatus }) {
  if (status === 'failed' || status === 'cancelled') {
    return (
      <div className="rounded-md border bg-[#fff7ed] px-4 py-3 text-sm text-[#9a3412]">
        Results stay unavailable until processing is retried and completed.
      </div>
    );
  }

  return (
    <div className="rounded-md border bg-muted/30 px-4 py-3 text-sm text-muted-foreground">
      Summary, transcript, and mind map tabs will appear after processing completes.
    </div>
  );
}

type UploadDrawerProps = {
  workspaceName: string;
  isUploading: boolean;
  error: string | null;
  onClose: () => void;
  onUpload: React.ComponentProps<typeof RecordingUploadForm>['onUpload'];
  onUploaded: React.ComponentProps<typeof RecordingUploadForm>['onUploaded'];
};

function UploadDrawer({
  workspaceName,
  isUploading,
  error,
  onClose,
  onUpload,
  onUploaded,
}: UploadDrawerProps) {
  return (
    <div className="fixed inset-0 z-50" role="dialog" aria-modal="true" aria-labelledby="upload-drawer-title">
      <button
        type="button"
        className="absolute inset-0 bg-black/30"
        aria-label="Close upload drawer"
        onClick={onClose}
        disabled={isUploading}
      />
      <section className="absolute top-0 right-0 flex h-full w-full max-w-xl flex-col bg-background shadow-xl">
        <header className="flex items-start justify-between gap-4 border-b p-5">
          <div className="min-w-0 space-y-1">
            <h2 id="upload-drawer-title" className="text-lg font-semibold">Upload recording</h2>
            <p className="truncate text-sm text-muted-foreground">{workspaceName}</p>
          </div>
          <Button type="button" variant="ghost" size="icon" onClick={onClose} disabled={isUploading} aria-label="Close upload drawer">
            <X className="size-4" aria-hidden="true" />
          </Button>
        </header>
        <div className="flex-1 overflow-y-auto p-5">
          <RecordingUploadForm
            variant="plain"
            onUpload={onUpload}
            onUploaded={onUploaded}
            isUploading={isUploading}
            error={error}
            onCancel={onClose}
          />
        </div>
      </section>
    </div>
  );
}

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

function syncRecordingListStatus(
  current: ListRecordingsResponse | undefined,
  status: RecordingStatusResponse,
  selectedRecording: Recording | undefined,
): ListRecordingsResponse | undefined {
  if (current === undefined) {
    return current;
  }

  let didChange = false;
  let didFindRecording = false;
  const seenRecordingIds = new Set<string>();
  const recordings: Recording[] = [];

  for (const recording of current.recordings) {
    if (seenRecordingIds.has(recording.id)) {
      didChange = true;
      continue;
    }
    seenRecordingIds.add(recording.id);

    if (recording.id !== status.id) {
      recordings.push(recording);
      continue;
    }

    didFindRecording = true;
    const updatedRecording = mergeRecordingStatus(recording, status);
    didChange ||= updatedRecording !== recording;
    recordings.push(updatedRecording);
  }

  if (!didFindRecording && selectedRecording !== undefined && selectedRecording.id === status.id) {
    recordings.unshift(mergeRecordingStatus(selectedRecording, status));
    didChange = true;
  }

  return didChange ? { ...current, recordings } : current;
}

function mergeRecordingStatus(recording: Recording, status: RecordingStatusResponse): Recording {
  const nextUpdatedAt = status.completed_at ?? status.failed_at ?? recording.updated_at;

  if (
    recording.status === status.status &&
    recording.failure_reason === status.failure_reason &&
    recording.completed_at === status.completed_at &&
    recording.failed_at === status.failed_at &&
    recording.updated_at === nextUpdatedAt
  ) {
    return recording;
  }

  return {
    ...recording,
    status: status.status,
    failure_reason: status.failure_reason,
    completed_at: status.completed_at,
    failed_at: status.failed_at,
    updated_at: nextUpdatedAt,
  };
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
