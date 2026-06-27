// Package board holds the kanban use cases — board, column, and card CRUD plus
// drag-and-drop reordering — orchestrating the domain rules through the
// Repository port. It depends on the domain only (the dependency rule);
// pgx/sqlc arrive as an injected adapter and uuid generation lives here so the
// domain stays free of id concerns (design D8).
package board

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	domain "github.com/ramdanaguss/selaras/server/internal/domain/board"
)

// maxConflictRetries bounds the re-read-recompute-retry loop a concurrent move
// runs on a unique-violation before giving up with ErrConflict (design D3).
const maxConflictRetries = 3

// Clock supplies the current time so created/updated timestamps are deterministic
// under test (a fixed clock) instead of reading the wall clock directly.
type Clock interface {
	Now() time.Time
}

// Service coordinates the board use cases over the domain port.
type Service struct {
	repo  domain.Repository
	clock Clock
	newID func() string // overridable in tests; defaults to uuid.NewString
}

// NewService wires the use cases with their repository and clock.
func NewService(repo domain.Repository, clock Clock) *Service {
	return &Service{repo: repo, clock: clock, newID: uuid.NewString}
}

// Tree is the nested read returned by GetBoard: a board with its columns,
// each carrying its cards, all ordered by position (design D6).
type Tree struct {
	Board   domain.Board
	Columns []ColumnWithCards
}

// ColumnWithCards is one column and its ordered cards within a Tree.
type ColumnWithCards struct {
	Column domain.Column
	Cards  []domain.Card
}

// authorize turns a resolved membership into the contract's access decision: a
// non-member (or a missing row) is 404 so existence never leaks, and an
// owner-only action attempted by a plain member is 403 (design D7).
func authorize(membership domain.Membership, ownerOnly bool) error {
	if !membership.Found {
		return domain.ErrNotFound
	}
	if ownerOnly && membership.Role != domain.RoleOwner {
		return domain.ErrForbidden
	}
	return nil
}

// neighbors returns the position bounds for inserting at targetIndex within an
// ordered sibling set. A nil or past-the-end index appends (after the last); a
// non-positive index prepends (before the first); otherwise it lands between the
// two siblings straddling the index. The empty Position is Between's open bound.
func neighbors(siblings []domain.Position, targetIndex *int) (prev, next domain.Position) {
	if targetIndex == nil || *targetIndex >= len(siblings) {
		if len(siblings) == 0 {
			return "", ""
		}
		return siblings[len(siblings)-1], ""
	}
	if *targetIndex <= 0 {
		return "", siblings[0]
	}
	return siblings[*targetIndex-1], siblings[*targetIndex]
}

// placeColumn computes a position for a column landing at targetIndex among its
// board's columns, renormalizing the board's columns within the open transaction
// and recomputing when the key would exhaust the length cap (design D2).
// excludeColumnID drops the column being moved from the sibling set so the index
// is measured against the others.
func (s *Service) placeColumn(
	ctx context.Context, tx domain.Repository, boardID, excludeColumnID string, targetIndex *int,
) (domain.Position, error) {
	position, err := positionAmong(columnPositions(ctx, tx, boardID, excludeColumnID), targetIndex)
	if err == nil {
		return position, nil
	}
	if !errors.Is(err, domain.ErrPositionExhausted) {
		return "", err
	}

	if renormErr := s.renormalizeColumns(ctx, tx, boardID); renormErr != nil {
		return "", renormErr
	}
	return positionAmong(columnPositions(ctx, tx, boardID, excludeColumnID), targetIndex)
}

// placeCard mirrors placeColumn for a card landing among a column's cards.
func (s *Service) placeCard(
	ctx context.Context, tx domain.Repository, columnID, excludeCardID string, targetIndex *int,
) (domain.Position, error) {
	position, err := positionAmong(cardPositions(ctx, tx, columnID, excludeCardID), targetIndex)
	if err == nil {
		return position, nil
	}
	if !errors.Is(err, domain.ErrPositionExhausted) {
		return "", err
	}

	if renormErr := s.renormalizeCards(ctx, tx, columnID); renormErr != nil {
		return "", renormErr
	}
	return positionAmong(cardPositions(ctx, tx, columnID, excludeCardID), targetIndex)
}

