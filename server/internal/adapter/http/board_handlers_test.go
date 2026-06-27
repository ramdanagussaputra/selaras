package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	adapterhttp "github.com/ramdanaguss/selaras/server/internal/adapter/http"
	"github.com/ramdanaguss/selaras/server/internal/adapter/security"
	appauth "github.com/ramdanaguss/selaras/server/internal/app/auth"
	appboard "github.com/ramdanaguss/selaras/server/internal/app/board"
	board "github.com/ramdanaguss/selaras/server/internal/domain/board"
)

// stubBoardRepo is a minimal board.Repository for the HTTP contract tests: only
// MembershipByBoard carries behaviour (driving the 404/403 rows); the remaining
// methods are inert because the validation/authorization checks short-circuit
// before any of them run.
type stubBoardRepo struct {
	membership board.Membership
}

func (s *stubBoardRepo) InTx(_ context.Context, fn func(repo board.Repository) error) error {
	return fn(s)
}

func (s *stubBoardRepo) MembershipByBoard(_ context.Context, _, _ string) (board.Membership, error) {
	return s.membership, nil
}
func (s *stubBoardRepo) MembershipByColumn(_ context.Context, _, _ string) (board.Membership, error) {
	return s.membership, nil
}
func (s *stubBoardRepo) MembershipByCard(_ context.Context, _, _ string) (board.Membership, error) {
	return s.membership, nil
}
func (s *stubBoardRepo) InsertBoard(_ context.Context, _ board.Board) error       { return nil }
func (s *stubBoardRepo) InsertMember(_ context.Context, _ board.Membership) error { return nil }
func (s *stubBoardRepo) ListBoards(_ context.Context, _ string) ([]board.Board, error) {
	return nil, nil
}
func (s *stubBoardRepo) GetBoard(_ context.Context, _ string) (board.Board, error) {
	return board.Board{}, nil
}
func (s *stubBoardRepo) UpdateBoardTitle(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}
func (s *stubBoardRepo) DeleteBoard(_ context.Context, _ string) error { return nil }
func (s *stubBoardRepo) GetColumn(_ context.Context, _ string) (board.Column, error) {
	return board.Column{}, nil
}
func (s *stubBoardRepo) ListColumns(_ context.Context, _ string) ([]board.Column, error) {
	return nil, nil
}
func (s *stubBoardRepo) InsertColumn(_ context.Context, _ board.Column) error   { return nil }
func (s *stubBoardRepo) UpdateColumnTitle(_ context.Context, _, _ string) error { return nil }
func (s *stubBoardRepo) UpdateColumnPosition(_ context.Context, _ string, _ board.Position) error {
	return nil
}
func (s *stubBoardRepo) RenormalizeColumns(_ context.Context, _ string, _ []board.ColumnPosition) error {
	return nil
}
func (s *stubBoardRepo) DeleteColumn(_ context.Context, _ string) error { return nil }
func (s *stubBoardRepo) GetCard(_ context.Context, _ string) (board.Card, error) {
	return board.Card{}, nil
}
func (s *stubBoardRepo) ListCardsByColumn(_ context.Context, _ string) ([]board.Card, error) {
	return nil, nil
}
func (s *stubBoardRepo) ListCardsByBoard(_ context.Context, _ string) ([]board.Card, error) {
	return nil, nil
}
func (s *stubBoardRepo) InsertCard(_ context.Context, _ board.Card) error { return nil }
func (s *stubBoardRepo) UpdateCardContent(_ context.Context, _, _, _ string, _ time.Time) error {
	return nil
}
func (s *stubBoardRepo) UpdateCardPosition(_ context.Context, _, _ string, _ board.Position) error {
	return nil
}
func (s *stubBoardRepo) RenormalizeCards(_ context.Context, _ string, _ []board.CardPosition) error {
	return nil
}
func (s *stubBoardRepo) DeleteCard(_ context.Context, _ string) error { return nil }

// newBoardTestRouter mounts the board routes behind real Bearer auth and returns
// a token already minted for `userID`, so a request carrying it reaches the
// handlers as that authenticated caller.
func newBoardTestRouter(t *testing.T, repo board.Repository, userID string) (http.Handler, string) {
	t.Helper()
	issuer := security.NewAccessTokenIssuer("0123456789abcdef0123456789abcdef", 15*time.Minute)
	authService, err := appauth.NewService(
		newMemUserRepo(), newMemTokenRepo(),
		security.NewArgon2idHasher(), issuer, security.NewRefreshTokenFactory(),
		security.SystemClock{}, 168*time.Hour,
	)
	if err != nil {
		t.Fatalf("auth NewService: %v", err)
	}

	token, err := issuer.Issue(userID, time.Now())
	if err != nil {
		t.Fatalf("issuing token: %v", err)
	}

	router := adapterhttp.NewRouter(adapterhttp.RouterConfig{
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		AuthService:     authService,
		BoardService:    appboard.NewService(repo, security.SystemClock{}),
		SecureCookies:   false,
		RefreshTokenTTL: 168 * time.Hour,
	})
	return router, token
}

func TestCreateBoardRejectsEmptyTitle(t *testing.T) {
	router, token := newBoardTestRouter(t, &stubBoardRepo{}, "user-1")

	resp := do(t, router, http.MethodPost, "/api/v1/boards", `{"title":""}`, nil, token)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, resp) != "VALIDATION_FAILED" {
		t.Fatalf("status = %d, want 422 VALIDATION_FAILED", resp.StatusCode)
	}
}

func TestNonMemberReadIs404(t *testing.T) {
	// No membership row → the use case answers ErrNotFound, never leaking existence.
	router, token := newBoardTestRouter(t, &stubBoardRepo{membership: board.Membership{Found: false}}, "outsider")

	resp := do(t, router, http.MethodGet, "/api/v1/boards/board-1", "", nil, token)
	if resp.StatusCode != http.StatusNotFound || errorCode(t, resp) != "NOT_FOUND" {
		t.Fatalf("status = %d, want 404 NOT_FOUND", resp.StatusCode)
	}
}

func TestOwnerOnlyDeleteByMemberIs403(t *testing.T) {
	member := board.Membership{BoardID: "board-1", UserID: "member-1", Role: board.RoleMember, Found: true}
	router, token := newBoardTestRouter(t, &stubBoardRepo{membership: member}, "member-1")

	resp := do(t, router, http.MethodDelete, "/api/v1/boards/board-1", "", nil, token)
	if resp.StatusCode != http.StatusForbidden || errorCode(t, resp) != "FORBIDDEN" {
		t.Fatalf("status = %d, want 403 FORBIDDEN", resp.StatusCode)
	}
}

func TestUnauthenticatedBoardRequestIs401(t *testing.T) {
	router, _ := newBoardTestRouter(t, &stubBoardRepo{}, "user-1")

	resp := do(t, router, http.MethodGet, "/api/v1/boards", "", nil, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}
