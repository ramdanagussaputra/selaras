package http

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ramdanaguss/selaras/server/internal/domain/health"
)

// RouterConfig carries the router's dependencies from the composition root.
type RouterConfig struct {
	Logger     *slog.Logger
	Pinger     health.Pinger
	CORSOrigin string // empty disables the CORS middleware
}

// NewRouter assembles middleware and routes. Order matters: the request ID
// must exist before logging, and recovery must wrap everything below it.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(requestLogger(cfg.Logger))
	r.Use(recoverPanic(cfg.Logger))
	if cfg.CORSOrigin != "" {
		r.Use(corsOrigin(cfg.CORSOrigin))
	}

	r.Method(http.MethodGet, "/healthz", NewHealthHandler(cfg.Pinger, cfg.Logger))

	r.NotFound(spaFallback())

	return r
}