// positionAmong is the shared neighbor → Between step. It is a small closure
// shim so placeColumn and placeCard read identically; the loader func defers the
// repository read so a failed read surfaces here rather than at the call sites.
func positionAmong(load func() ([]domain.Position, error), targetIndex *int) (domain.Position, error) {
	siblings, err := load()
	if err != nil {
		return "", err
	}
	prev, next := neighbors(siblings, targetIndex)
	return domain.Between(prev, next)
}

func columnPositions(
	ctx context.Context, tx domain.Repository, boardID, excludeColumnID string,
) func() ([]domain.Position, error) {
	return func() ([]domain.Position, error) {
		columns, err := tx.ListColumns(ctx, boardID)
		if err != nil {
			return nil, err
		}
		positions := make([]domain.Position, 0, len(columns))
		for _, column := range columns {
			if column.ID == excludeColumnID {
				continue
			}
			positions = append(positions, column.Position)
		}
		return positions, nil
	}
}

func cardPositions(
	ctx context.Context, tx domain.Repository, columnID, excludeCardID string,
) func() ([]domain.Position, error) {
	return func() ([]domain.Position, error) {
		cards, err := tx.ListCardsByColumn(ctx, columnID)
		if err != nil {
			return nil, err
		}
		positions := make([]domain.Position, 0, len(cards))
		for _, card := range cards {
			if card.ID == excludeCardID {
				continue
			}
			positions = append(positions, card.Position)
		}
		return positions, nil
	}
}

// renormalizeColumns rewrites every column of a board to fresh, evenly-spaced
// keys in their current visible order, so the order is unchanged but the keys are
// short again (design D2). The DEFERRABLE unique constraint lets the adapter
// apply all rows in one statement without a transient collision.
func (s *Service) renormalizeColumns(ctx context.Context, tx domain.Repository, boardID string) error {
	columns, err := tx.ListColumns(ctx, boardID)
	if err != nil {
		return err
	}
	fresh, err := domain.Renormalize(len(columns))
	if err != nil {
		return err
	}
	rewrites := make([]domain.ColumnPosition, len(columns))
	for index, column := range columns {
		rewrites[index] = domain.ColumnPosition{ColumnID: column.ID, Position: fresh[index]}
	}
	return tx.RenormalizeColumns(ctx, boardID, rewrites)
}

func (s *Service) renormalizeCards(ctx context.Context, tx domain.Repository, columnID string) error {
	cards, err := tx.ListCardsByColumn(ctx, columnID)
	if err != nil {
		return err
	}
	fresh, err := domain.Renormalize(len(cards))
	if err != nil {
		return err
	}
	rewrites := make([]domain.CardPosition, len(cards))
	for index, card := range cards {
		rewrites[index] = domain.CardPosition{CardID: card.ID, Position: fresh[index]}
	}
	return tx.RenormalizeCards(ctx, columnID, rewrites)
}

// retryOnConflict runs attempt in its own transaction, retrying on ErrConflict up
// to maxConflictRetries. Each retry is a fresh transaction because a Postgres
// unique-violation aborts the current one; the attempt re-reads neighbors and
// recomputes, so the loser of a concurrent move lands just after the winner —
// last-write-wins, with no raw constraint error surfaced (design D3).
func (s *Service) retryOnConflict(ctx context.Context, attempt func(tx domain.Repository) error) error {
	var err error
	for try := 0; try < maxConflictRetries; try++ {
		err = s.repo.InTx(ctx, attempt)
		if !errors.Is(err, domain.ErrConflict) {
			return err
		}
	}
	return err
}
