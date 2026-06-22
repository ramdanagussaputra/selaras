// Authenticated request layer: holds the access token in memory (never web
// storage), injects it as a Bearer header, and on a 401 performs a single shared
// refresh then retries once. On refresh failure it notifies the app to clear the
// session and redirect (design D10). The refresh token itself is the HttpOnly
// cookie the browser sends automatically with credentials: 'include'.

import { ApiError, apiFetch } from './api';

let accessToken: string | null = null;
let onUnauthorized: () => void = () => {};
let refreshInFlight: Promise<boolean> | null = null;

export function setAccessToken(token: string | null): void {
  accessToken = token;
}

export function getAccessToken(): string | null {
  return accessToken;
}

// setUnauthorizedHandler registers the callback invoked when a refresh fails, so
// the AuthProvider can clear state and the route guard can redirect to /login.
export function setUnauthorizedHandler(handler: () => void): void {
  onUnauthorized = handler;
}

interface RefreshResponse {
  accessToken: string;
}

// refreshAccessToken refreshes the access token, sharing a single in-flight
// request so concurrent 401s trigger exactly one /auth/refresh call per tab.
export function refreshAccessToken(): Promise<boolean> {
  if (!refreshInFlight) {
    refreshInFlight = (async (): Promise<boolean> => {
      try {
        const data = await apiFetch<RefreshResponse>('/api/v1/auth/refresh', {
          method: 'POST',
          credentials: 'include',
        });
        accessToken = data.accessToken;
        return true;
      } catch {
        accessToken = null;
        return false;
      } finally {
        refreshInFlight = null;
      }
    })();
  }
  return refreshInFlight;
}

// authFetch performs an authenticated request, refreshing once on a 401 and
// retrying; if the refresh fails it signals the unauthorized handler and rethrows.
export async function authFetch<ResponseBody>(
  path: string,
  init: RequestInit = {},
): Promise<ResponseBody> {
  try {
    return await requestWithToken<ResponseBody>(path, init);
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      const refreshed = await refreshAccessToken();
      if (refreshed) {
        return await requestWithToken<ResponseBody>(path, init);
      }
      onUnauthorized();
    }
    throw error;
  }
}

function requestWithToken<ResponseBody>(path: string, init: RequestInit): Promise<ResponseBody> {
  const headers: Record<string, string> = { ...(init.headers as Record<string, string>) };
  if (accessToken) {
    headers.Authorization = `Bearer ${accessToken}`;
  }
  return apiFetch<ResponseBody>(path, { ...init, headers, credentials: 'include' });
}
