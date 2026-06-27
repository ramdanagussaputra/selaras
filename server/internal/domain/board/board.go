// Package board holds the kanban domain: boards, columns, cards, the fractional
// index Position type, the events the use cases emit, and the Repository
// port the application layer drives them through. It imports the standard library
// only (the dependency rule, enforced by internal/domain/deprule_test.go) — uuid
// generation, pgx, and HTTP all live in adapters; ids are passed in (design D8).
package board

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Field length limits (spec 03-kanban-crud Validation). They are pure rules, so
// they live in the domain and are shared by every create/edit use case.
const (
	maxTitleLength       = 200
	maxDescriptionLength = 10_000
)

// Role is a member's authorization level on a board. Owner is recorded when a
// board is created; member is the level M5 will grant. Only the owner may delete.
type Role string

// The two membership roles. Owner is recorded at creation and may delete the
// board; member is the level M5's sharing will grant.
const (
	RoleOwner  Role = "owner"
	RoleMember Role = "member"
)

// Sentinel errors expected callers match with errors.Is; the HTTP layer maps each
// to its (status, code) contract pair (design D7, spec contract table):
// ErrValidation→422, ErrNotFound→404, ErrForbidden→403, ErrConflict→409.
var (
	ErrValidation = errors.New("validation failed")
	ErrNotFound   = errors.New("not found")
	ErrForbidden  = errors.New("forbidden")
	ErrConflict   = errors.New("conflict")
)

// Board is a kanban board. OwnerID is the user who created it; membership and
// role live in the board_members table, read through the repository (design D7).
type Board struct {
	ID        string
	OwnerID   string
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Column is an ordered lane within a board. Position is its fractional index
// among the board's columns.
type Column struct {
	ID        string
	BoardID   string
	Title     string
	Position  Position
	CreatedAt time.Time
}

// Card is an item within a column. Position is its fractional index among the
// column's cards; Description defaults to the empty string.
type Card struct {
	ID          string
	ColumnID    string
	Title       string
	Description string
	Position    Position
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Membership is the caller's row in board_members for a given board, resolved by
// the repository from a board, column, or card id so the app layer can authorize
// without re-querying (design D7). Found is false when the caller is not a member.
type Membership struct {
	BoardID string
	UserID  string
	Role    Role
	Found   bool
}

// ValidateTitle enforces the 1–200 character title rule, returning an error that
// wraps ErrValidation (matched via errors.Is) on failure.
func ValidateTitle(title string) error {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return fmt.Errorf("%w: title must not be empty", ErrValidation)
	}
	if len(title) > maxTitleLength {
		return fmt.Errorf("%w: title must be at most %d characters", ErrValidation, maxTitleLength)
	}
	return nil
}

// ValidateDescription enforces the 10 000 character description ceiling. The
// empty string is valid (a card's default description).
func ValidateDescription(description string) error {
	if len(description) > maxDescriptionLength {
		return fmt.Errorf("%w: description must be at most %d characters", ErrValidation, maxDescriptionLength)
	}
	return nil
}
