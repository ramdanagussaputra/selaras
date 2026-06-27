package board_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appboard "github.com/ramdanaguss/selaras/server/internal/app/board"
	domain "github.com/ramdanaguss/selaras/server/internal/domain/board"
)

const (
	owner   = "user-owner"
	member  = "user-member"
	outside = "user-outsider"
)

func newService(repo *fakeRepo) *appboard.Service {
	return appboard.NewService(repo, fakeClock{now: time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)})
}

// seedBoard creates a board owned by `owner` with one column, returning their ids.
func seedBoard(t *testing.T, repo *fakeRepo) (boardID, columnID string) {
	t.Helper()
	boardID, columnID = "board-1", "column-1"
	repo.boards[boardID] = domain.Board{ID: boardID, OwnerID: owner, Title: "Sprint"}
	repo.members[boardID] = map[string]domain.Membership{
		owner: {BoardID: boardID, UserID: owner, Role: domain.RoleOwner, Found: true},
	}
	repo.columns[columnID] = domain.Column{ID: columnID, BoardID: boardID, Title: "Todo", Position: "V"}
	return boardID, columnID
}

func ctx() context.Context { return context.Background() }

func TestCreateBoardRecordsOwner(t *testing.T) {
	repo := newFakeRepo()
	service := newService(repo)

	board, events, err := service.CreateBoard(ctx(), owner, "Roadmap")
	if err != nil {
		t.Fatalf("CreateBoard: %v", err)
	}

	membership := repo.membership(board.ID, owner)
	if !membership.Found || membership.Role != domain.RoleOwner {
		t.Errorf("owner membership = %+v, want found owner", membership)
	}
	if len(events) != 1 {
		t.Fatalf("events = %v, want one Created", events)
	}
	if created, ok := events[0].(domain.Created); !ok || created.BoardID != board.ID {
		t.Errorf("event = %#v, want Created for %s", events[0], board.ID)
	}

	boards, err := service.ListBoards(ctx(), owner)
	if err != nil || len(boards) != 1 || boards[0].ID != board.ID {
		t.Errorf("ListBoards = %v, %v; want the created board", boards, err)
	}
}

func TestCreateBoardRejectsInvalidTitle(t *testing.T) {
	service := newService(newFakeRepo())
	for _, title := range []string{"", "   ", strings.Repeat("x", 201)} {
		if _, _, err := service.CreateBoard(ctx(), owner, title); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("CreateBoard(%q) err = %v, want ErrValidation", title, err)
		}
	}
}

