package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramdanaguss/selaras/server/internal/adapter/postgres/sqlcgen"
	domain "github.com/ramdanaguss/selaras/server/internal/domain/board"
)

// BoardRepository implements domain/board.Repository over sqlc-generated queries.
// It owns the transaction boundary: InTx begins a pgx transaction and hands the
// use case a tx-scoped repository, so the use cases stay free of pgx (design D5).
type BoardRepository struct {
	pool    *pgxpool.Pool // nil on a tx-scoped repository; only the root begins transactions
	db      sqlcgen.DBTX  // the pool or the open transaction; backs queries and raw Exec
	queries *sqlcgen.Queries
}

// Compile-time proof the adapter satisfies the port it implements.
var _ domain.Repository = (*BoardRepository)(nil)

// NewBoardRepository wraps a pool as a board repository.
func NewBoardRepository(pool *pgxpool.Pool) *BoardRepository {
	return &BoardRepository{pool: pool, db: pool, queries: sqlcgen.New(pool)}
}

// InTx runs fn inside a transaction. A root repository begins one and commits on
// success or rolls back on error; a tx-scoped repository simply joins the open
// transaction so nested calls don't deadlock on a new connection.
func (r *BoardRepository) InTx(ctx context.Context, operation func(repo domain.Repository) error) error {
	if r.pool == nil {
		return operation(r)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	scoped := &BoardRepository{db: tx, queries: sqlcgen.New(tx)}
	if err := operation(scoped); err != nil {
		_ = tx.Rollback(ctx) // the original error is what matters; rollback errors are moot
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

// --- authorization -----------------------------------------------------------

// MembershipByBoard resolves the caller's membership on a board directly.
func (r *BoardRepository) MembershipByBoard(ctx context.Context, boardID, userID string) (domain.Membership, error) {
	row, err := r.queries.MembershipByBoard(ctx, sqlcgen.MembershipByBoardParams{BoardID: boardID, UserID: userID})
	return membershipFrom(row.BoardID, row.Role, userID, err)
}

// MembershipByColumn resolves the caller's membership via the column's board.
func (r *BoardRepository) MembershipByColumn(ctx context.Context, columnID, userID string) (domain.Membership, error) {
	row, err := r.queries.MembershipByColumn(ctx, sqlcgen.MembershipByColumnParams{ColumnID: columnID, UserID: userID})
	return membershipFrom(row.BoardID, row.Role, userID, err)
}

// MembershipByCard resolves the caller's membership via the card's column and board.
func (r *BoardRepository) MembershipByCard(ctx context.Context, cardID, userID string) (domain.Membership, error) {
	row, err := r.queries.MembershipByCard(ctx, sqlcgen.MembershipByCardParams{CardID: cardID, UserID: userID})
	return membershipFrom(row.BoardID, row.Role, userID, err)
}

// membershipFrom turns a membership query result into a domain.Membership. A
// missing row (the leaf doesn't exist, or the caller isn't a member) is reported
// as Found=false rather than an error, so the use case answers 404 either way.
func membershipFrom(boardID, role, userID string, err error) (domain.Membership, error) {
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Membership{Found: false}, nil
		}
		return domain.Membership{}, fmt.Errorf("resolving membership: %w", err)
	}
	return domain.Membership{BoardID: boardID, UserID: userID, Role: domain.Role(role), Found: true}, nil
}

// --- boards ------------------------------------------------------------------

// InsertBoard persists a new board.
func (r *BoardRepository) InsertBoard(ctx context.Context, board domain.Board) error {
	return r.queries.InsertBoard(ctx, sqlcgen.InsertBoardParams{
		ID:        board.ID,
		OwnerID:   board.OwnerID,
		Title:     board.Title,
		CreatedAt: board.CreatedAt,
		UpdatedAt: board.UpdatedAt,
	})
}

// InsertMember persists a board_members row (created_at defaults in the DB).
func (r *BoardRepository) InsertMember(ctx context.Context, membership domain.Membership) error {
	return r.queries.InsertMember(ctx, sqlcgen.InsertMemberParams{
		BoardID: membership.BoardID,
		UserID:  membership.UserID,
		Role:    string(membership.Role),
	})
}

// ListBoards returns the boards the user is a member of, oldest first.
func (r *BoardRepository) ListBoards(ctx context.Context, userID string) ([]domain.Board, error) {
	rows, err := r.queries.ListBoardsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing boards: %w", err)
	}
	boards := make([]domain.Board, len(rows))
	for index, row := range rows {
		boards[index] = domain.Board{
			ID: row.ID, OwnerID: row.OwnerID, Title: row.Title,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
	}
	return boards, nil
}

// GetBoard returns one board, or domain.ErrNotFound.
func (r *BoardRepository) GetBoard(ctx context.Context, boardID string) (domain.Board, error) {
	record, err := r.queries.GetBoard(ctx, boardID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Board{}, domain.ErrNotFound
		}
		return domain.Board{}, fmt.Errorf("getting board: %w", err)
	}
	return domain.Board{
		ID: record.ID, OwnerID: record.OwnerID, Title: record.Title,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}, nil
}

