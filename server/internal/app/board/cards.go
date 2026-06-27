package board

import (
	"context"

	domain "github.com/ramdanaguss/selaras/server/internal/domain/board"
)

// CreateCard adds a card to a column at targetIndex, appending when nil. The
// description defaults to the empty string (spec). Member-gated, with conflict
// retry on the placement.
func (s *Service) CreateCard(
	ctx context.Context, userID, columnID, title string, targetIndex *int,
) (domain.Card, []domain.Event, error) {
	if err := domain.ValidateTitle(title); err != nil {
		return domain.Card{}, nil, err
	}

	membership, err := s.repo.MembershipByColumn(ctx, columnID, userID)
	if err != nil {
		return domain.Card{}, nil, err
	}
	if err := authorize(membership, false); err != nil {
		return domain.Card{}, nil, err
	}

	timestamp := s.clock.Now()
	card := domain.Card{
		ID:        s.newID(),
		ColumnID:  columnID,
		Title:     title,
		CreatedAt: timestamp,
		UpdatedAt: timestamp,
	}

	err = s.retryOnConflict(ctx, func(tx domain.Repository) error {
		position, placeErr := s.placeCard(ctx, tx, columnID, card.ID, targetIndex)
		if placeErr != nil {
			return placeErr
		}
		card.Position = position
		return tx.InsertCard(ctx, card)
	})
	if err != nil {
		return domain.Card{}, nil, err
	}

	events := []domain.Event{domain.CardCreated{ColumnID: columnID, CardID: card.ID, Position: card.Position}}
	return card, events, nil
}

// EditCard updates a card's title and/or description (spec: PATCH with
// {title?, description?}). A nil field is left unchanged. Member-gated.
func (s *Service) EditCard(
	ctx context.Context, userID, cardID string, title, description *string,
) ([]domain.Event, error) {
	membership, err := s.repo.MembershipByCard(ctx, cardID, userID)
	if err != nil {
		return nil, err
	}
	if err := authorize(membership, false); err != nil {
		return nil, err
	}

	card, err := s.repo.GetCard(ctx, cardID)
	if err != nil {
		return nil, err
	}

	newTitle := card.Title
	if title != nil {
		newTitle = *title
	}
	newDescription := card.Description
	if description != nil {
		newDescription = *description
	}

	if err := domain.ValidateTitle(newTitle); err != nil {
		return nil, err
	}
	if err := domain.ValidateDescription(newDescription); err != nil {
		return nil, err
	}

	if err := s.repo.UpdateCardContent(ctx, cardID, newTitle, newDescription, s.clock.Now()); err != nil {
		return nil, err
	}
	return []domain.Event{domain.CardUpdated{CardID: cardID}}, nil
}

// MoveCard moves a card within or across columns to targetIndex, as a single-row
// update of the card's column and position (spec, AC-1). When the destination is
// a different column it must also be one the caller is a member of. The placement
// retries on a concurrent-move conflict so two clients moving the same card
// converge last-write-wins (design D3).
func (s *Service) MoveCard(
	ctx context.Context, userID, cardID string, targetColumnID *string, targetIndex *int,
) ([]domain.Event, error) {
	membership, err := s.repo.MembershipByCard(ctx, cardID, userID)
	if err != nil {
		return nil, err
	}
	if err := authorize(membership, false); err != nil {
		return nil, err
	}

	card, err := s.repo.GetCard(ctx, cardID)
	if err != nil {
		return nil, err
	}

	destinationColumnID := card.ColumnID
	if targetColumnID != nil && *targetColumnID != card.ColumnID {
		destinationColumnID = *targetColumnID
		// A cross-column move must land in a column the caller can also reach,
		// or it leaks one board's cards into another (design D7).
		destinationMembership, membershipErr := s.repo.MembershipByColumn(ctx, destinationColumnID, userID)
		if membershipErr != nil {
			return nil, membershipErr
		}
		if err := authorize(destinationMembership, false); err != nil {
			return nil, err
		}
	}

	var newPosition domain.Position
	err = s.retryOnConflict(ctx, func(tx domain.Repository) error {
		position, placeErr := s.placeCard(ctx, tx, destinationColumnID, cardID, targetIndex)
		if placeErr != nil {
			return placeErr
		}
		newPosition = position
		return tx.UpdateCardPosition(ctx, cardID, destinationColumnID, position)
	})
	if err != nil {
		return nil, err
	}

	return []domain.Event{domain.CardMoved{
		CardID:       cardID,
		FromColumnID: card.ColumnID,
		ToColumnID:   destinationColumnID,
		Position:     newPosition,
	}}, nil
}

// DeleteCard deletes a card. Member-gated.
func (s *Service) DeleteCard(ctx context.Context, userID, cardID string) ([]domain.Event, error) {
	membership, err := s.repo.MembershipByCard(ctx, cardID, userID)
	if err != nil {
		return nil, err
	}
	if err := authorize(membership, false); err != nil {
		return nil, err
	}

	if err := s.repo.DeleteCard(ctx, cardID); err != nil {
		return nil, err
	}
	return []domain.Event{domain.CardDeleted{CardID: cardID}}, nil
}
