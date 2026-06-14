import { type Workspace } from '@soniq/api-client';

import { Label } from '@/components/ui/label';
import { Select } from '@/components/ui/select';

export type WorkspaceSwitcherProps = {
  workspaces: Workspace[];
  selectedWorkspaceId: string | null;
  onSelectWorkspace: (workspaceId: string) => void;
  isLoading: boolean;
  error: string | null;
};

export function WorkspaceSwitcher({
  workspaces,
  selectedWorkspaceId,
  onSelectWorkspace,
  isLoading,
  error,
}: WorkspaceSwitcherProps) {
  const selectedWorkspace = workspaces.find((workspace) => workspace.id === selectedWorkspaceId);

  return (
    <div className="min-w-0 flex-1 space-y-1">
      <Label htmlFor="workspace-select" className="sr-only">Current workspace</Label>
      <Select
        id="workspace-select"
        value={selectedWorkspaceId ?? ''}
        onChange={(event) => onSelectWorkspace(event.target.value)}
        disabled={isLoading || workspaces.length === 0}
        aria-label="Current workspace"
        className="h-6 min-w-0 border-transparent bg-transparent px-0 py-0 text-[13px] font-bold text-[#091426] shadow-none focus-visible:ring-1 focus-visible:ring-[#3b82f6]"
      >
        {workspaces.length === 0 && <option value="">No workspaces</option>}
        {workspaces.map((workspace) => (
          <option key={workspace.id} value={workspace.id}>
            {workspace.name}
          </option>
        ))}
      </Select>

      <div className="min-w-0 truncate font-mono text-[11px] font-medium leading-[14px] tracking-[0.02em] text-[#45474c]">
        {isLoading && 'Loading workspaces...'}
        {!isLoading && error === null && (selectedWorkspace?.role === 'owner' ? 'Audio Intelligence' : selectedWorkspace?.role ?? selectedWorkspaceId ?? 'No workspace selected')}
      </div>
      {error !== null && <p className="text-xs text-[#ba1a1a]" role="alert">{error}</p>}
    </div>
  );
}
