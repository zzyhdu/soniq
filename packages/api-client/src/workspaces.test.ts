import { describe, expect, it, vi } from 'vitest';

import { listWorkspaces } from './workspaces';

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
}

describe('listWorkspaces', () => {
  it('loads workspaces for the current user', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse({
        workspaces: [
          {
            id: 'wsp_default',
            name: 'Default Workspace',
            role: 'owner',
            created_at: '2026-06-11T00:00:00Z',
            updated_at: '2026-06-11T00:00:00Z',
          },
        ],
      }),
    );

    const result = await listWorkspaces({ fetch: fetchMock });

    expect(result.workspaces).toHaveLength(1);
    expect(result.workspaces[0]?.role).toBe('owner');
    expect(fetchMock).toHaveBeenCalledWith('/workspaces', { method: 'GET' });
  });
});
