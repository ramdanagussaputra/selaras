package auth

import "time"

// RefreshGraceWindow is how long a just-rotated refresh token keeps being
// tolerated when replayed (design D4). It absorbs benign concurrent / cross-tab
// refresh races: a revoked token replayed within this window while its family is
// still intact is treated as a race, not as theft.
const RefreshGraceWindow = 10 * time.Second

// RefreshDecision classifies a presented refresh token (design D4).
type RefreshDecision int

const (
	// RefreshInvalid means the token is unknown or expired → 401 TOKEN_INVALID.
	RefreshInvalid RefreshDecision = iota
	// RefreshRotate means a live token → revoke it and mint a successor in the family.
	RefreshRotate
	// RefreshToleratedReplay means a just-revoked token was replayed within the
	// grace window while the family is intact → mint a successor WITHOUT revoking
	// the family (benign race).
	RefreshToleratedReplay
	// RefreshReuse means a revoked token was replayed outside the grace window, or
	// with a compromised family → revoke the whole family, 401 TOKEN_REUSED.
	RefreshReuse
)

// DecideRefresh classifies an already-looked-up refresh token. familyHasLiveToken
// reports whether the token's family still has a live (unrevoked, unexpired)
// token — i.e. it looks like a normal rotation chain rather than a compromised
// family. Pure function of its inputs, so rotation/reuse/grace are table-testable
// against a fixed clock.
func DecideRefresh(token RefreshToken, familyHasLiveToken bool, currentTime time.Time, grace time.Duration) RefreshDecision {
	if token.RevokedAt != nil {
		withinGrace := currentTime.Sub(*token.RevokedAt) <= grace
		if withinGrace && familyHasLiveToken {
			return RefreshToleratedReplay
		}
		return RefreshReuse
	}

	if !token.ExpiresAt.After(currentTime) { // currentTime >= ExpiresAt
		return RefreshInvalid
	}

	return RefreshRotate
}

// NewRefreshToken builds a refresh-token row for a family, computing its expiry
// from the clock and TTL — the expiry rule lives in the domain.
func NewRefreshToken(id, userID, familyID string, tokenHash []byte, currentTime time.Time, lifetime time.Duration) RefreshToken {
	return RefreshToken{
		ID:        id,
		UserID:    userID,
		FamilyID:  familyID,
		TokenHash: tokenHash,
		ExpiresAt: currentTime.Add(lifetime),
		CreatedAt: currentTime,
	}
}
