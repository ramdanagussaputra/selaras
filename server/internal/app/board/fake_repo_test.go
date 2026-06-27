package board_test

import (
	"context"
	"sort"
	"time"

	domain "github.com/ramdanaguss/selaras/server/internal/domain/board"
)

// fakeClock returns a fixed instant so created/updated timestamps are stable.
type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

// fakeRepo is an in-memory Repository for the use-case tests. It enforces
// the (parent, position) uniqueness that the real schema does — so the conflict
// and validation paths are exercised without a database — and can inject a bounded
// number of artificial conflicts to drive the retry loop deterministically.
type fakeRepo struct {
	boards          map[string]domain.Board
	members         map[string]map[string]domain.Membership // boardID -> userID -> membership
	columns         map[string]domain.Column
	cards           map[string]domain.Card
	injectConflicts int // position writes return ErrConflict while > 0
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		boards:  map[string]domain.Board{},
		members: map[string]map[string]domain.Membership{},
		columns: map[string]domain.Column{},
		cards:   map[string]domain.Card{},
	}
}

// InTx runs fn against the same repo, restoring a snapshot on error so a failed
// attempt leaves no partial writes — mirroring a real transaction's rollback.
func (r *fakeRepo) InTx(_ context.Context, fn func(repo domain.Repository) error) error {
	snapshot := r.clone()
	if err := fn(r); err != nil {
		r.restore(snapshot)
		return err
	}
	return nil
}

func (r *fakeRepo) clone() *fakeRepo {
	other := newFakeRepo()
	for id, board := range r.boards {
		other.boards[id] = board
	}
	for boardID, byUser := range r.members {
		other.members[boardID] = map[string]domain.Membership{}
		for userID, membership := range byUser {
			other.members[boardID][userID] = membership
		}
	}
	for id, column := range r.columns {
		other.columns[id] = column
	}
	for id, card := range r.cards {
		other.cards[id] = card
	}
	other.injectConflicts = r.injectConflicts
	return other
}

func (r *fakeRepo) restore(snapshot *fakeRepo) {
	r.boards = snapshot.boards
	r.members = snapshot.members
	r.columns = snapshot.columns
	r.cards = snapshot.cards
	// injectConflicts is intentionally not restored: an injected conflict is
	// "consumed" so the next attempt can succeed.
}

func (r *fakeRepo) membership(boardID, userID string) domain.Membership {
	if byUser, ok := r.members[boardID]; ok {
		if membership, ok := byUser[userID]; ok {
			return membership
		}
	}
	return domain.Membership{Found: false}
}

func (r *fakeRepo) MembershipByBoard(_ context.Context, boardID, userID string) (domain.Membership, error) {
	if _, ok := r.boards[boardID]; !ok {
		return domain.Membership{Found: false}, nil
	}
	return r.membership(boardID, userID), nil
}

func (r *fakeRepo) MembershipByColumn(_ context.Context, columnID, userID string) (domain.Membership, error) {
	column, ok := r.columns[columnID]
	if !ok {
		return domain.Membership{Found: false}, nil
	}
	return r.membership(column.BoardID, userID), nil
}

func (r *fakeRepo) MembershipByCard(_ context.Context, cardID, userID string) (domain.Membership, error) {
	card, ok := r.cards[cardID]
	if !ok {
		return domain.Membership{Found: false}, nil
	}
	column, ok := r.columns[card.ColumnID]
	if !ok {
		return domain.Membership{Found: false}, nil
	}
	return r.membership(column.BoardID, userID), nil
}

func (r *fakeRepo) InsertBoard(_ context.Context, board domain.Board) error {
	r.boards[board.ID] = board
	return nil
}

func (r *fakeRepo) InsertMember(_ context.Context, membership domain.Membership) error {
	if r.members[membership.BoardID] == nil {
		r.members[membership.BoardID] = map[string]domain.Membership{}
	}
	r.members[membership.BoardID][membership.UserID] = membership
	return nil
}

func (r *fakeRepo) ListBoards(_ context.Context, userID string) ([]domain.Board, error) {
	var boards []domain.Board
	for boardID, byUser := range r.members {
		if _, ok := byUser[userID]; ok {
			boards = append(boards, r.boards[boardID])
		}
	}
	sort.Slice(boards, func(i, j int) bool { return boards[i].ID < boards[j].ID })
	return boards, nil
}

func (r *fakeRepo) GetBoard(_ context.Context, boardID string) (domain.Board, error) {
	board, ok := r.boards[boardID]
	if !ok {
		return domain.Board{}, domain.ErrNotFound
	}
	return board, nil
}

func (r *fakeRepo) UpdateBoardTitle(_ context.Context, boardID, title string, updatedAt time.Time) error {
	board, ok := r.boards[boardID]
	if !ok {
		return domain.ErrNotFound
	}
	board.Title = title
	board.UpdatedAt = updatedAt
	r.boards[boardID] = board
	return nil
}

func (r *fakeRepo) DeleteBoard(_ context.Context, boardID string) error {
	delete(r.boards, boardID)
	delete(r.members, boardID)
	for id, column := range r.columns {
		if column.BoardID == boardID {
			r.deleteColumnCascade(id)
		}
	}
	return nil
}

func (r *fakeRepo) deleteColumnCascade(columnID string) {
	delete(r.columns, columnID)
	for id, card := range r.cards {
		if card.ColumnID == columnID {
			delete(r.cards, id)
		}
	}
}

func (r *fakeRepo) GetColumn(_ context.Context, columnID string) (domain.Column, error) {
	column, ok := r.columns[columnID]
	if !ok {
		return domain.Column{}, domain.ErrNotFound
	}
	return column, nil
}

