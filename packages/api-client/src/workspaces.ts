import { type SoniqApiClientOptions, requestJSON } from './http';

export type WorkspaceRole = 'owner' | 'member';

export type Workspace = {
  id: string;
  name: string;
  role: WorkspaceRole;
  created_at: string;
  updated_at: string;
};

export type ListWorkspacesResponse = {
  workspaces: Workspace[];
};

export async function listWorkspaces(options: SoniqApiClientOptions = {}): Promise<ListWorkspacesResponse> {
  return requestJSON<ListWorkspacesResponse>('/workspaces', { method: 'GET' }, options);
}
