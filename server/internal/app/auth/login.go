package auth

import (
	"context"
	"errors"
	"fmt"

	domain "github.com/ramdanaguss/selaras/server/internal/domain/auth"
)

// Login verifies credentials and, on success, opens a new session (access token
// + refresh token in a fresh family). It is timing-safe against unknown emails:
// it still performs one password verification against a dummy hash so the
// response time does not reveal whether the email exists (design D9).
func (s *Service) Login(ctx context.Context, email, password string) (Tokens, error) {
	user, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			// Equalize timing with the real path, then fail identically.
			_, _ = s.hasher.Verify(password, s.dummyHash)
			return Tokens{}, domain.ErrInvalidCredentials
		}
		return Tokens{}, fmt.Errorf("looking up user: %w", err)
	}

	match, err := s.hasher.Verify(password, user.PasswordHash)
	if err != nil {
		return Tokens{}, fmt.Errorf("verifying password: %w", err)
	}
	if !match {
		return Tokens{}, domain.ErrInvalidCredentials
	}

	accessToken, rawRefresh, err := s.mintInFamily(ctx, user.ID, s.newID())
	if err != nil {
		return Tokens{}, err
	}

	return Tokens{AccessToken: accessToken, RefreshToken: rawRefresh, User: user}, nil
}
