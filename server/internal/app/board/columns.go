package board

import (
	"context"

	domain "github.com/ramdanaguss/selaras/server/internal/domain/board"
)

// CreateColumn adds a column to a board at targetIndex, appending when the index
// is nil (spec: omitted position appends after the last column). The placement
// runs in a transaction with conflict retry so a concurrent create cannot leave
// two columns sharing a position.
func (s *Service) CreateColumn(
	ctx context.Context, userID, boardID, title string, targetIndex *int,
) (domain.Column, []domain.Event, error) {
	if err := domain.ValidateTitle(title); err != nil {
		return domain.Column{}, nil, err
	}

	membership, err := s.repo.MembershipByBoard(ctx, boardID, userID)
	if err != nil {
		return domain.Column{}, nil, err
	}
	if err := authorize(membership, false); err != nil {
		return domain.Column{}, nil, err
	}

	column := domain.Column{
		ID:        s.newID(),
		BoardID:   boardID,
		Title:     title,
		CreatedAt: s.clock.Now(),
	}

	err = s.retryOnConflict(ctx, func(tx domain.Repository) error {
		position, placeErr := s.placeColumn(ctx, tx, boardID, column.ID, targetIndex)
		if placeErr != nil {
			return placeErr
		}
		column.Position = position
		return tx.InsertColumn(ctx, column)
	})
	if err != nil {
		return domain.Column{}, nil, err
	}

	events := []domain.Event{domain.ColumnCreated{BoardID: boardID, ColumnID: column.ID, Position: column.Position}}
	return column, events, nil
}

// RenameColumn updates a column's title. Member-gated.
func (s *Service) RenameColumn(ctx context.Context, userID, columnID, title string) ([]domain.Event, error) {
	if err := domain.ValidateTitle(title); err != nil {
		return nil, err
	}

	membership, err := s.repo.MembershipByColumn(ctx, columnID, userID)
	if err != nil {
		return nil, err
	}
	if err := authorize(membership, false); err != nil {
		return nil, err
	}

	if err := s.repo.UpdateColumnTitle(ctx, columnID, title); err != nil {
		return nil, err
	}
	return []domain.Event{domain.ColumnRenamed{BoardID: membership.BoardID, ColumnID: columnID}}, nil
}

// ReorderColumn moves a column to targetIndex within its board as a single-row
// position update, retrying on a concurrent-move conflict (design D3).
func (s *Service) ReorderColumn(
	ctx context.Context, userID, columnID string, targetIndex *int,
) ([]domain.Event, error) {
	membership, err := s.repo.MembershipByColumn(ctx, columnID, userID)
	if err != nil {
		return nil, err
	}
	if err := authorize(membership, false); err != nil {
		return nil, err
	}

	var moved domain.Position
	err = s.retryOnConflict(ctx, func(tx domain.Repository) error {
		position, placeErr := s.placeColumn(ctx, tx, membership.BoardID, columnID, targetIndex)
		if placeErr != nil {
			return placeErr
		}
		moved = position
		return tx.UpdateColumnPosition(ctx, columnID, position)
	})
	if err != nil {
		return nil, err
	}

	return []domain.Event{domain.ColumnMoved{BoardID: membership.BoardID, ColumnID: columnID, Position: moved}}, nil
}

// DeleteColumn deletes a column; the database cascades its cards. Member-gated.
func (s *Service) DeleteColumn(ctx context.Context, userID, columnID string) ([]domain.Event, error) {
	membership, err := s.repo.MembershipByColumn(ctx, columnID, userID)
	if err != nil {
		return nil, err
	}
	if err := authorize(membership, false); err != nil {
		return nil, err
	}

	if err := s.repo.DeleteColumn(ctx, columnID); err != nil {
		return nil, err
	}
	return []domain.Event{domain.ColumnDeleted{BoardID: membership.BoardID, ColumnID: columnID}}, nil
}
