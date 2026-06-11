import { type Workspace } from '@soniq/api-client';

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
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
  return (
    <Card>
      <CardHeader>
        <CardTitle>Workspace</CardTitle>
        <CardDescription>{selectedWorkspaceId ?? 'No workspace selected'}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="space-y-2">
          <Label htmlFor="workspace-select">Current workspace</Label>
          <Select
            id="workspace-select"
            value={selectedWorkspaceId ?? ''}
            onChange={(event) => onSelectWorkspace(event.target.value)}
            disabled={isLoading || workspaces.length === 0}
          >
            {workspaces.length === 0 && <option value="">No workspaces</option>}
            {workspaces.map((workspace) => (
              <option key={workspace.id} value={workspace.id}>
                {workspace.name}
              </option>
            ))}
          </Select>
        </div>

        {isLoading && <p className="text-muted-foreground text-sm">Loading workspaces...</p>}
        {error !== null && <p className="text-destructive text-sm" role="alert">{error}</p>}
      </CardContent>
    </Card>
  );
}
