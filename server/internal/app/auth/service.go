// Package auth holds the authentication use cases — Register, Login, Refresh,
// Logout — orchestrating the domain rules through its ports. It depends on the
// domain only (the dependency rule); argon2id, JWT, and pgx arrive as injected
// adapters.
package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/ramdanaguss/selaras/server/internal/domain/auth"
)

// Service coordinates the auth use cases over the domain ports.
type Service struct {
	users         domain.UserRepository
	tokens        domain.RefreshTokenRepository
	hasher        domain.PasswordHasher
	accessTokens  domain.AccessTokenIssuer
	refreshTokens domain.RefreshTokenFactory
	clock         domain.Clock
	refreshTTL    time.Duration
	graceWindow   time.Duration
	dummyHash     string        // precomputed, for timing-safe unknown-email login (design D9)
	newID         func() string // overridable in tests; defaults to uuid.NewString
}

// Tokens is the result of a successful login: an access token, the raw refresh
// token (to be set as the cookie), and the authenticated user.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	User         domain.User
}

// RefreshResult is the result of a successful refresh: a new access token and the
// rotated raw refresh token for the new cookie.
type RefreshResult struct {
	AccessToken  string
	RefreshToken string
}

// NewService wires the use cases. It precomputes a dummy password hash so that
// logins for unknown emails cost the same as for real ones (anti-enumeration).
func NewService(
	users domain.UserRepository,
	tokens domain.RefreshTokenRepository,
	hasher domain.PasswordHasher,
	accessTokens domain.AccessTokenIssuer,
	refreshTokens domain.RefreshTokenFactory,
	clock domain.Clock,
	refreshTTL time.Duration,
) (*Service, error) {
	dummyHash, err := hasher.Hash("timing-equalizer-not-a-real-password")
	if err != nil {
		return nil, fmt.Errorf("precomputing dummy hash: %w", err)
	}

	return &Service{
		users:         users,
		tokens:        tokens,
		hasher:        hasher,
		accessTokens:  accessTokens,
		refreshTokens: refreshTokens,
		clock:         clock,
		refreshTTL:    refreshTTL,
		graceWindow:   domain.RefreshGraceWindow,
		dummyHash:     dummyHash,
		newID:         uuid.NewString,
	}, nil
}

// Authenticate verifies an access token and returns its subject user id. Used by
// the HTTP auth middleware; returns domain.ErrTokenExpired or ErrTokenInvalid.
func (s *Service) Authenticate(accessToken string) (string, error) {
	return s.accessTokens.Verify(accessToken)
}

// Me returns the user for an authenticated request.
func (s *Service) Me(ctx context.Context, userID string) (domain.User, error) {
	return s.users.FindByID(ctx, userID)
}

// mintInFamily issues an access token and a fresh refresh token within an
// existing family, persisting the refresh token. Shared by login (new family),
// rotation, and tolerated replay.
func (s *Service) mintInFamily(ctx context.Context, userID, familyID string) (accessToken, rawRefresh string, err error) {
	currentTime := s.clock.Now()

	accessToken, err = s.accessTokens.Issue(userID, currentTime)
	if err != nil {
		return "", "", fmt.Errorf("issuing access token: %w", err)
	}

	rawRefresh, hash, err := s.refreshTokens.Generate()
	if err != nil {
		return "", "", fmt.Errorf("generating refresh token: %w", err)
	}

	token := domain.NewRefreshToken(s.newID(), userID, familyID, hash, currentTime, s.refreshTTL)
	if err := s.tokens.Create(ctx, token); err != nil {
		return "", "", fmt.Errorf("storing refresh token: %w", err)
	}

	return accessToken, rawRefresh, nil
}
