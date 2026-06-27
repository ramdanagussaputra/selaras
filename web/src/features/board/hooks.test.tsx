import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { BoardTree } from './api';
import { moveCardInTree, useMoveCard } from './hooks';

function sampleTree(): BoardTree {
  return {
    board: { id: 'board-1', ownerId: 'owner', title: 'Sprint', createdAt: '', updatedAt: '' },
    columns: [
      {
        id: 'col-a',
        boardId: 'board-1',
        title: 'Todo',
        position: 'V',
        createdAt: '',
        cards: [{ id: 'card-1', columnId: 'col-a', title: 'Task', description: '', position: 'V', createdAt: '', updatedAt: '' }],
      },
      { id: 'col-b', boardId: 'board-1', title: 'Doing', position: 'k', createdAt: '', cards: [] },
    ],
  };
}

describe('moveCardInTree', () => {
  it('moves a card from its source column into the target column', () => {
    const next = moveCardInTree(sampleTree(), {
      cardId: 'card-1',
      fromColumnId: 'col-a',
      toColumnId: 'col-b',
      toIndex: 0,
    });
    expect(next.columns[0].cards).toHaveLength(0);
    expect(next.columns[1].cards.map((card) => card.id)).toEqual(['card-1']);
    expect(next.columns[1].cards[0].columnId).toBe('col-b');
  });
});

describe('useMoveCard', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('optimistically patches the cache, then rolls back when the PATCH fails', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    queryClient.setQueryData(['board', 'board-1'], sampleTree());

    // A controllable fetch: pending until we resolve it with a 500, so we can
    // observe the optimistic cache before the failure rolls it back (AC-2).
    let settle: (response: unknown) => void = () => {};
    const fetchMock = vi.fn(() => new Promise((resolve) => { settle = resolve; }));
    vi.stubGlobal('fetch', fetchMock);

    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useMoveCard('board-1'), { wrapper });

    act(() => {
      result.current.mutate({ cardId: 'card-1', fromColumnId: 'col-a', toColumnId: 'col-b', toIndex: 0 });
    });

    // Optimistic: the card is already shown in the destination column.
    await waitFor(() => {
      const tree = queryClient.getQueryData<BoardTree>(['board', 'board-1']);
      expect(tree?.columns[1].cards.map((card) => card.id)).toEqual(['card-1']);
    });

    // The PATCH fails (500) → the cache rolls back to the snapshot.
    act(() => {
      settle({ ok: false, status: 500, json: async () => ({ error: { code: 'INTERNAL' } }) });
    });

    await waitFor(() => {
      const tree = queryClient.getQueryData<BoardTree>(['board', 'board-1']);
      expect(tree?.columns[0].cards.map((card) => card.id)).toEqual(['card-1']);
      expect(tree?.columns[1].cards).toHaveLength(0);
    });
  });
});