// UpdateBoardTitle renames a board and stamps its updated_at.
func (r *BoardRepository) UpdateBoardTitle(ctx context.Context, boardID, title string, updatedAt time.Time) error {
	return r.queries.UpdateBoardTitle(ctx, sqlcgen.UpdateBoardTitleParams{ID: boardID, Title: title, UpdatedAt: updatedAt})
}

// DeleteBoard removes a board; the DB cascades its members, columns, and cards.
func (r *BoardRepository) DeleteBoard(ctx context.Context, boardID string) error {
	return r.queries.DeleteBoard(ctx, boardID)
}

// --- columns -----------------------------------------------------------------

// GetColumn returns one column, or domain.ErrNotFound.
func (r *BoardRepository) GetColumn(ctx context.Context, columnID string) (domain.Column, error) {
	record, err := r.queries.GetColumn(ctx, columnID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Column{}, domain.ErrNotFound
		}
		return domain.Column{}, fmt.Errorf("getting column: %w", err)
	}
	return columnFrom(record.ID, record.BoardID, record.Title, record.Position, record.CreatedAt), nil
}

// ListColumns returns a board's columns ordered by position.
func (r *BoardRepository) ListColumns(ctx context.Context, boardID string) ([]domain.Column, error) {
	rows, err := r.queries.ListColumns(ctx, boardID)
	if err != nil {
		return nil, fmt.Errorf("listing columns: %w", err)
	}
	columns := make([]domain.Column, len(rows))
	for index, row := range rows {
		columns[index] = columnFrom(row.ID, row.BoardID, row.Title, row.Position, row.CreatedAt)
	}
	return columns, nil
}

// InsertColumn persists a new column, mapping a position clash to ErrConflict.
func (r *BoardRepository) InsertColumn(ctx context.Context, column domain.Column) error {
	err := r.queries.InsertColumn(ctx, sqlcgen.InsertColumnParams{
		ID:        column.ID,
		BoardID:   column.BoardID,
		Title:     column.Title,
		Position:  string(column.Position),
		CreatedAt: column.CreatedAt,
	})
	return mapConflict(err)
}

// UpdateColumnTitle renames a column.
func (r *BoardRepository) UpdateColumnTitle(ctx context.Context, columnID, title string) error {
	return r.queries.UpdateColumnTitle(ctx, sqlcgen.UpdateColumnTitleParams{ID: columnID, Title: title})
}

// UpdateColumnPosition reorders a column as a single-row update.
func (r *BoardRepository) UpdateColumnPosition(ctx context.Context, columnID string, position domain.Position) error {
	err := r.queries.UpdateColumnPosition(ctx, sqlcgen.UpdateColumnPositionParams{ID: columnID, Position: string(position)})
	return mapConflict(err)
}

// RenormalizeColumns rewrites a board's column positions under deferred constraints.
func (r *BoardRepository) RenormalizeColumns(ctx context.Context, _ string, positions []domain.ColumnPosition) error {
	if err := r.deferConstraints(ctx); err != nil {
		return err
	}
	for _, rewrite := range positions {
		if err := r.queries.UpdateColumnPosition(ctx, sqlcgen.UpdateColumnPositionParams{
			ID: rewrite.ColumnID, Position: string(rewrite.Position),
		}); err != nil {
			return fmt.Errorf("renormalizing column %s: %w", rewrite.ColumnID, err)
		}
	}
	return nil
}

// DeleteColumn removes a column; the DB cascades its cards.
func (r *BoardRepository) DeleteColumn(ctx context.Context, columnID string) error {
	return r.queries.DeleteColumn(ctx, columnID)
}

// --- cards -------------------------------------------------------------------

// GetCard returns one card, or domain.ErrNotFound.
func (r *BoardRepository) GetCard(ctx context.Context, cardID string) (domain.Card, error) {
	record, err := r.queries.GetCard(ctx, cardID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Card{}, domain.ErrNotFound
		}
		return domain.Card{}, fmt.Errorf("getting card: %w", err)
	}
	return cardFrom(record.ID, record.ColumnID, record.Title, record.Description, record.Position, record.CreatedAt, record.UpdatedAt), nil
}

