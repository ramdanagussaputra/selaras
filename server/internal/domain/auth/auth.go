// Package auth holds the authentication domain: user accounts, refresh-token
// family rules, and the ports the application layer drives them through. It
// imports the standard library only (the dependency rule, enforced by
// internal/domain/deprule_test.go) — argon2id, JWT, and pgx all live in adapters.
package auth

import (
	"errors"
	"time"
)

// User is a registered account. PasswordHash carries the argon2id PHC string used
// to verify credentials; the HTTP layer maps User to a response that omits it.
type User struct {
	ID           string
	Email        string
	DisplayName  string
	PasswordHash string
	CreatedAt    time.Time
}

// RefreshToken is one issued refresh token. Only TokenHash (the SHA-256 of the
// raw token) is ever persisted; the raw value lives only in the client cookie.
// RevokedAt is nil while the token is live.
type RefreshToken struct {
	ID        string
	UserID    string
	FamilyID  string
	TokenHash []byte
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

// Clock supplies the current time, letting the family rules be tested against a
// fixed instant instead of the wall clock (design D4).
type Clock interface {
	Now() time.Time
}

// Sentinel errors expected callers match with errors.Is; the HTTP layer maps each
// to its contract (status, code) pair (design D6).
var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
	ErrTokenInvalid       = errors.New("refresh token invalid")
	ErrTokenReused        = errors.New("refresh token reused")
	ErrTokenExpired       = errors.New("access token expired")
)

// ValidationError reports a single invalid input field. It maps to
// 422 VALIDATION_FAILED at the HTTP boundary (matched via errors.As).
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}
