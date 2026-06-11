import { describe, expect, it, vi } from 'vitest';

import { getMe } from './users';

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
}

describe('getMe', () => {
  it('loads the current user', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse({
        id: 'usr_dev',
        email: 'dev@local.soniq',
        display_name: 'Local Developer',
        created_at: '2026-06-11T00:00:00Z',
        updated_at: '2026-06-11T00:00:00Z',
      }),
    );

    const result = await getMe({ fetch: fetchMock });

    expect(result.id).toBe('usr_dev');
    expect(fetchMock).toHaveBeenCalledWith('/me', { method: 'GET' });
  });
});
