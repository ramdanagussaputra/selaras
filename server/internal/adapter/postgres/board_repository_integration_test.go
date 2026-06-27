package postgres_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramdanaguss/selaras/server/internal/adapter/postgres"
	"github.com/ramdanaguss/selaras/server/internal/adapter/security"
	appboard "github.com/ramdanaguss/selaras/server/internal/app/board"
	domain "github.com/ramdanaguss/selaras/server/internal/domain/board"
)

// boardTestPool connects to the local/dev Postgres and skips when it is
// unreachable or unmigrated, so `make test` stays green without a database while
// giving real coverage when one is up (mirrors the auth integration test).
func boardTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = defaultTestDatabaseURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Skipf("no database available (%v)", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("no database available (%v)", err)
	}
	if _, err := pool.Exec(context.Background(),
		`TRUNCATE users, boards, board_members, columns, cards RESTART IDENTITY CASCADE`); err != nil {
		pool.Close()
		t.Skipf("schema not migrated (%v); run make migrate-up", err)
	}

	t.Cleanup(pool.Close)
	return pool
}

// boardFixture seeds a user and returns a service plus that user's id, ready to
// drive board/column/card use cases against the real database.
func boardFixture(t *testing.T, pool *pgxpool.Pool) (*appboard.Service, string) {
	t.Helper()
	ctx := context.Background()

	user, err := postgres.NewUserRepository(pool).Create(ctx, newUser())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	service := appboard.NewService(postgres.NewBoardRepository(pool), security.SystemClock{})
	return service, user.ID
}

func TestBoardNestedReadOrdersByPosition(t *testing.T) {
	pool := boardTestPool(t)
	ctx := context.Background()
	service, userID := boardFixture(t, pool)

	board, _, err := service.CreateBoard(ctx, userID, "Sprint")
	if err != nil {
		t.Fatalf("CreateBoard: %v", err)
	}
	column, _, err := service.CreateColumn(ctx, userID, board.ID, "Todo", nil)
	if err != nil {
		t.Fatalf("CreateColumn: %v", err)
	}
	// Append three cards; appends must read back in creation order.
	for _, title := range []string{"first", "second", "third"} {
		if _, _, err := service.CreateCard(ctx, userID, column.ID, title, nil); err != nil {
			t.Fatalf("CreateCard %q: %v", title, err)
		}
	}

	tree, err := service.GetBoard(ctx, userID, board.ID)
	if err != nil {
		t.Fatalf("GetBoard: %v", err)
	}
	titles := []string{}
	for _, card := range tree.Columns[0].Cards {
		titles = append(titles, card.Title)
	}
	if want := []string{"first", "second", "third"}; !equalStrings(titles, want) {
		t.Errorf("card order = %v, want %v", titles, want)
	}
}

func TestColumnDeleteCascadesCards(t *testing.T) {
	pool := boardTestPool(t)
	ctx := context.Background()
	service, userID := boardFixture(t, pool)

	board, _, _ := service.CreateBoard(ctx, userID, "Sprint")
	column, _, _ := service.CreateColumn(ctx, userID, board.ID, "Todo", nil)
	if _, _, err := service.CreateCard(ctx, userID, column.ID, "doomed", nil); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	if _, err := service.DeleteColumn(ctx, userID, column.ID); err != nil {
		t.Fatalf("DeleteColumn: %v", err)
	}

	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cards WHERE column_id = $1::uuid`, column.ID).Scan(&remaining); err != nil {
		t.Fatalf("count cards: %v", err)
	}
	if remaining != 0 {
		t.Errorf("cards remaining after column delete = %d, want 0 (cascade failed)", remaining)
	}
}

func TestMoveCardIsSingleRowUpdate(t *testing.T) {
	pool := boardTestPool(t)
	ctx := context.Background()
	service, userID := boardFixture(t, pool)

	board, _, _ := service.CreateBoard(ctx, userID, "Sprint")
	source, _, _ := service.CreateColumn(ctx, userID, board.ID, "Todo", nil)
	destination, _, _ := service.CreateColumn(ctx, userID, board.ID, "Doing", nil)
	moving, _, _ := service.CreateCard(ctx, userID, source.ID, "moving", nil)
	staying, _, _ := service.CreateCard(ctx, userID, source.ID, "staying", nil)

	stayingBefore := positionOf(t, pool, staying.ID)

	index := 0
	if _, err := service.MoveCard(ctx, userID, moving.ID, &destination.ID, &index); err != nil {
		t.Fatalf("MoveCard: %v", err)
	}

	// The moved card now lives in the destination column; the untouched sibling's
	// position is unchanged — proving the move was a single-row update (AC-1).
	if got := columnOf(t, pool, moving.ID); got != destination.ID {
		t.Errorf("moved card column = %s, want %s", got, destination.ID)
	}
	if got := positionOf(t, pool, staying.ID); got != stayingBefore {
		t.Errorf("untouched sibling position changed from %q to %q", stayingBefore, got)
	}
}

func TestConcurrentMovesConverge(t *testing.T) {
	pool := boardTestPool(t)
	ctx := context.Background()
	service, userID := boardFixture(t, pool)

	board, _, _ := service.CreateBoard(ctx, userID, "Sprint")
	column, _, _ := service.CreateColumn(ctx, userID, board.ID, "Todo", nil)
	// A spread of cards so two movers to index 0 contend for the same head gap.
	for _, title := range []string{"a", "b", "c", "d"} {
		service.CreateCard(ctx, userID, column.ID, title, nil) //nolint:errcheck // seed
	}
	target, _, _ := service.CreateCard(ctx, userID, column.ID, "target", nil)

	var waitGroup sync.WaitGroup
	errs := make([]error, 2)
	for mover := range 2 {
		waitGroup.Add(1)
		go func(slot int) {
			defer waitGroup.Done()
			index := 0
			_, errs[slot] = service.MoveCard(ctx, userID, target.ID, nil, &index)
		}(mover)
	}
	waitGroup.Wait()

	for slot, err := range errs {
		if err != nil && !errors.Is(err, domain.ErrConflict) {
			t.Errorf("mover %d returned unexpected error: %v", slot, err)
		}
	}
	// Whichever ordering won, positions must remain unique (the backstop held).
	var duplicates int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT position FROM cards WHERE column_id = $1::uuid
			GROUP BY position HAVING count(*) > 1
		) dupes`, column.ID).Scan(&duplicates); err != nil {
		t.Fatalf("dup check: %v", err)
	}
	if duplicates != 0 {
		t.Errorf("found %d duplicated positions; the unique backstop failed", duplicates)
	}
}

func positionOf(t *testing.T, pool *pgxpool.Pool, cardID string) string {
	t.Helper()
	var position string
	if err := pool.QueryRow(context.Background(), `SELECT position FROM cards WHERE id = $1::uuid`, cardID).Scan(&position); err != nil {
		t.Fatalf("position of %s: %v", cardID, err)
	}
	return position
}

func columnOf(t *testing.T, pool *pgxpool.Pool, cardID string) string {
	t.Helper()
	var columnID string
	if err := pool.QueryRow(context.Background(), `SELECT column_id::text FROM cards WHERE id = $1::uuid`, cardID).Scan(&columnID); err != nil {
		t.Fatalf("column of %s: %v", cardID, err)
	}
	return columnID
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
