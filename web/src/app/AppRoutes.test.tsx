import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { AppRoutes } from './AppRoutes';

function renderAt(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <AppRoutes />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('AppRoutes', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it.each([
    ['/', 'Your boards'],
    ['/login', 'Login'],
    ['/boards/123', 'Board 123'],
  ])('renders the placeholder page at %s', (path, heading) => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ status: 'ok' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    );

    renderAt(path);

    expect(screen.getByRole('heading', { name: heading })).toBeInTheDocument();
  });
});
