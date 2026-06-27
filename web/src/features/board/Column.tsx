import { useDroppable } from '@dnd-kit/core';
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable';

import type { Card as CardData, ColumnWithCards } from './api';
import { Card } from './Card';

interface ColumnProps {
  column: ColumnWithCards;
  isHighlighted: boolean;
  onOpenCard: (card: CardData) => void;
  onAddCard: (columnId: string) => void;
  onDelete: (columnId: string) => void;
}

// Column is both a sortable item (so columns can be reordered) and a droppable
// region (so a card can be dropped into an empty column). Its cards live in their
// own vertical SortableContext.
export function Column({ column, isHighlighted, onOpenCard, onAddCard, onDelete }: Readonly<ColumnProps>) {
  const { attributes, listeners, setNodeRef, transform, transition } = useSortable({
    id: column.id,
    data: { type: 'column' },
  });
  const { setNodeRef: setDropRef } = useDroppable({
    id: `column-body-${column.id}`,
    data: { type: 'column-body', columnId: column.id },
  });

  const style = {
    transform: transform ? `translate3d(${transform.x}px, ${transform.y}px, 0)` : undefined,
    transition,
  };

  return (
    <section
      ref={setNodeRef}
      style={style}
      className={`flex w-72 shrink-0 flex-col rounded-lg bg-slate-100 p-3 ${
        isHighlighted ? 'ring-2 ring-sky-400' : ''
      }`}
    >
      <header className="mb-3 flex items-center justify-between">
        <h2 {...attributes} {...listeners} className="cursor-grab text-sm font-semibold text-slate-700">
          {column.title}
        </h2>
        <button
          type="button"
          onClick={() => onDelete(column.id)}
          aria-label={`Delete column ${column.title}`}
          className="text-xs text-slate-400 hover:text-red-500"
        >
          ✕
        </button>
      </header>

      <div ref={setDropRef} className="flex min-h-[2rem] flex-col gap-2">
        <SortableContext items={column.cards.map((card) => card.id)} strategy={verticalListSortingStrategy}>
          {column.cards.map((card) => (
            <Card key={card.id} card={card} onOpen={onOpenCard} />
          ))}
        </SortableContext>
      </div>

      <button
        type="button"
        onClick={() => onAddCard(column.id)}
        className="mt-3 rounded-md px-2 py-1 text-left text-sm text-slate-500 hover:bg-slate-200"
      >
        + Add card
      </button>
    </section>
  );
}
