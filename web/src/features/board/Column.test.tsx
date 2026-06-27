import { DndContext } from '@dnd-kit/core';
import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import type { ColumnWithCards } from './api';
import { Column } from './Column';

function sampleColumn(): ColumnWithCards {
  return {
    id: 'col-a',
    boardId: 'board-1',
    title: 'Todo',
    position: 'V',
    createdAt: '',
    cards: [
      { id: 'card-1', columnId: 'col-a', title: 'First task', description: '', position: 'V', createdAt: '', updatedAt: '' },
      { id: 'card-2', columnId: 'col-a', title: 'Second task', description: '', position: 'k', createdAt: '', updatedAt: '' },
    ],
  };
}

// useSortable/useDroppable require a DndContext ancestor.
function renderColumn(column: ColumnWithCards) {
  render(
    <DndContext>
      <Column
        column={column}
        isHighlighted={false}
        onOpenCard={vi.fn()}
        onAddCard={vi.fn()}
        onDelete={vi.fn()}
      />
    </DndContext>,
  );
}

describe('Column', () => {
  it('renders the column title and its cards in order', () => {
    renderColumn(sampleColumn());

    expect(screen.getByText('Todo')).toBeInTheDocument();
    const cards = screen.getAllByText(/task$/);
    expect(cards.map((node) => node.textContent)).toEqual(['First task', 'Second task']);
  });
});
