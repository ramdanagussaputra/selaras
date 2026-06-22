package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "github.com/ramdanaguss/selaras/server/internal/domain/auth"
)

// RefreshTokenRepository implements domain/auth.RefreshTokenRepository over pgx.
type RefreshTokenRepository struct {
	pool *pgxpool.Pool
}

// NewRefreshTokenRepository wraps a pool as a refresh-token repository.
func NewRefreshTokenRepository(pool *pgxpool.Pool) *RefreshTokenRepository {
	return &RefreshTokenRepository{pool: pool}
}

// Create inserts a newly minted refresh token.
func (r *RefreshTokenRepository) Create(ctx context.Context, token domain.RefreshToken) error {
	const query = `
		INSERT INTO refresh_tokens (id, user_id, token_hash, family_id, expires_at, revoked_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := r.pool.Exec(ctx, query,
		token.ID, token.UserID, token.TokenHash, token.FamilyID,
		token.ExpiresAt, token.RevokedAt, token.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting refresh token: %w", err)
	}
	return nil
}

// FindByHash returns the token with the given SHA-256 hash, or
// domain.ErrTokenInvalid when none exists.
func (r *RefreshTokenRepository) FindByHash(ctx context.Context, tokenHash []byte) (domain.RefreshToken, error) {
	const query = `
		SELECT id::text, user_id::text, token_hash, family_id::text, expires_at, revoked_at, created_at
		FROM refresh_tokens WHERE token_hash = $1`

	var token domain.RefreshToken
	err := r.pool.QueryRow(ctx, query, tokenHash).Scan(
		&token.ID, &token.UserID, &token.TokenHash, &token.FamilyID,
		&token.ExpiresAt, &token.RevokedAt, &token.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.RefreshToken{}, domain.ErrTokenInvalid
		}
		return domain.RefreshToken{}, fmt.Errorf("querying refresh token: %w", err)
	}
	return token, nil
}

// Revoke marks a single token revoked at currentTime (idempotent: already-revoked rows
// are left untouched).
func (r *RefreshTokenRepository) Revoke(ctx context.Context, id string, currentTime time.Time) error {
	const query = `UPDATE refresh_tokens SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL`
	if _, err := r.pool.Exec(ctx, query, id, currentTime); err != nil {
		return fmt.Errorf("revoking refresh token: %w", err)
	}
	return nil
}

// RevokeFamily marks every still-live token in the family revoked at currentTime.
func (r *RefreshTokenRepository) RevokeFamily(ctx context.Context, familyID string, currentTime time.Time) error {
	const query = `UPDATE refresh_tokens SET revoked_at = $2 WHERE family_id = $1 AND revoked_at IS NULL`
	if _, err := r.pool.Exec(ctx, query, familyID, currentTime); err != nil {
		return fmt.Errorf("revoking token family: %w", err)
	}
	return nil
}

// FamilyHasLiveToken reports whether the family still has an unrevoked, unexpired
// token as of currentTime.
func (r *RefreshTokenRepository) FamilyHasLiveToken(ctx context.Context, familyID string, currentTime time.Time) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM refresh_tokens
			WHERE family_id = $1 AND revoked_at IS NULL AND expires_at > $2
		)`

	var exists bool
	if err := r.pool.QueryRow(ctx, query, familyID, currentTime).Scan(&exists); err != nil {
		return false, fmt.Errorf("checking family for live token: %w", err)
	}
	return exists, nil
}

// PurgeExpiredFamilies deletes the user's families whose newest token has passed
// expiry (opportunistic cleanup; design D9).
func (r *RefreshTokenRepository) PurgeExpiredFamilies(ctx context.Context, userID string, currentTime time.Time) error {
	const query = `
		DELETE FROM refresh_tokens
		WHERE user_id = $1 AND family_id IN (
			SELECT family_id FROM refresh_tokens
			WHERE user_id = $1
			GROUP BY family_id
			HAVING max(expires_at) <= $2
		)`

	if _, err := r.pool.Exec(ctx, query, userID, currentTime); err != nil {
		return fmt.Errorf("purging expired token families: %w", err)
	}
	return nil
}
