// TanStack Query hooks for the board feature. Query is the sole owner of this
// server state (PRD state taxonomy): nothing is copied into another store, and
// WebSocket patches (M4) will mutate this same cache. The move/reorder hooks are
// optimistic — they patch the cache from an onMutate snapshot and roll back on
// error — so drag-and-drop feels instant.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { showToast } from '../../lib/toast';
import {
  type Board,
  type BoardTree,
  createBoard,
  createCard,
  createColumn,
  deleteCard,
  deleteColumn,
  getBoard,
  listBoards,
  updateCard,
  updateColumn,
} from './api';

const boardsKey = ['boards'] as const;
const boardKey = (boardId: string) => ['board', boardId] as const;

export function useBoards() {
  return useQuery({ queryKey: boardsKey, queryFn: listBoards });
}

export function useBoard(boardId: string) {
  return useQuery({ queryKey: boardKey(boardId), queryFn: () => getBoard(boardId) });
}

export function useCreateBoard() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (title: string) => createBoard(title),
    onSuccess: (board: Board) => {
      void queryClient.invalidateQueries({ queryKey: boardsKey });
      return board;
    },
  });
}

// invalidateBoard is the shared success/settle handler for the non-optimistic
// mutations: re-fetch the board so the cache reflects the server's truth.
function useBoardInvalidator(boardId: string) {
  const queryClient = useQueryClient();
  return () => void queryClient.invalidateQueries({ queryKey: boardKey(boardId) });
}

export function useCreateColumn(boardId: string) {
  const invalidate = useBoardInvalidator(boardId);
  return useMutation({
    mutationFn: (title: string) => createColumn(boardId, title),
    onSuccess: invalidate,
  });
}

export function useDeleteColumn(boardId: string) {
  const invalidate = useBoardInvalidator(boardId);
  return useMutation({ mutationFn: (columnId: string) => deleteColumn(columnId), onSuccess: invalidate });
}

export function useCreateCard(boardId: string) {
  const invalidate = useBoardInvalidator(boardId);
  return useMutation({
    mutationFn: (input: { columnId: string; title: string }) => createCard(input.columnId, input.title),
    onSuccess: invalidate,
  });
}

export function useEditCard(boardId: string) {
  const invalidate = useBoardInvalidator(boardId);
  return useMutation({
    mutationFn: (input: { cardId: string; title?: string; description?: string }) =>
      updateCard(input.cardId, { title: input.title, description: input.description }),
    onSuccess: invalidate,
  });
}

export function useDeleteCard(boardId: string) {
  const invalidate = useBoardInvalidator(boardId);
  return useMutation({ mutationFn: (cardId: string) => deleteCard(cardId), onSuccess: invalidate });
}

export interface MoveCardInput {
  cardId: string;
  fromColumnId: string;
  toColumnId: string;
  toIndex: number;
}

export function useMoveCard(boardId: string) {
  const queryClient = useQueryClient();
  return useMutation<void, Error, MoveCardInput, { previous?: BoardTree }>({
    mutationFn: (input) => updateCard(input.cardId, { columnId: input.toColumnId, position: input.toIndex }),
    onMutate: async (input) => {
      await queryClient.cancelQueries({ queryKey: boardKey(boardId) });
      const previous = queryClient.getQueryData<BoardTree>(boardKey(boardId));
      if (previous) {
        queryClient.setQueryData<BoardTree>(boardKey(boardId), moveCardInTree(previous, input));
      }
      return { previous };
    },
    onError: (_error, _input, context) => {
      if (context?.previous) {
        queryClient.setQueryData(boardKey(boardId), context.previous);
      }
      showToast('Could not move the card — it was returned to its place.');
    },
    onSettled: () => void queryClient.invalidateQueries({ queryKey: boardKey(boardId) }),
  });
}

export interface ReorderColumnInput {
  columnId: string;
  toIndex: number;
}

export function useReorderColumn(boardId: string) {
  const queryClient = useQueryClient();
  return useMutation<void, Error, ReorderColumnInput, { previous?: BoardTree }>({
    mutationFn: (input) => updateColumn(input.columnId, { position: input.toIndex }),
    onMutate: async (input) => {
      await queryClient.cancelQueries({ queryKey: boardKey(boardId) });
      const previous = queryClient.getQueryData<BoardTree>(boardKey(boardId));
      if (previous) {
        queryClient.setQueryData<BoardTree>(boardKey(boardId), moveColumnInTree(previous, input));
      }
      return { previous };
    },
    onError: (_error, _input, context) => {
      if (context?.previous) {
        queryClient.setQueryData(boardKey(boardId), context.previous);
      }
      showToast('Could not reorder the column — it was returned to its place.');
    },
    onSettled: () => void queryClient.invalidateQueries({ queryKey: boardKey(boardId) }),
  });
}

// moveCardInTree returns a new tree with the card removed from its source column
// and inserted into the target column at toIndex — the optimistic shape the board
// renders immediately, before the server confirms the fractional position.
export function moveCardInTree(tree: BoardTree, input: MoveCardInput): BoardTree {
  const moving = tree.columns
    .find((column) => column.id === input.fromColumnId)
    ?.cards.find((card) => card.id === input.cardId);
  if (!moving) {
    return tree;
  }
  const moved = { ...moving, columnId: input.toColumnId };

  const columns = tree.columns.map((column) => {
    if (column.id === input.fromColumnId) {
      return { ...column, cards: column.cards.filter((card) => card.id !== input.cardId) };
    }
    return column;
  });

  return {
    ...tree,
    columns: columns.map((column) => {
      if (column.id !== input.toColumnId) {
        return column;
      }
      const cards = [...column.cards];
      cards.splice(Math.min(input.toIndex, cards.length), 0, moved);
      return { ...column, cards };
    }),
  };
}

// moveColumnInTree returns a new tree with the column moved to toIndex.
export function moveColumnInTree(tree: BoardTree, input: ReorderColumnInput): BoardTree {
  const current = tree.columns.findIndex((column) => column.id === input.columnId);
  if (current < 0) {
    return tree;
  }
  const columns = [...tree.columns];
  const [moved] = columns.splice(current, 1);
  columns.splice(Math.min(input.toIndex, columns.length), 0, moved);
  return { ...tree, columns };
}
