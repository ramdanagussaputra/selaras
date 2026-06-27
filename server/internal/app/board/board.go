package board

import (
	"context"

	domain "github.com/ramdanaguss/selaras/server/internal/domain/board"
)

// CreateBoard creates a board and records the caller as its owner in the same
// transaction (spec: owner recorded on creation). It returns the new board and a
// Created event (discarded in M3, forwarded by the broadcaster in M4).
func (s *Service) CreateBoard(ctx context.Context, userID, title string) (domain.Board, []domain.Event, error) {
	if err := domain.ValidateTitle(title); err != nil {
		return domain.Board{}, nil, err
	}

	timestamp := s.clock.Now()
	board := domain.Board{
		ID:        s.newID(),
		OwnerID:   userID,
		Title:     title,
		CreatedAt: timestamp,
		UpdatedAt: timestamp,
	}

	err := s.repo.InTx(ctx, func(tx domain.Repository) error {
		if insertErr := tx.InsertBoard(ctx, board); insertErr != nil {
			return insertErr
		}
		return tx.InsertMember(ctx, domain.Membership{
			BoardID: board.ID,
			UserID:  userID,
			Role:    domain.RoleOwner,
			Found:   true,
		})
	})
	if err != nil {
		return domain.Board{}, nil, err
	}

	events := []domain.Event{domain.Created{BoardID: board.ID, OwnerID: userID}}
	return board, events, nil
}

// ListBoards returns exactly the boards the caller is a member of.
func (s *Service) ListBoards(ctx context.Context, userID string) ([]domain.Board, error) {
	return s.repo.ListBoards(ctx, userID)
}

// GetBoard returns a board with its columns and cards nested and ordered by
// position. It runs three ordered reads and assembles the tree in Go rather than
// an N+1 walk or a json_agg blob (design D6); access is member-gated.
func (s *Service) GetBoard(ctx context.Context, userID, boardID string) (Tree, error) {
	membership, err := s.repo.MembershipByBoard(ctx, boardID, userID)
	if err != nil {
		return Tree{}, err
	}
	if err := authorize(membership, false); err != nil {
		return Tree{}, err
	}

	board, err := s.repo.GetBoard(ctx, boardID)
	if err != nil {
		return Tree{}, err
	}
	columns, err := s.repo.ListColumns(ctx, boardID)
	if err != nil {
		return Tree{}, err
	}
	cards, err := s.repo.ListCardsByBoard(ctx, boardID)
	if err != nil {
		return Tree{}, err
	}

	return assembleTree(board, columns, cards), nil
}

// assembleTree groups the board's cards under their columns, preserving the
// position order the repository read them in. Both inputs arrive ordered, so a
// single pass per column suffices.
func assembleTree(board domain.Board, columns []domain.Column, cards []domain.Card) Tree {
	cardsByColumn := make(map[string][]domain.Card, len(columns))
	for _, card := range cards {
		cardsByColumn[card.ColumnID] = append(cardsByColumn[card.ColumnID], card)
	}

	withCards := make([]ColumnWithCards, len(columns))
	for index, column := range columns {
		withCards[index] = ColumnWithCards{Column: column, Cards: cardsByColumn[column.ID]}
	}
	return Tree{Board: board, Columns: withCards}
}

// RenameBoard updates a board's title. Any member may rename (spec).
func (s *Service) RenameBoard(ctx context.Context, userID, boardID, title string) ([]domain.Event, error) {
	if err := domain.ValidateTitle(title); err != nil {
		return nil, err
	}

	membership, err := s.repo.MembershipByBoard(ctx, boardID, userID)
	if err != nil {
		return nil, err
	}
	if err := authorize(membership, false); err != nil {
		return nil, err
	}

	if err := s.repo.UpdateBoardTitle(ctx, boardID, title, s.clock.Now()); err != nil {
		return nil, err
	}
	return []domain.Event{domain.Updated{BoardID: boardID}}, nil
}

// DeleteBoard deletes a board and cascades its columns, cards, and membership
// rows in the database. Deletion is owner-only (spec).
func (s *Service) DeleteBoard(ctx context.Context, userID, boardID string) ([]domain.Event, error) {
	membership, err := s.repo.MembershipByBoard(ctx, boardID, userID)
	if err != nil {
		return nil, err
	}
	if err := authorize(membership, true); err != nil {
		return nil, err
	}

	if err := s.repo.DeleteBoard(ctx, boardID); err != nil {
		return nil, err
	}
	return []domain.Event{domain.Deleted{BoardID: boardID}}, nil
}
