package auth

import (
	"context"
	"fmt"

	domain "github.com/ramdanaguss/selaras/server/internal/domain/auth"
)

// Refresh validates a presented refresh token and applies the family state
// machine (design D4): rotate a live token, tolerate a benign within-grace
// replay, revoke the family on genuine reuse, or reject an invalid/expired token.
func (s *Service) Refresh(ctx context.Context, rawToken string) (RefreshResult, error) {
	hash := s.refreshTokens.Hash(rawToken)

	token, err := s.tokens.FindByHash(ctx, hash)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("looking up refresh token: %w", err)
	}

	currentTime := s.clock.Now()
	familyHasLiveToken, err := s.tokens.FamilyHasLiveToken(ctx, token.FamilyID, currentTime)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("checking token family: %w", err)
	}

	switch domain.DecideRefresh(token, familyHasLiveToken, currentTime, s.graceWindow) {
	case domain.RefreshInvalid:
		return RefreshResult{}, domain.ErrTokenInvalid

	case domain.RefreshReuse:
		if err := s.tokens.RevokeFamily(ctx, token.FamilyID, currentTime); err != nil {
			return RefreshResult{}, fmt.Errorf("revoking reused token family: %w", err)
		}
		return RefreshResult{}, domain.ErrTokenReused

	case domain.RefreshRotate:
		if err := s.tokens.Revoke(ctx, token.ID, currentTime); err != nil {
			return RefreshResult{}, fmt.Errorf("revoking rotated token: %w", err)
		}
		return s.continueFamily(ctx, token)

	case domain.RefreshToleratedReplay:
		// Benign concurrent-refresh race: mint a successor but DO NOT revoke the
		// family, and leave the (already-revoked) presented token as is.
		return s.continueFamily(ctx, token)

	default:
		return RefreshResult{}, domain.ErrTokenInvalid
	}
}

// continueFamily mints a successor token in the presented token's family and
// opportunistically purges the user's fully-expired families (design D9).
func (s *Service) continueFamily(ctx context.Context, presented domain.RefreshToken) (RefreshResult, error) {
	accessToken, rawRefresh, err := s.mintInFamily(ctx, presented.UserID, presented.FamilyID)
	if err != nil {
		return RefreshResult{}, err
	}

	// Opportunistic cleanup; a failure here must not fail the refresh.
	_ = s.tokens.PurgeExpiredFamilies(ctx, presented.UserID, s.clock.Now())

	return RefreshResult{AccessToken: accessToken, RefreshToken: rawRefresh}, nil
}
