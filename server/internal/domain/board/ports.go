package board

import (
	"context"
	"time"
)

// CardPosition pairs a card with a new position, used to rewrite a column's
// cards during renormalization (design D2).
type CardPosition struct {
	CardID   string
	Position Position
}

// ColumnPosition pairs a column with a new position, used to rewrite a board's
// columns during renormalization (design D2).
type ColumnPosition struct {
	ColumnID string
	Position Position
}

// Repository is the driven port the board use cases depend on (pgx + sqlc
// adapter). It exposes the reads authorization and the nested board view need,
// the single-row writes that make a move O(1), and a unit-of-work (InTx) so a
// move's read-recompute-write-and-retry runs atomically (design D2, D3, D5).
//
// Position-write methods (InsertColumn, InsertCard, the Update*Position methods)
// MUST translate a Postgres unique-violation on the (parent, position) constraint
// into ErrConflict so the use case can retry; all reads return ErrNotFound when
// the row is absent. Membership lookups resolve the owning board from a board,
// column, or card id and report Found=false for both "no such row" and
// "caller is not a member", so the HTTP layer can answer 404 without leaking
// existence (design D7).
type Repository interface {
	// InTx runs fn against a transaction-scoped repository, committing on nil and
	// rolling back on error. The scoped repository joins the open transaction, so
	// calls to its InTx run inline rather than opening a nested transaction.
	InTx(ctx context.Context, fn func(repo Repository) error) error

	// --- authorization (resolve membership from the leaf id) ---
	MembershipByBoard(ctx context.Context, boardID, userID string) (Membership, error)
	MembershipByColumn(ctx context.Context, columnID, userID string) (Membership, error)
	MembershipByCard(ctx context.Context, cardID, userID string) (Membership, error)

	// --- boards ---
	InsertBoard(ctx context.Context, board Board) error
	InsertMember(ctx context.Context, membership Membership) error
	ListBoards(ctx context.Context, userID string) ([]Board, error)
	GetBoard(ctx context.Context, boardID string) (Board, error)
	UpdateBoardTitle(ctx context.Context, boardID, title string, updatedAt time.Time) error
	DeleteBoard(ctx context.Context, boardID string) error

	// --- columns ---
	GetColumn(ctx context.Context, columnID string) (Column, error)
	ListColumns(ctx context.Context, boardID string) ([]Column, error)
	InsertColumn(ctx context.Context, column Column) error
	UpdateColumnTitle(ctx context.Context, columnID, title string) error
	UpdateColumnPosition(ctx context.Context, columnID string, position Position) error
	RenormalizeColumns(ctx context.Context, boardID string, positions []ColumnPosition) error
	DeleteColumn(ctx context.Context, columnID string) error

	// --- cards ---
	GetCard(ctx context.Context, cardID string) (Card, error)
	ListCardsByColumn(ctx context.Context, columnID string) ([]Card, error)
	ListCardsByBoard(ctx context.Context, boardID string) ([]Card, error)
	InsertCard(ctx context.Context, card Card) error
	UpdateCardContent(ctx context.Context, cardID, title, description string, updatedAt time.Time) error
	UpdateCardPosition(ctx context.Context, cardID, columnID string, position Position) error
	RenormalizeCards(ctx context.Context, columnID string, positions []CardPosition) error
	DeleteCard(ctx context.Context, cardID string) error
}
