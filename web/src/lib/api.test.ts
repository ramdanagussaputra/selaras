import { afterEach, describe, expect, it, vi } from 'vitest';

import { ApiError, apiFetch } from './api';

function stubFetch(status: number, body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    }),
  );
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

describe('apiFetch', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('returns the parsed JSON body on success', async () => {
    stubFetch(200, { status: 'ok' });

    await expect(apiFetch<{ status: string }>('/healthz')).resolves.toEqual({ status: 'ok' });
  });

  it('throws ApiError with status and body on non-2xx', async () => {
    stubFetch(503, { status: 'degraded' });

    const err = await apiFetch('/healthz').catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(503);
    expect((err as ApiError).body).toEqual({ status: 'degraded' });
  });

  it('sends an Accept: application/json header', async () => {
    const fetchMock = stubFetch(200, {});

    await apiFetch('/healthz');

    expect(fetchMock).toHaveBeenCalledWith(
      '/healthz',
      expect.objectContaining({ headers: expect.objectContaining({ Accept: 'application/json' }) }),
    );
  });
});
