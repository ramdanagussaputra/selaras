import {
  closestCorners,
  DndContext,
  type DragEndEvent,
  type DragOverEvent,
  type Over,
  PointerSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import { horizontalListSortingStrategy, SortableContext } from '@dnd-kit/sortable';
import { startTransition, useState } from 'react';
import { useParams } from 'react-router';

import type { Card as CardData } from './api';
import { CardModal } from './CardModal';
import { Column } from './Column';
import {
  useBoard,
  useCreateCard,
  useCreateColumn,
  useDeleteColumn,
  useMoveCard,
  useReorderColumn,
} from './hooks';
import { type ChannelState, useBoardChannel } from './useBoardChannel';

// ConnectionIndicator shows whether live updates are flowing for this board.
function ConnectionIndicator({ state }: Readonly<{ state: ChannelState }>) {
  const live = state === 'live';
  return (
    <span className={`flex items-center gap-1 text-xs ${live ? 'text-green-600' : 'text-amber-600'}`}>
      <span className={`h-2 w-2 rounded-full ${live ? 'bg-green-500' : 'bg-amber-400'}`} />
      {live ? 'Live' : state === 'connecting' ? 'Connecting…' : 'Reconnecting…'}
    </span>
  );
}

// columnIdFromOver resolves which column the drop landed in, whether the pointer
// is over a column header, a card, or an empty column body.
function columnIdFromOver(over: Over | null): string | null {
  if (!over) {
    return null;
  }
  const overType = over.data.current?.type;
  if (overType === 'column') {
    return String(over.id);
  }
  if (overType === 'card' || overType === 'column-body') {
    return String(over.data.current?.columnId);
  }
  return null;
}

export function BoardPage() {
  const { id } = useParams<{ id: string }>();
  const boardId = id ?? '';

  const boardQuery = useBoard(boardId);
  const moveCard = useMoveCard(boardId);
  const reorderColumn = useReorderColumn(boardId);
  const createColumn = useCreateColumn(boardId);
  const createCard = useCreateCard(boardId);
  const deleteColumn = useDeleteColumn(boardId);

  // Subscribe to live updates for this board (patches the same Query cache).
  const channelState = useBoardChannel(boardId);

  const [activeCard, setActiveCard] = useState<CardData | null>(null);
  const [highlightColumnId, setHighlightColumnId] = useState<string | null>(null);

  // An activation distance lets a plain click open a card while a drag moves it.
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }));

  if (boardQuery.isPending) {
    return <p className="text-slate-600">Loading board…</p>;
  }
  if (boardQuery.isError || !boardQuery.data) {
    return <p className="text-slate-600">Could not load this board.</p>;
  }
  const tree = boardQuery.data;

  // The drag-hover highlight is non-urgent, so it runs in a transition and never
  // blocks the drag animation (PRD: startTransition on drag-hover).
  function handleDragOver(event: DragOverEvent) {
    const overColumnId = columnIdFromOver(event.over);
    startTransition(() => setHighlightColumnId(overColumnId));
  }

  function handleDragEnd(event: DragEndEvent) {
    setHighlightColumnId(null);
    const { active, over } = event;
    if (!over) {
      return;
    }

    if (active.data.current?.type === 'column') {
      const overColumnId = columnIdFromOver(over);
      if (!overColumnId || overColumnId === active.id) {
        return;
      }
      const toIndex = tree.columns.findIndex((column) => column.id === overColumnId);
      reorderColumn.mutate({ columnId: String(active.id), toIndex });
      return;
    }

    if (active.data.current?.type === 'card') {
      const fromColumnId = String(active.data.current.columnId);
      const toColumnId = columnIdFromOver(over);
      const targetColumn = tree.columns.find((column) => column.id === toColumnId);
      if (!toColumnId || !targetColumn) {
        return;
      }

      let toIndex = targetColumn.cards.length;
      if (over.data.current?.type === 'card') {
        const overIndex = targetColumn.cards.findIndex((card) => card.id === over.id);
        if (overIndex >= 0) {
          toIndex = overIndex;
        }
      }
      if (toColumnId === fromColumnId && String(active.id) === String(over.id)) {
        return;
      }
      moveCard.mutate({ cardId: String(active.id), fromColumnId, toColumnId, toIndex });
    }
  }

  function handleAddColumn() {
    const title = window.prompt('Column title')?.trim();
    if (title) {
      createColumn.mutate(title);
    }
  }

  function handleAddCard(columnId: string) {
    const title = window.prompt('Card title')?.trim();
    if (title) {
      createCard.mutate({ columnId, title });
    }
  }

  return (
    <section>
      <div className="mb-4 flex items-center gap-3">
        <h1 className="text-2xl font-semibold">{tree.board.title}</h1>
        <ConnectionIndicator state={channelState} />
      </div>

      <DndContext
        sensors={sensors}
        collisionDetection={closestCorners}
        onDragOver={handleDragOver}
        onDragEnd={handleDragEnd}
      >
        <div className="flex gap-4 overflow-x-auto pb-4">
          <SortableContext
            items={tree.columns.map((column) => column.id)}
            strategy={horizontalListSortingStrategy}
          >
            {tree.columns.map((column) => (
              <Column
                key={column.id}
                column={column}
                isHighlighted={highlightColumnId === column.id}
                onOpenCard={setActiveCard}
                onAddCard={handleAddCard}
                onDelete={(columnId) => deleteColumn.mutate(columnId)}
              />
            ))}
          </SortableContext>

          <button
            type="button"
            onClick={handleAddColumn}
            className="h-12 w-72 shrink-0 rounded-lg border-2 border-dashed border-slate-300 text-sm text-slate-500 hover:border-sky-300"
          >
            + Add column
          </button>
        </div>
      </DndContext>

      {activeCard ? (
        <CardModal boardId={boardId} card={activeCard} onClose={() => setActiveCard(null)} />
      ) : null}
    </section>
  );
}
