package board_test

import (
	"errors"
	"strings"
	"testing"

	domain "github.com/ramdanaguss/selaras/server/internal/domain/board"
)

func TestCreateColumnAppends(t *testing.T) {
	repo := newFakeRepo()
	boardID, existing := seedBoard(t, repo)
	service := newService(repo)

	column, events, err := service.CreateColumn(ctx(), owner, boardID, "Doing", nil)
	if err != nil {
		t.Fatalf("CreateColumn: %v", err)
	}
	// Appended after the seeded column at "V", so its key must sort after it.
	if column.Position <= repo.columns[existing].Position {
		t.Errorf("appended position %q is not after %q", column.Position, repo.columns[existing].Position)
	}
	if created, ok := events[0].(domain.ColumnCreated); !ok || created.ColumnID != column.ID {
		t.Errorf("event = %#v, want ColumnCreated", events[0])
	}
}

func TestRenameColumnRejectsEmptyTitle(t *testing.T) {
	repo := newFakeRepo()
	_, columnID := seedBoard(t, repo)
	service := newService(repo)

	if _, err := service.RenameColumn(ctx(), owner, columnID, "   "); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("err = %v, want ErrValidation", err)
	}
}

func TestRenameColumnUpdatesTitle(t *testing.T) {
	repo := newFakeRepo()
	_, columnID := seedBoard(t, repo)
	service := newService(repo)

	events, err := service.RenameColumn(ctx(), owner, columnID, "In progress")
	if err != nil {
		t.Fatalf("RenameColumn: %v", err)
	}
	if repo.columns[columnID].Title != "In progress" {
		t.Errorf("title = %q, want %q", repo.columns[columnID].Title, "In progress")
	}
	if _, ok := events[0].(domain.ColumnRenamed); !ok {
		t.Errorf("event = %#v, want ColumnRenamed", events[0])
	}
}

func TestReorderColumnMovesToFront(t *testing.T) {
	repo := newFakeRepo()
	boardID, first := seedBoard(t, repo)
	repo.columns["column-2"] = domain.Column{ID: "column-2", BoardID: boardID, Title: "Doing", Position: "k"}
	service := newService(repo)

	index := 0
	events, err := service.ReorderColumn(ctx(), owner, "column-2", &index)
	if err != nil {
		t.Fatalf("ReorderColumn: %v", err)
	}
	// Moving column-2 before the seeded column means its key now sorts first.
	if repo.columns["column-2"].Position >= repo.columns[first].Position {
		t.Errorf("reordered position %q is not before %q", repo.columns["column-2"].Position, repo.columns[first].Position)
	}
	if _, ok := events[0].(domain.ColumnMoved); !ok {
		t.Errorf("event = %#v, want ColumnMoved", events[0])
	}
}

func TestCreateColumnRenormalizesWhenKeysExhaust(t *testing.T) {
	repo := newFakeRepo()
	boardID, seeded := seedBoard(t, repo)
	delete(repo.columns, seeded) // replace the seeded column with two near-cap neighbors
	lowKey := domain.Position(strings.Repeat("V", 64))
	highKey := domain.Position(strings.Repeat("V", 63) + "W")
	repo.columns["col-low"] = domain.Column{ID: "col-low", BoardID: boardID, Title: "Low", Position: lowKey}
	repo.columns["col-high"] = domain.Column{ID: "col-high", BoardID: boardID, Title: "High", Position: highKey}
	service := newService(repo)

	index := 1
	column, _, err := service.CreateColumn(ctx(), owner, boardID, "Middle", &index)
	if err != nil {
		t.Fatalf("CreateColumn through renormalization: %v", err)
	}
	if len(repo.columns["col-low"].Position) > 8 {
		t.Errorf("col-low position %q was not renormalized", repo.columns["col-low"].Position)
	}
	if repo.columns["col-low"].Position >= column.Position || column.Position >= repo.columns["col-high"].Position {
		t.Errorf("order broken after column renormalization")
	}
}

func TestDeleteColumnEmitsEvent(t *testing.T) {
	repo := newFakeRepo()
	_, columnID := seedBoard(t, repo)
	service := newService(repo)

	events, err := service.DeleteColumn(ctx(), owner, columnID)
	if err != nil {
		t.Fatalf("DeleteColumn: %v", err)
	}
	if _, ok := repo.columns[columnID]; ok {
		t.Error("column should be gone after delete")
	}
	if _, ok := events[0].(domain.ColumnDeleted); !ok {
		t.Errorf("event = %#v, want ColumnDeleted", events[0])
	}
}

func TestRenameBoardUpdatesTitle(t *testing.T) {
	repo := newFakeRepo()
	boardID, _ := seedBoard(t, repo)
	service := newService(repo)

	events, err := service.RenameBoard(ctx(), owner, boardID, "Renamed")
	if err != nil {
		t.Fatalf("RenameBoard: %v", err)
	}
	if repo.boards[boardID].Title != "Renamed" {
		t.Errorf("title = %q, want Renamed", repo.boards[boardID].Title)
	}
	if _, ok := events[0].(domain.Updated); !ok {
		t.Errorf("event = %#v, want Updated", events[0])
	}
}

func TestCreateCardAppendsAndEditUpdates(t *testing.T) {
	repo := newFakeRepo()
	_, columnID := seedBoard(t, repo)
	service := newService(repo)

	card, _, err := service.CreateCard(ctx(), owner, columnID, "Task", nil)
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if card.Description != "" {
		t.Errorf("new card description = %q, want empty", card.Description)
	}

	newTitle, newDescription := "Edited", "Some detail"
	if _, err := service.EditCard(ctx(), owner, card.ID, &newTitle, &newDescription); err != nil {
		t.Fatalf("EditCard: %v", err)
	}
	stored := repo.cards[card.ID]
	if stored.Title != newTitle || stored.Description != newDescription {
		t.Errorf("card = %+v, want title %q description %q", stored, newTitle, newDescription)
	}
}

func TestDeleteCardEmitsEvent(t *testing.T) {
	repo := newFakeRepo()
	_, columnID := seedBoard(t, repo)
	repo.cards["card-1"] = domain.Card{ID: "card-1", ColumnID: columnID, Title: "Task", Position: "V"}
	service := newService(repo)

	events, err := service.DeleteCard(ctx(), owner, "card-1")
	if err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}
	if _, ok := repo.cards["card-1"]; ok {
		t.Error("card should be gone after delete")
	}
	if _, ok := events[0].(domain.CardDeleted); !ok {
		t.Errorf("event = %#v, want CardDeleted", events[0])
	}
}
