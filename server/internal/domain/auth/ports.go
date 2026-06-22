package auth

import (
	"context"
	"time"
)

// UserRepository persists user accounts (driven port; pgx adapter).
type UserRepository interface {
	// Create inserts a new user and returns it; returns ErrEmailTaken on a
	// duplicate email.
	Create(ctx context.Context, user User) (User, error)
	// FindByEmail returns the user with the given email, or ErrUserNotFound.
	FindByEmail(ctx context.Context, email string) (User, error)
	// FindByID returns the user with the given id, or ErrUserNotFound.
	FindByID(ctx context.Context, id string) (User, error)
}

// RefreshTokenRepository persists refresh-token families (driven port; pgx
// adapter). Implementations must be safe for concurrent use.
type RefreshTokenRepository interface {
	// Create inserts a newly minted refresh token.
	Create(ctx context.Context, token RefreshToken) error
	// FindByHash returns the token with the given SHA-256 hash, or
	// ErrTokenInvalid when none exists.
	FindByHash(ctx context.Context, tokenHash []byte) (RefreshToken, error)
	// Revoke marks a single token revoked at currentTime (idempotent).
	Revoke(ctx context.Context, id string, currentTime time.Time) error
	// RevokeFamily marks every still-live token in the family revoked at currentTime.
	RevokeFamily(ctx context.Context, familyID string, currentTime time.Time) error
	// FamilyHasLiveToken reports whether the family still has an unrevoked,
	// unexpired token as of currentTime.
	FamilyHasLiveToken(ctx context.Context, familyID string, currentTime time.Time) (bool, error)
	// PurgeExpiredFamilies deletes the user's families whose tokens are all past
	// expiry (opportunistic cleanup; design D9).
	PurgeExpiredFamilies(ctx context.Context, userID string, currentTime time.Time) error
}

// PasswordHasher hashes and verifies passwords (driven port; argon2id adapter).
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, encodedHash string) (bool, error)
}

// AccessTokenIssuer issues and verifies short-lived JWT access tokens (driven
// port; golang-jwt adapter). Verify returns the subject user id, or
// ErrTokenExpired / ErrTokenInvalid.
type AccessTokenIssuer interface {
	Issue(userID string, currentTime time.Time) (string, error)
	Verify(token string) (userID string, err error)
}

// RefreshTokenFactory generates raw refresh tokens and hashes them for storage
// and lookup (driven port; crypto/rand + SHA-256 adapter).
type RefreshTokenFactory interface {
	// Generate returns a fresh raw token (base64url) and its SHA-256 hash.
	Generate() (raw string, hash []byte, err error)
	// Hash returns the SHA-256 of a raw token, for looking up a presented cookie.
	Hash(raw string) []byte
}
