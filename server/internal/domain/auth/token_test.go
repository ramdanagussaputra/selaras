package auth_test

import (
	"testing"
	"time"

	"github.com/ramdanaguss/selaras/server/internal/domain/auth"
)

func TestDecideRefresh(t *testing.T) {
	currentTime := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	grace := auth.RefreshGraceWindow

	revokedAt := func(d time.Duration) *time.Time {
		instant := currentTime.Add(d)
		return &instant
	}

	tests := []struct {
		name               string
		token              auth.RefreshToken
		familyHasLiveToken bool
		want               auth.RefreshDecision
	}{
		{
			name:  "live token rotates",
			token: auth.RefreshToken{ExpiresAt: currentTime.Add(time.Hour)},
			want:  auth.RefreshRotate,
		},
		{
			name:  "expired unrevoked token is invalid",
			token: auth.RefreshToken{ExpiresAt: currentTime.Add(-time.Second)},
			want:  auth.RefreshInvalid,
		},
		{
			name:  "expiry exactly currentTime is invalid",
			token: auth.RefreshToken{ExpiresAt: currentTime},
			want:  auth.RefreshInvalid,
		},
		{
			name:               "revoked within grace with live family is tolerated",
			token:              auth.RefreshToken{ExpiresAt: currentTime.Add(time.Hour), RevokedAt: revokedAt(-2 * time.Second)},
			familyHasLiveToken: true,
			want:               auth.RefreshToleratedReplay,
		},
		{
			name:               "revoked exactly at grace boundary is tolerated",
			token:              auth.RefreshToken{ExpiresAt: currentTime.Add(time.Hour), RevokedAt: revokedAt(-grace)},
			familyHasLiveToken: true,
			want:               auth.RefreshToleratedReplay,
		},
		{
			name:               "revoked within grace but family compromised is reuse",
			token:              auth.RefreshToken{ExpiresAt: currentTime.Add(time.Hour), RevokedAt: revokedAt(-2 * time.Second)},
			familyHasLiveToken: false,
			want:               auth.RefreshReuse,
		},
		{
			name:               "revoked outside grace is reuse even with live family",
			token:              auth.RefreshToken{ExpiresAt: currentTime.Add(time.Hour), RevokedAt: revokedAt(-grace - time.Second)},
			familyHasLiveToken: true,
			want:               auth.RefreshReuse,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := auth.DecideRefresh(testCase.token, testCase.familyHasLiveToken, currentTime, grace)
			if got != testCase.want {
				t.Errorf("DecideRefresh() = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestNewRefreshToken(t *testing.T) {
	currentTime := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	lifetime := 168 * time.Hour
	hash := []byte{0x01, 0x02}

	token := auth.NewRefreshToken("token-id", "user-id", "family-id", hash, currentTime, lifetime)

	if token.ExpiresAt != currentTime.Add(lifetime) {
		t.Errorf("ExpiresAt = %v, want %v", token.ExpiresAt, currentTime.Add(lifetime))
	}
	if token.CreatedAt != currentTime {
		t.Errorf("CreatedAt = %v, want %v", token.CreatedAt, currentTime)
	}
	if token.RevokedAt != nil {
		t.Errorf("RevokedAt = %v, want nil", token.RevokedAt)
	}
	if token.FamilyID != "family-id" || token.UserID != "user-id" || token.ID != "token-id" {
		t.Errorf("identity fields not carried through: %+v", token)
	}
}
