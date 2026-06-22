import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { AuthProvider } from '../features/auth/AuthProvider';
import { setAccessToken } from '../lib/authClient';
import { AppRoutes } from './AppRoutes';

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

// stubFetch routes responses by URL substring; anything unmatched returns 200 ok.
function stubFetch(handlers: Array<[string, () => Response]>): void {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: string | URL | Request) => {
      const requestUrl = typeof input === 'string' ? input : input.toString();
      for (const [needle, make] of handlers) {
        if (requestUrl.includes(needle)) {
          return Promise.resolve(make());
        }
      }
      return Promise.resolve(jsonResponse(200, { status: 'ok' }));
    }),
  );
}

function renderApp(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <AuthProvider>
          <AppRoutes />
        </AuthProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('AppRoutes', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    setAccessToken(null);
  });

  describe('unauthenticated (bootstrap refresh fails)', () => {
    const unauthenticated: Array<[string, () => Response]> = [
      ['/api/v1/auth/refresh', () => jsonResponse(401, { error: { code: 'TOKEN_INVALID' } })],
    ];

    it('shows the sign-in page at /login', async () => {
      stubFetch(unauthenticated);
      renderApp('/login');
      expect(await screen.findByRole('heading', { name: 'Sign in' })).toBeInTheDocument();
    });

    it('shows the register page at /register', async () => {
      stubFetch(unauthenticated);
      renderApp('/register');
      expect(await screen.findByRole('heading', { name: 'Create account' })).toBeInTheDocument();
    });

    it('redirects a protected route to /login', async () => {
      stubFetch(unauthenticated);
      renderApp('/');
      expect(await screen.findByRole('heading', { name: 'Sign in' })).toBeInTheDocument();
    });
  });

  describe('authenticated (bootstrap refresh succeeds)', () => {
    it('renders the protected board list at /', async () => {
      stubFetch([
        ['/api/v1/auth/refresh', () => jsonResponse(200, { accessToken: 'token' })],
        [
          '/api/v1/me',
          () =>
            jsonResponse(200, {
              user: { id: '1', email: 'user@example.com', displayName: 'User', createdAt: '' },
            }),
        ],
      ]);
      renderApp('/');
      expect(await screen.findByRole('heading', { name: 'Your boards' })).toBeInTheDocument();
    });
  });
});
