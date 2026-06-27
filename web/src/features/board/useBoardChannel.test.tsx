import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { BoardTree } from './api';
import { applyCardMoved, useBoardChannel } from './useBoardChannel';

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

describe('applyCardMoved', () => {
  it('relocates the card and orders the target column by position', () => {
    const next = applyCardMoved(sampleTree(), { cardId: 'card-1', toColumnId: 'col-b', position: 'k' });
    expect(next.columns[0].cards).toHaveLength(0);
    expect(next.columns[1].cards[0]).toMatchObject({ id: 'card-1', columnId: 'col-b', position: 'k' });
  });
});

// FakeWebSocket lets the test drive the connection lifecycle deterministically.
class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  socketUrl: string;
  sent: string[] = [];
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  constructor(socketUrl: string) {
    this.socketUrl = socketUrl;
    FakeWebSocket.instances.push(this);
  }
  send(data: string) {
    this.sent.push(data);
  }
  close() {
    this.onclose?.();
  }
  emit(message: unknown) {
    this.onmessage?.({ data: JSON.stringify(message) });
  }
}

describe('useBoardChannel', () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket);
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  function renderChannel() {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    queryClient.setQueryData(['board', 'board-1'], sampleTree());
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
    const view = renderHook(() => useBoardChannel('board-1'), { wrapper });
    return { queryClient, view };
  }

  function welcome(socket: FakeWebSocket, connId: string) {
    act(() => socket.onopen?.());
    act(() => socket.emit({ type: 'welcome', connId }));
  }

  it('patches the cache on a peer card.moved and ignores its own echo', () => {
    const { queryClient } = renderChannel();
    const socket = FakeWebSocket.instances[0];
    welcome(socket, 'mine');

    // A peer's move patches the cache.
    act(() =>
      socket.emit({
        type: 'card.moved',
        boardId: 'board-1',
        actor: { userId: 'user-b', connId: 'peer' },
        payload: { cardId: 'card-1', toColumnId: 'col-b', position: 'k' },
      }),
    );
    let tree = queryClient.getQueryData<BoardTree>(['board', 'board-1']);
    expect(tree?.columns[1].cards.map((card) => card.id)).toEqual(['card-1']);

    // Our own echo (matching connId) is ignored — the card stays put.
    act(() =>
      socket.emit({
        type: 'card.moved',
        boardId: 'board-1',
        actor: { userId: 'user-a', connId: 'mine' },
        payload: { cardId: 'card-1', toColumnId: 'col-a', position: 'V' },
      }),
    );
    tree = queryClient.getQueryData<BoardTree>(['board', 'board-1']);
    expect(tree?.columns[1].cards.map((card) => card.id)).toEqual(['card-1']);
  });

  it('reconnects after a drop and resyncs by invalidating the board query', () => {
    vi.useFakeTimers();
    const { queryClient } = renderChannel();
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries');

    const first = FakeWebSocket.instances[0];
    welcome(first, 'mine'); // initial connect does not invalidate
    expect(invalidate).not.toHaveBeenCalled();

    act(() => first.close()); // network drop → schedule reconnect
    act(() => vi.advanceTimersByTime(2000)); // past the first backoff delay

    expect(FakeWebSocket.instances.length).toBeGreaterThan(1);
    const second = FakeWebSocket.instances[FakeWebSocket.instances.length - 1];
    welcome(second, 'mine-2'); // reconnect → resync

    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['board', 'board-1'] });
  });
});
