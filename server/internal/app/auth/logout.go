package auth

import (
	"context"
	"errors"
	"fmt"

	domain "github.com/ramdanaguss/selaras/server/internal/domain/auth"
)

// Logout revokes the presented refresh token server-side. It is best-effort: an
// unknown token is treated as already-logged-out (no error), so the handler can
// still clear the cookie and return 204.
func (s *Service) Logout(ctx context.Context, rawToken string) error {
	hash := s.refreshTokens.Hash(rawToken)

	token, err := s.tokens.FindByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, domain.ErrTokenInvalid) {
			return nil
		}
		return fmt.Errorf("looking up refresh token: %w", err)
	}

	if err := s.tokens.Revoke(ctx, token.ID, s.clock.Now()); err != nil {
		return fmt.Errorf("revoking refresh token: %w", err)
	}

	return nil
}
