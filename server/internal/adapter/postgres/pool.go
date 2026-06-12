// Package postgres holds the driven adapters backed by a pgx connection pool.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool builds a pgx connection pool from a DATABASE_URL connection string.
// It does not ping: boot stays fast and /healthz owns connectivity reporting.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("creating pgx pool: %w", err)
	}

	return pool, nil
}