// ListCardsByColumn returns a column's cards ordered by position.
func (r *BoardRepository) ListCardsByColumn(ctx context.Context, columnID string) ([]domain.Card, error) {
	rows, err := r.queries.ListCardsByColumn(ctx, columnID)
	if err != nil {
		return nil, fmt.Errorf("listing cards: %w", err)
	}
	cards := make([]domain.Card, len(rows))
	for index, row := range rows {
		cards[index] = cardFrom(row.ID, row.ColumnID, row.Title, row.Description, row.Position, row.CreatedAt, row.UpdatedAt)
	}
	return cards, nil
}

// ListCardsByBoard returns every card under a board, ordered by (column, position).
func (r *BoardRepository) ListCardsByBoard(ctx context.Context, boardID string) ([]domain.Card, error) {
	rows, err := r.queries.ListCardsByBoard(ctx, boardID)
	if err != nil {
		return nil, fmt.Errorf("listing board cards: %w", err)
	}
	cards := make([]domain.Card, len(rows))
	for index, row := range rows {
		cards[index] = cardFrom(row.ID, row.ColumnID, row.Title, row.Description, row.Position, row.CreatedAt, row.UpdatedAt)
	}
	return cards, nil
}

// InsertCard persists a new card, mapping a position clash to ErrConflict.
func (r *BoardRepository) InsertCard(ctx context.Context, card domain.Card) error {
	err := r.queries.InsertCard(ctx, sqlcgen.InsertCardParams{
		ID:          card.ID,
		ColumnID:    card.ColumnID,
		Title:       card.Title,
		Description: card.Description,
		Position:    string(card.Position),
		CreatedAt:   card.CreatedAt,
		UpdatedAt:   card.UpdatedAt,
	})
	return mapConflict(err)
}

// UpdateCardContent edits a card's title and description.
func (r *BoardRepository) UpdateCardContent(ctx context.Context, cardID, title, description string, updatedAt time.Time) error {
	return r.queries.UpdateCardContent(ctx, sqlcgen.UpdateCardContentParams{
		ID: cardID, Title: title, Description: description, UpdatedAt: updatedAt,
	})
}

// UpdateCardPosition moves a card (column + position) as a single-row update.
func (r *BoardRepository) UpdateCardPosition(ctx context.Context, cardID, columnID string, position domain.Position) error {
	err := r.queries.UpdateCardPosition(ctx, sqlcgen.UpdateCardPositionParams{
		ID: cardID, ColumnID: columnID, Position: string(position),
	})
	return mapConflict(err)
}

// RenormalizeCards rewrites a column's card positions under deferred constraints.
func (r *BoardRepository) RenormalizeCards(ctx context.Context, columnID string, positions []domain.CardPosition) error {
	if err := r.deferConstraints(ctx); err != nil {
		return err
	}
	for _, rewrite := range positions {
		if err := r.queries.UpdateCardPosition(ctx, sqlcgen.UpdateCardPositionParams{
			ID: rewrite.CardID, ColumnID: columnID, Position: string(rewrite.Position),
		}); err != nil {
			return fmt.Errorf("renormalizing card %s: %w", rewrite.CardID, err)
		}
	}
	return nil
}

// DeleteCard removes a card.
func (r *BoardRepository) DeleteCard(ctx context.Context, cardID string) error {
	return r.queries.DeleteCard(ctx, cardID)
}

// deferConstraints defers the DEFERRABLE unique checks to commit time, so a
// renormalization can rewrite every sibling's position without a transient
// mid-rewrite collision (design D2). It is a no-op outside a transaction, but
// renormalization always runs inside one.
func (r *BoardRepository) deferConstraints(ctx context.Context) error {
	if _, err := r.db.Exec(ctx, "SET CONSTRAINTS ALL DEFERRED"); err != nil {
		return fmt.Errorf("deferring constraints: %w", err)
	}
	return nil
}

// mapConflict translates a Postgres unique-violation (the (parent, position)
// backstop) into domain.ErrConflict so the use case can retry (design D3).
func mapConflict(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return domain.ErrConflict
	}
	return fmt.Errorf("position write: %w", err)
}

func columnFrom(id, boardID, title, position string, createdAt time.Time) domain.Column {
	return domain.Column{
		ID: id, BoardID: boardID, Title: title,
		Position: domain.Position(position), CreatedAt: createdAt,
	}
}

func cardFrom(id, columnID, title, description, position string, createdAt, updatedAt time.Time) domain.Card {
	return domain.Card{
		ID: id, ColumnID: columnID, Title: title, Description: description,
		Position: domain.Position(position), CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
}
