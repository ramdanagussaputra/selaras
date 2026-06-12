package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pinger implements the health.Pinger port over a pgx pool.
type Pinger struct {
	pool *pgxpool.Pool
}

// NewPinger wraps a pool as a health pinger.
func NewPinger(pool *pgxpool.Pool) *Pinger {
	return &Pinger{pool: pool}
}

// Ping checks database reachability.
func (p *Pinger) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}
