// useBoardChannel subscribes the board view to live updates over the broadcast-
// only WebSocket. It patches the TanStack Query cache on each event (so peers'
// changes appear without a refetch), suppresses the echo of its own connection,
// and reconnects with jittered backoff — resyncing via invalidation on every
// reconnect so anything missed during a gap re-converges (design D9).

import { useQueryClient, type QueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';

import { getAccessToken } from '../../lib/authClient';
import type { BoardTree, Card } from './api';
import { setConnectionId } from './connection';

export type ChannelState = 'connecting' | 'live' | 'reconnecting';

interface Envelope {
  type: string;
  boardId: string;
  actor: { userId: string; connId: string };
  payload: {
    columnId?: string;
    cardId?: string;
    fromColumnId?: string;
    toColumnId?: string;
    position?: string;
  };
}

const reconnectBaseDelay = 1000;
const reconnectMaxDelay = 30000;

function boardSocketURL(boardId: string): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${protocol}//${window.location.host}/api/v1/ws?board=${boardId}`;
}

// applyCardMoved relocates a card to its new column and position, keeping the
// target column ordered by position. Exported for unit testing.
export function applyCardMoved(
  tree: BoardTree,
  input: { cardId: string; toColumnId: string; position: string },
): BoardTree {
  let moving: Card | undefined;
  const stripped = tree.columns.map((column) => ({
    ...column,
    cards: column.cards.filter((card) => {
      if (card.id === input.cardId) {
        moving = card;
        return false;
      }
      return true;
    }),
  }));
  if (!moving) {
    return tree;
  }
  const moved: Card = { ...moving, columnId: input.toColumnId, position: input.position };

  return {
    ...tree,
    columns: stripped.map((column) => {
      if (column.id !== input.toColumnId) {
        return column;
      }
      const cards = [...column.cards, moved].sort((left, right) =>
        left.position < right.position ? -1 : left.position > right.position ? 1 : 0,
      );
      return { ...column, cards };
    }),
  };
}

// applyCardDeleted removes a card from whichever column holds it.
export function applyCardDeleted(tree: BoardTree, cardId: string): BoardTree {
  return {
    ...tree,
    columns: tree.columns.map((column) => ({
      ...column,
      cards: column.cards.filter((card) => card.id !== cardId),
    })),
  };
}

// patchBoardCache applies the events worth patching in place (card moves and
// deletes — the hot path) and returns true. For every other event type it
// returns false, signalling the caller to invalidate and refetch (design D5/D9).
export function patchBoardCache(queryClient: QueryClient, boardId: string, envelope: Envelope): boolean {
  const cacheKey = ['board', boardId];
  const current = queryClient.getQueryData<BoardTree>(cacheKey);
  if (!current) {
    return false;
  }

  switch (envelope.type) {
    case 'card.moved':
      if (envelope.payload.cardId && envelope.payload.toColumnId && envelope.payload.position) {
        queryClient.setQueryData<BoardTree>(
          cacheKey,
          applyCardMoved(current, {
            cardId: envelope.payload.cardId,
            toColumnId: envelope.payload.toColumnId,
            position: envelope.payload.position,
          }),
        );
        return true;
      }
      return false;
    case 'card.deleted':
      if (envelope.payload.cardId) {
        queryClient.setQueryData<BoardTree>(cacheKey, applyCardDeleted(current, envelope.payload.cardId));
        return true;
      }
      return false;
    default:
      return false;
  }
}

export function useBoardChannel(boardId: string): ChannelState {
  const queryClient = useQueryClient();
  const [state, setState] = useState<ChannelState>('connecting');

  useEffect(() => {
    let socket: WebSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined;
    let attempts = 0;
    let disposed = false;
    let connectionId: string | null = null;

    function resync() {
      void queryClient.invalidateQueries({ queryKey: ['board', boardId] });
    }

    function scheduleReconnect() {
      if (disposed) {
        return;
      }
      setState('reconnecting');
      const delay = Math.min(reconnectBaseDelay * 2 ** attempts, reconnectMaxDelay);
      const jitter = Math.random() * 0.3 * delay;
      attempts += 1;
      reconnectTimer = setTimeout(connect, delay + jitter);
    }

    function handleEnvelope(envelope: Envelope) {
      if (envelope.actor?.connId && envelope.actor.connId === connectionId) {
        return; // our own change — the optimistic update already applied (D7)
      }
      if (!patchBoardCache(queryClient, boardId, envelope)) {
        resync();
      }
    }

    function connect() {
      if (disposed) {
        return;
      }
      const reconnected = attempts > 0;
      const next = new WebSocket(boardSocketURL(boardId));
      socket = next;

      next.onopen = () => next.send(JSON.stringify({ type: 'auth', token: getAccessToken() }));
      next.onmessage = (event: MessageEvent) => {
        const message = JSON.parse(event.data as string) as Envelope & { connId?: string };
        if (message.type === 'welcome') {
          connectionId = message.connId ?? null;
          setConnectionId(connectionId);
          attempts = 0;
          setState('live');
          if (reconnected) {
            resync(); // pull anything missed during the gap (D9)
          }
          return;
        }
        handleEnvelope(message);
      };
      next.onclose = () => {
        setConnectionId(null);
        connectionId = null;
        scheduleReconnect();
      };
      next.onerror = () => next.close();
    }

    connect();
    return () => {
      disposed = true;
      clearTimeout(reconnectTimer);
      setConnectionId(null);
      socket?.close();
    };
  }, [boardId, queryClient]);

  return state;
}
