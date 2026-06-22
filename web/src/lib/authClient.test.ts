import { afterEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from './api';
import {
  authFetch,
  refreshAccessToken,
  setAccessToken,
  setUnauthorizedHandler,
} from './authClient';

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
  setAccessToken(null);
  setUnauthorizedHandler(() => {});
});

describe('authFetch', () => {
  it('refreshes once on 401 then retries with the new token', async () => {
    setAccessToken('expired');
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: { code: 'TOKEN_EXPIRED' } })) // first /me
      .mockResolvedValueOnce(jsonResponse(200, { accessToken: 'fresh' })) // refresh
      .mockResolvedValueOnce(jsonResponse(200, { user: { id: '1' } })); // retried /me
    vi.stubGlobal('fetch', fetchMock);

    const result = await authFetch<{ user: { id: string } }>('/api/v1/me');

    expect(result).toEqual({ user: { id: '1' } });
    expect(fetchMock).toHaveBeenCalledTimes(3);

    const retryInit = fetchMock.mock.calls[2][1] as RequestInit;
    const retryHeaders = retryInit.headers as Record<string, string>;
    expect(retryHeaders.Authorization).toBe('Bearer fresh');
  });

  it('calls the unauthorized handler and rethrows when refresh fails', async () => {
    setAccessToken('expired');
    const onUnauthorized = vi.fn();
    setUnauthorizedHandler(onUnauthorized);

    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: { code: 'TOKEN_EXPIRED' } })) // /me
      .mockResolvedValueOnce(jsonResponse(401, { error: { code: 'TOKEN_INVALID' } })); // refresh fails
    vi.stubGlobal('fetch', fetchMock);

    await expect(authFetch('/api/v1/me')).rejects.toBeInstanceOf(ApiError);
    expect(onUnauthorized).toHaveBeenCalledOnce();
  });

  it('shares a single in-flight refresh across concurrent callers', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { accessToken: 'fresh' }));
    vi.stubGlobal('fetch', fetchMock);

    const [first, second] = await Promise.all([refreshAccessToken(), refreshAccessToken()]);

    expect(first).toBe(true);
    expect(second).toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
