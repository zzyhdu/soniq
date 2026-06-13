import { describe, expect, it, vi } from 'vitest';

import { signIn, signOut, signUp } from './auth';
import { setUnauthorizedHandler } from './http';

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
}

describe('auth client', () => {
  it('posts signup credentials', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({ user: userFixture() }, { status: 201 }));

    const result = await signUp({
      email: 'owner@local.soniq',
      display_name: 'Owner',
      password: 'correct horse',
    }, { fetch: fetchMock });

    expect(result.user.id).toBe('usr_dev');
    expect(fetchMock).toHaveBeenCalledWith('/auth/signup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'owner@local.soniq',
        display_name: 'Owner',
        password: 'correct horse',
      }),
    });
  });

  it('posts signin credentials', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({ user: userFixture() }));

    const result = await signIn({
      email: 'owner@local.soniq',
      password: 'correct horse',
    }, { fetch: fetchMock });

    expect(result.user.email).toBe('owner@local.soniq');
    expect(fetchMock).toHaveBeenCalledWith('/auth/signin', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'owner@local.soniq',
        password: 'correct horse',
      }),
    });
  });

  it('does not notify the unauthorized handler for rejected signin credentials', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse(apiError('invalid_credentials', 'invalid email or password', 401), {
        status: 401,
        statusText: 'Unauthorized',
      }),
    );
    const unauthorizedHandler = vi.fn();
    const cleanupUnauthorizedHandler = setUnauthorizedHandler(unauthorizedHandler);

    try {
      await expect(signIn({
        email: 'owner@local.soniq',
        password: 'wrong horse',
      }, { fetch: fetchMock })).rejects.toMatchObject({
        code: 'invalid_credentials',
        status: 401,
        message: 'invalid email or password',
      });
    } finally {
      cleanupUnauthorizedHandler();
    }

    expect(unauthorizedHandler).not.toHaveBeenCalled();
  });

  it('posts signout', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(new Response(null, { status: 204 }));

    await signOut({ fetch: fetchMock });

    expect(fetchMock).toHaveBeenCalledWith('/auth/signout', { method: 'POST' });
  });
});

function userFixture() {
  return {
    id: 'usr_dev',
    email: 'owner@local.soniq',
    display_name: 'Owner',
    created_at: '2026-06-11T00:00:00Z',
    updated_at: '2026-06-11T00:00:00Z',
  };
}

function apiError(code: string, message: string, status: number) {
  return { code, message, status };
}
