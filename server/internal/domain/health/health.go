// Package health defines the core's view of service health.
//
// Pinger is the project's first port (design D4): the http adapter depends on
// it to answer /healthz, and the postgres adapter implements it. It exists to
// keep the handler free of any database dependency.
package health

import "context"

// Pinger reports whether a backing dependency is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}