func TestAuthorization(t *testing.T) {
	tests := []struct {
		name    string
		action  func(service *appboard.Service, boardID string) error
		userID  string
		wantErr error
	}{
		{
			name: "non-member read is 404",
			action: func(service *appboard.Service, boardID string) error {
				_, err := service.GetBoard(ctx(), outside, boardID)
				return err
			},
			wantErr: domain.ErrNotFound,
		},
		{
			name: "non-owner member delete is 403",
			action: func(service *appboard.Service, boardID string) error {
				_, err := service.DeleteBoard(ctx(), member, boardID)
				return err
			},
			wantErr: domain.ErrForbidden,
		},
		{
			name: "owner delete succeeds",
			action: func(service *appboard.Service, boardID string) error {
				_, err := service.DeleteBoard(ctx(), owner, boardID)
				return err
			},
			wantErr: nil,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			repo := newFakeRepo()
			boardID, _ := seedBoard(t, repo)
			repo.members[boardID][member] = domain.Membership{BoardID: boardID, UserID: member, Role: domain.RoleMember, Found: true}
			service := newService(repo)

			if err := testCase.action(service, boardID); !errors.Is(err, testCase.wantErr) {
				t.Errorf("err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestGetBoardNestsAndOrders(t *testing.T) {
	repo := newFakeRepo()
	boardID, columnID := seedBoard(t, repo)
	// Insert two cards out of position order to prove the read orders them.
	repo.cards["card-late"] = domain.Card{ID: "card-late", ColumnID: columnID, Title: "Second", Position: "k"}
	repo.cards["card-early"] = domain.Card{ID: "card-early", ColumnID: columnID, Title: "First", Position: "V"}
	service := newService(repo)

	tree, err := service.GetBoard(ctx(), owner, boardID)
	if err != nil {
		t.Fatalf("GetBoard: %v", err)
	}
	if len(tree.Columns) != 1 || len(tree.Columns[0].Cards) != 2 {
		t.Fatalf("tree shape = %+v, want 1 column with 2 cards", tree)
	}
	if tree.Columns[0].Cards[0].ID != "card-early" || tree.Columns[0].Cards[1].ID != "card-late" {
		t.Errorf("cards not ordered by position: %+v", tree.Columns[0].Cards)
	}
}

func TestMoveCardAcrossColumnsEmitsEvent(t *testing.T) {
	repo := newFakeRepo()
	boardID, source := seedBoard(t, repo)
	destination := "column-2"
	repo.columns[destination] = domain.Column{ID: destination, BoardID: boardID, Title: "Doing", Position: "k"}
	repo.cards["card-1"] = domain.Card{ID: "card-1", ColumnID: source, Title: "Task", Position: "V"}
	service := newService(repo)

	index := 0
	events, err := service.MoveCard(ctx(), owner, "card-1", &destination, &index)
	if err != nil {
		t.Fatalf("MoveCard: %v", err)
	}

	moved, ok := events[0].(domain.CardMoved)
	if !ok {
		t.Fatalf("event = %#v, want CardMoved", events[0])
	}
	if moved.FromColumnID != source || moved.ToColumnID != destination {
		t.Errorf("move = %s→%s, want %s→%s", moved.FromColumnID, moved.ToColumnID, source, destination)
	}
	if stored := repo.cards["card-1"]; stored.ColumnID != destination {
		t.Errorf("card column = %s, want %s (single-row move did not persist)", stored.ColumnID, destination)
	}
}

func TestMoveCardRetriesOnConflict(t *testing.T) {
	repo := newFakeRepo()
	_, columnID := seedBoard(t, repo)
	repo.cards["card-1"] = domain.Card{ID: "card-1", ColumnID: columnID, Title: "Task", Position: "V"}
	repo.injectConflicts = 1 // the first position write loses the race, then retries
	service := newService(repo)

	index := 0
	if _, err := service.MoveCard(ctx(), owner, "card-1", nil, &index); err != nil {
		t.Fatalf("MoveCard should converge after one conflict, got: %v", err)
	}
	if repo.injectConflicts != 0 {
		t.Errorf("injected conflict not consumed: %d remaining", repo.injectConflicts)
	}
}

func TestMoveCardSurfacesConflictAfterMaxRetries(t *testing.T) {
	repo := newFakeRepo()
	_, columnID := seedBoard(t, repo)
	repo.cards["card-1"] = domain.Card{ID: "card-1", ColumnID: columnID, Title: "Task", Position: "V"}
	repo.injectConflicts = 99 // never stops conflicting
	service := newService(repo)

	index := 0
	if _, err := service.MoveCard(ctx(), owner, "card-1", nil, &index); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("err = %v, want ErrConflict after exhausting retries", err)
	}
}

func TestCreateCardRenormalizesWhenKeysExhaust(t *testing.T) {
	repo := newFakeRepo()
	_, columnID := seedBoard(t, repo)
	// Two valid, adjacent, near-cap keys so inserting between them exhausts the
	// length budget and forces a renormalization (design D2 / AC-5).
	lowKey := domain.Position(strings.Repeat("V", 64))
	highKey := domain.Position(strings.Repeat("V", 63) + "W")
	repo.cards["card-low"] = domain.Card{ID: "card-low", ColumnID: columnID, Title: "Low", Position: lowKey}
	repo.cards["card-high"] = domain.Card{ID: "card-high", ColumnID: columnID, Title: "High", Position: highKey}
	service := newService(repo)

	index := 1 // between the two existing cards
	card, _, err := service.CreateCard(ctx(), owner, columnID, "Middle", &index)
	if err != nil {
		t.Fatalf("CreateCard through renormalization: %v", err)
	}

	if got := repo.cards["card-low"].Position; len(got) > 8 {
		t.Errorf("card-low position %q was not renormalized (len %d)", got, len(got))
	}
	// Visible order must be unchanged: low < middle < high.
	if repo.cards["card-low"].Position >= card.Position || card.Position >= repo.cards["card-high"].Position {
		t.Errorf("order broken after renormalization: low=%q mid=%q high=%q",
			repo.cards["card-low"].Position, card.Position, repo.cards["card-high"].Position)
	}
}

func TestEditCardValidatesDescriptionLength(t *testing.T) {
	repo := newFakeRepo()
	_, columnID := seedBoard(t, repo)
	repo.cards["card-1"] = domain.Card{ID: "card-1", ColumnID: columnID, Title: "Task", Position: "V"}
	service := newService(repo)

	tooLong := strings.Repeat("x", 10_001)
	if _, err := service.EditCard(ctx(), owner, "card-1", nil, &tooLong); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("err = %v, want ErrValidation", err)
	}
	if repo.cards["card-1"].Description != "" {
		t.Error("card description should be unchanged after a rejected edit")
	}
}
