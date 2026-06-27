import { useState } from 'react';
import { Link } from 'react-router';

import { apiErrorMessage } from '../../lib/api';
import { useBoards, useCreateBoard } from './hooks';

// BoardListPage lists the user's boards (TanStack Query owns that state), lets
// them create one, shows an empty state when there are none, and links each board
// to its view.
export function BoardListPage() {
  const boardsQuery = useBoards();
  const createBoard = useCreateBoard();
  const [title, setTitle] = useState('');

  function handleCreate(event: React.FormEvent) {
    event.preventDefault();
    const trimmed = title.trim();
    if (!trimmed) {
      return;
    }
    createBoard.mutate(trimmed, { onSuccess: () => setTitle('') });
  }

  return (
    <section>
      <h1 className="text-2xl font-semibold">Your boards</h1>

      <form onSubmit={handleCreate} className="mt-4 flex gap-2">
        <input
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          placeholder="New board title"
          aria-label="New board title"
          className="flex-1 rounded-md border border-slate-300 px-3 py-2 text-sm"
        />
        <button
          type="submit"
          disabled={createBoard.isPending}
          className="rounded-md bg-sky-600 px-4 py-2 text-sm text-white hover:bg-sky-700 disabled:opacity-50"
        >
          Create
        </button>
      </form>
      {createBoard.isError ? (
        <p role="alert" className="mt-2 text-sm text-red-600">
          {apiErrorMessage(createBoard.error, 'Could not create the board.')}
        </p>
      ) : null}

      {boardsQuery.isPending ? <p className="mt-6 text-slate-600">Loading…</p> : null}

      {boardsQuery.data && boardsQuery.data.length === 0 ? (
        <p className="mt-6 text-slate-600">
          You don’t have any boards yet. Create your first one above.
        </p>
      ) : null}

      {boardsQuery.data && boardsQuery.data.length > 0 ? (
        <ul className="mt-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {boardsQuery.data.map((board) => (
            <li key={board.id}>
              <Link
                to={`/boards/${board.id}`}
                className="block rounded-lg border border-slate-200 bg-white p-4 shadow-sm hover:border-sky-300"
              >
                <span className="font-medium text-slate-800">{board.title}</span>
              </Link>
            </li>
          ))}
        </ul>
      ) : null}
    </section>
  );
}
