import { describe, expect, it, vi } from 'vitest';

import { setUnauthorizedHandler } from './http';
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

  it('notifies the unauthorized handler for protected 401 responses', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse(apiError('unauthenticated', 'resolve current user', 401), {
        status: 401,
        statusText: 'Unauthorized',
      }),
    );
    const unauthorizedHandler = vi.fn();
    const cleanupUnauthorizedHandler = setUnauthorizedHandler(unauthorizedHandler);

    try {
      await expect(getMe({ fetch: fetchMock })).rejects.toMatchObject({
        code: 'unauthenticated',
        status: 401,
        message: 'resolve current user',
      });
    } finally {
      cleanupUnauthorizedHandler();
    }

    expect(unauthorizedHandler).toHaveBeenCalledTimes(1);
    expect(unauthorizedHandler.mock.calls[0]?.[0]).toMatchObject({ status: 401 });
  });
});

function apiError(code: string, message: string, status: number) {
  return { code, message, status };
}
