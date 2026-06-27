// Board API client. Every call goes through authFetch so it carries the Bearer
// token and the refresh-once-on-401 behavior, exactly like the rest of the app.

import { authFetch } from '../../lib/authClient';
import { getConnectionId } from './connection';

// withConnId stamps the originating WebSocket connection id on a mutation, so the
// broadcast it triggers is suppressed as an echo on this client (design D7).
function withConnId(headers: Record<string, string> = {}): Record<string, string> {
  const connectionId = getConnectionId();
  return connectionId ? { ...headers, 'X-Conn-Id': connectionId } : headers;
}

export interface Board {
  id: string;
  ownerId: string;
  title: string;
  createdAt: string;
  updatedAt: string;
}

export interface Card {
  id: string;
  columnId: string;
  title: string;
  description: string;
  position: string;
  createdAt: string;
  updatedAt: string;
}

export interface ColumnWithCards {
  id: string;
  boardId: string;
  title: string;
  position: string;
  createdAt: string;
  cards: Card[];
}

export interface BoardTree {
  board: Board;
  columns: ColumnWithCards[];
}

const jsonHeaders = { 'Content-Type': 'application/json' };

export async function listBoards(): Promise<Board[]> {
  const response = await authFetch<{ boards: Board[] }>('/api/v1/boards');
  return response.boards;
}

export async function createBoard(title: string): Promise<Board> {
  const response = await authFetch<{ board: Board }>('/api/v1/boards', {
    method: 'POST',
    headers: withConnId(jsonHeaders),
    body: JSON.stringify({ title }),
  });
  return response.board;
}

export async function getBoard(boardId: string): Promise<BoardTree> {
  return authFetch<BoardTree>(`/api/v1/boards/${boardId}`);
}

export async function renameBoard(boardId: string, title: string): Promise<void> {
  await authFetch(`/api/v1/boards/${boardId}`, {
    method: 'PATCH',
    headers: withConnId(jsonHeaders),
    body: JSON.stringify({ title }),
  });
}

export async function deleteBoard(boardId: string): Promise<void> {
  await authFetch(`/api/v1/boards/${boardId}`, { method: 'DELETE', headers: withConnId() });
}

export async function createColumn(
  boardId: string,
  title: string,
  position?: number,
): Promise<ColumnWithCards> {
  const response = await authFetch<{ column: ColumnWithCards }>(
    `/api/v1/boards/${boardId}/columns`,
    { method: 'POST', headers: withConnId(jsonHeaders), body: JSON.stringify({ title, position }) },
  );
  return response.column;
}

export interface ColumnPatch {
  title?: string;
  position?: number;
}

export async function updateColumn(columnId: string, patch: ColumnPatch): Promise<void> {
  await authFetch(`/api/v1/columns/${columnId}`, {
    method: 'PATCH',
    headers: withConnId(jsonHeaders),
    body: JSON.stringify(patch),
  });
}

export async function deleteColumn(columnId: string): Promise<void> {
  await authFetch(`/api/v1/columns/${columnId}`, { method: 'DELETE', headers: withConnId() });
}

export async function createCard(columnId: string, title: string, position?: number): Promise<Card> {
  const response = await authFetch<{ card: Card }>(`/api/v1/columns/${columnId}/cards`, {
    method: 'POST',
    headers: withConnId(jsonHeaders),
    body: JSON.stringify({ title, position }),
  });
  return response.card;
}

export interface CardPatch {
  title?: string;
  description?: string;
  columnId?: string;
  position?: number;
}

export async function updateCard(cardId: string, patch: CardPatch): Promise<void> {
  await authFetch(`/api/v1/cards/${cardId}`, {
    method: 'PATCH',
    headers: withConnId(jsonHeaders),
    body: JSON.stringify(patch),
  });
}

export async function deleteCard(cardId: string): Promise<void> {
  await authFetch(`/api/v1/cards/${cardId}`, { method: 'DELETE', headers: withConnId() });
}
