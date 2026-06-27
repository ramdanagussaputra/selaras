import { useSortable } from '@dnd-kit/sortable';

import type { Card as CardData } from './api';

interface CardProps {
  card: CardData;
  onOpen: (card: CardData) => void;
}

// Card is a sortable, draggable item. The pointer sensor in BoardPage uses an
// activation distance, so a plain click (no drag) falls through to onOpen and
// opens the detail modal.
export function Card({ card, onOpen }: Readonly<CardProps>) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: card.id,
    data: { type: 'card', columnId: card.columnId },
  });

  const style = {
    transform: transform ? `translate3d(${transform.x}px, ${transform.y}px, 0)` : undefined,
    transition,
    opacity: isDragging ? 0.5 : 1,
  };

  return (
    <button
      ref={setNodeRef}
      style={style}
      {...attributes}
      {...listeners}
      type="button"
      onClick={() => onOpen(card)}
      className="w-full cursor-grab rounded-md border border-slate-200 bg-white px-3 py-2 text-left text-sm shadow-sm hover:border-sky-300"
    >
      {card.title}
    </button>
  );
}
