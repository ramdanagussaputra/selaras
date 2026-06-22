import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';

import {
  refreshAccessToken,
  setAccessToken,
  setUnauthorizedHandler,
} from '../../lib/authClient';
import * as authApi from './api';
import type { User } from './api';
import { AuthContext, type AuthStatus } from './authContext';

// AuthProvider owns the auth session (server state for the current user). The
// access token is held in memory by authClient; this component tracks the user
// and status, bootstraps a session from the refresh cookie on mount, and clears
// state when a refresh fails.
export function AuthProvider({ children }: Readonly<{ children: ReactNode }>) {
  const [user, setUser] = useState<User | null>(null);
  const [status, setStatus] = useState<AuthStatus>('loading');

  const clearSession = useCallback(() => {
    setAccessToken(null);
    setUser(null);
    setStatus('unauthenticated');
  }, []);

  // The interceptor calls this when a refresh fails; the route guard redirects.
  useEffect(() => {
    setUnauthorizedHandler(clearSession);
  }, [clearSession]);

  // Bootstrap: one silent refresh re-establishes a session from the cookie.
  useEffect(() => {
    let active = true;

    void (async () => {
      const refreshed = await refreshAccessToken();
      if (!active) {
        return;
      }
      if (!refreshed) {
        setStatus('unauthenticated');
        return;
      }
      try {
        const profile = await authApi.fetchMe();
        if (active) {
          setUser(profile);
          setStatus('authenticated');
        }
      } catch {
        if (active) {
          clearSession();
        }
      }
    })();

    return () => {
      active = false;
    };
  }, [clearSession]);

  const login = useCallback(async (email: string, password: string) => {
    const profile = await authApi.login(email, password);
    setUser(profile);
    setStatus('authenticated');
  }, []);

  const register = useCallback(
    async (email: string, password: string, displayName: string) => {
      // Registration returns no tokens; the page sends the user to /login.
      await authApi.register(email, password, displayName);
    },
    [],
  );

  const logout = useCallback(async () => {
    try {
      await authApi.logout();
    } finally {
      clearSession();
    }
  }, [clearSession]);

  const value = useMemo(
    () => ({ user, status, login, register, logout }),
    [user, status, login, register, logout],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
