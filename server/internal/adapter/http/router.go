package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	appauth "github.com/ramdanaguss/selaras/server/internal/app/auth"
	"github.com/ramdanaguss/selaras/server/internal/domain/health"
)

// RouterConfig carries the router's dependencies from the composition root.
type RouterConfig struct {
	Logger          *slog.Logger
	Pinger          health.Pinger
	CORSOrigin      string           // empty disables the CORS middleware
	AuthService     *appauth.Service // nil disables the /api/v1 auth routes
	SecureCookies   bool             // true sets Secure on the refresh cookie (prod)
	RefreshTokenTTL time.Duration    // refresh cookie max-age
}

// NewRouter assembles middleware and routes. Order matters: the request ID
// must exist before logging, and recovery must wrap everything below it.
func NewRouter(config RouterConfig) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(requestLogger(config.Logger))
	r.Use(recoverPanic(config.Logger))
	if config.CORSOrigin != "" {
		r.Use(corsOrigin(config.CORSOrigin))
	}

	r.Method(http.MethodGet, "/healthz", NewHealthHandler(config.Pinger, config.Logger))

	if config.AuthService != nil {
		mountAuthRoutes(r, config)
	}

	r.NotFound(spaFallback())

	return r
}

// mountAuthRoutes wires the /api/v1 auth surface (design D1): rate-limited
// register/login/refresh/logout under /auth, and the Bearer-protected /me.
func mountAuthRoutes(r chi.Router, config RouterConfig) {
	handler := NewAuthHandler(config.AuthService, config.Logger, config.SecureCookies, config.RefreshTokenTTL)
	limiter := newIPRateLimiter()

	r.Route("/api/v1", func(apiRouter chi.Router) {
		apiRouter.Route("/auth", func(authRoutes chi.Router) {
			authRoutes.Use(rateLimit(limiter))

			authRoutes.Post("/register", handler.Register)
			authRoutes.Post("/login", handler.Login)
			authRoutes.Post("/refresh", handler.Refresh)
			authRoutes.Post("/logout", handler.Logout)
		})

		apiRouter.With(requireAuth(config.AuthService, config.Logger)).Get("/me", handler.Me)
	})
}