func (r *fakeRepo) ListColumns(_ context.Context, boardID string) ([]domain.Column, error) {
	var columns []domain.Column
	for _, column := range r.columns {
		if column.BoardID == boardID {
			columns = append(columns, column)
		}
	}
	sort.Slice(columns, func(i, j int) bool { return columns[i].Position < columns[j].Position })
	return columns, nil
}

func (r *fakeRepo) columnPositionTaken(boardID, columnID string, position domain.Position) bool {
	for id, column := range r.columns {
		if column.BoardID == boardID && id != columnID && column.Position == position {
			return true
		}
	}
	return false
}

func (r *fakeRepo) InsertColumn(_ context.Context, column domain.Column) error {
	if r.takeInjectedConflict() {
		return domain.ErrConflict
	}
	if r.columnPositionTaken(column.BoardID, column.ID, column.Position) {
		return domain.ErrConflict
	}
	r.columns[column.ID] = column
	return nil
}

func (r *fakeRepo) UpdateColumnTitle(_ context.Context, columnID, title string) error {
	column, ok := r.columns[columnID]
	if !ok {
		return domain.ErrNotFound
	}
	column.Title = title
	r.columns[columnID] = column
	return nil
}

func (r *fakeRepo) UpdateColumnPosition(_ context.Context, columnID string, position domain.Position) error {
	if r.takeInjectedConflict() {
		return domain.ErrConflict
	}
	column, ok := r.columns[columnID]
	if !ok {
		return domain.ErrNotFound
	}
	if r.columnPositionTaken(column.BoardID, columnID, position) {
		return domain.ErrConflict
	}
	column.Position = position
	r.columns[columnID] = column
	return nil
}

func (r *fakeRepo) RenormalizeColumns(_ context.Context, _ string, positions []domain.ColumnPosition) error {
	// Deferred-constraint semantics: apply all rewrites atomically with no
	// per-row uniqueness check (the real adapter sets constraints DEFERRED).
	for _, rewrite := range positions {
		column := r.columns[rewrite.ColumnID]
		column.Position = rewrite.Position
		r.columns[rewrite.ColumnID] = column
	}
	return nil
}

func (r *fakeRepo) DeleteColumn(_ context.Context, columnID string) error {
	r.deleteColumnCascade(columnID)
	return nil
}

func (r *fakeRepo) GetCard(_ context.Context, cardID string) (domain.Card, error) {
	card, ok := r.cards[cardID]
	if !ok {
		return domain.Card{}, domain.ErrNotFound
	}
	return card, nil
}

func (r *fakeRepo) ListCardsByColumn(_ context.Context, columnID string) ([]domain.Card, error) {
	var cards []domain.Card
	for _, card := range r.cards {
		if card.ColumnID == columnID {
			cards = append(cards, card)
		}
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].Position < cards[j].Position })
	return cards, nil
}

func (r *fakeRepo) ListCardsByBoard(_ context.Context, boardID string) ([]domain.Card, error) {
	columnsInBoard := map[string]bool{}
	for id, column := range r.columns {
		if column.BoardID == boardID {
			columnsInBoard[id] = true
		}
	}
	var cards []domain.Card
	for _, card := range r.cards {
		if columnsInBoard[card.ColumnID] {
			cards = append(cards, card)
		}
	}
	sort.Slice(cards, func(i, j int) bool {
		if cards[i].ColumnID != cards[j].ColumnID {
			return cards[i].ColumnID < cards[j].ColumnID
		}
		return cards[i].Position < cards[j].Position
	})
	return cards, nil
}

func (r *fakeRepo) cardPositionTaken(columnID, cardID string, position domain.Position) bool {
	for id, card := range r.cards {
		if card.ColumnID == columnID && id != cardID && card.Position == position {
			return true
		}
	}
	return false
}

func (r *fakeRepo) InsertCard(_ context.Context, card domain.Card) error {
	if r.takeInjectedConflict() {
		return domain.ErrConflict
	}
	if r.cardPositionTaken(card.ColumnID, card.ID, card.Position) {
		return domain.ErrConflict
	}
	r.cards[card.ID] = card
	return nil
}

func (r *fakeRepo) UpdateCardContent(_ context.Context, cardID, title, description string, updatedAt time.Time) error {
	card, ok := r.cards[cardID]
	if !ok {
		return domain.ErrNotFound
	}
	card.Title = title
	card.Description = description
	card.UpdatedAt = updatedAt
	r.cards[cardID] = card
	return nil
}

func (r *fakeRepo) UpdateCardPosition(_ context.Context, cardID, columnID string, position domain.Position) error {
	if r.takeInjectedConflict() {
		return domain.ErrConflict
	}
	card, ok := r.cards[cardID]
	if !ok {
		return domain.ErrNotFound
	}
	if r.cardPositionTaken(columnID, cardID, position) {
		return domain.ErrConflict
	}
	card.ColumnID = columnID
	card.Position = position
	r.cards[cardID] = card
	return nil
}

func (r *fakeRepo) RenormalizeCards(_ context.Context, _ string, positions []domain.CardPosition) error {
	for _, rewrite := range positions {
		card := r.cards[rewrite.CardID]
		card.Position = rewrite.Position
		r.cards[rewrite.CardID] = card
	}
	return nil
}

func (r *fakeRepo) DeleteCard(_ context.Context, cardID string) error {
	delete(r.cards, cardID)
	return nil
}

func (r *fakeRepo) takeInjectedConflict() bool {
	if r.injectConflicts > 0 {
		r.injectConflicts--
		return true
	}
	return false
}
