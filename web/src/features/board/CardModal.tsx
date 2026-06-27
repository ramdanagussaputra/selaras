import { useState } from 'react';

import type { Card } from './api';
import { useDeleteCard, useEditCard } from './hooks';

interface CardModalProps {
  boardId: string;
  card: Card;
  onClose: () => void;
}

// CardModal edits a card's title and description and saves them via PATCH, then
// closes. Deleting the card is offered here too. The mutations invalidate the
// board query, so the board view reflects the change on close.
export function CardModal({ boardId, card, onClose }: Readonly<CardModalProps>) {
  const [title, setTitle] = useState(card.title);
  const [description, setDescription] = useState(card.description);
  const editCard = useEditCard(boardId);
  const removeCard = useDeleteCard(boardId);

  function handleSave() {
    editCard.mutate({ cardId: card.id, title, description }, { onSuccess: onClose });
  }

  function handleDelete() {
    removeCard.mutate(card.id, { onSuccess: onClose });
  }

  return (
    <div
      className="fixed inset-0 z-40 flex items-center justify-center bg-slate-900/40 p-4"
      role="dialog"
      aria-modal="true"
      onClick={onClose}
    >
      <div
        className="w-full max-w-lg rounded-lg bg-white p-6 shadow-xl"
        onClick={(event) => event.stopPropagation()}
      >
        <label className="mb-1 block text-xs font-medium text-slate-500" htmlFor="card-title">
          Title
        </label>
        <input
          id="card-title"
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          className="mb-4 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
        />

        <label className="mb-1 block text-xs font-medium text-slate-500" htmlFor="card-description">
          Description
        </label>
        <textarea
          id="card-description"
          value={description}
          rows={6}
          onChange={(event) => setDescription(event.target.value)}
          className="mb-4 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
        />

        <div className="flex items-center justify-between">
          <button
            type="button"
            onClick={handleDelete}
            className="text-sm text-red-500 hover:text-red-600"
          >
            Delete
          </button>
          <div className="flex gap-2">
            <button type="button" onClick={onClose} className="rounded-md px-3 py-2 text-sm text-slate-600">
              Cancel
            </button>
            <button
              type="button"
              onClick={handleSave}
              disabled={editCard.isPending}
              className="rounded-md bg-sky-600 px-3 py-2 text-sm text-white hover:bg-sky-700 disabled:opacity-50"
            >
              Save
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
