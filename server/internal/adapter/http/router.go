package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	appauth "github.com/ramdanaguss/selaras/server/internal/app/auth"
	appboard "github.com/ramdanaguss/selaras/server/internal/app/board"
	"github.com/ramdanaguss/selaras/server/internal/domain/health"
)

// RouterConfig carries the router's dependencies from the composition root.
type RouterConfig struct {
	Logger          *slog.Logger
	Pinger          health.Pinger
	CORSOrigin      string            // empty disables the CORS middleware
	AuthService     *appauth.Service  // nil disables the /api/v1 auth routes
	BoardService    *appboard.Service // nil disables the /api/v1 board routes
	SecureCookies   bool              // true sets Secure on the refresh cookie (prod)
	RefreshTokenTTL time.Duration     // refresh cookie max-age
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

		if config.BoardService != nil {
			mountBoardRoutes(apiRouter, config)
		}
	})
}

// mountBoardRoutes wires the kanban surface under the same /api/v1 group, all
// behind Bearer auth (spec 03-kanban-crud contract table). Columns and cards are
// addressed top-level (PATCH/DELETE /columns/{id}, /cards/{id}); the use cases
// resolve the owning board from the leaf id for authorization (design D7).
func mountBoardRoutes(apiRouter chi.Router, config RouterConfig) {
	handler := NewBoardHandler(config.BoardService, config.Logger)

	apiRouter.Group(func(protected chi.Router) {
		protected.Use(requireAuth(config.AuthService, config.Logger))

		protected.Get("/boards", handler.ListBoards)
		protected.Post("/boards", handler.CreateBoard)
		protected.Get("/boards/{id}", handler.GetBoard)
		protected.Patch("/boards/{id}", handler.RenameBoard)
		protected.Delete("/boards/{id}", handler.DeleteBoard)

		protected.Post("/boards/{id}/columns", handler.CreateColumn)
		protected.Patch("/columns/{id}", handler.UpdateColumn)
		protected.Delete("/columns/{id}", handler.DeleteColumn)

		protected.Post("/columns/{id}/cards", handler.CreateCard)
		protected.Patch("/cards/{id}", handler.UpdateCard)
		protected.Delete("/cards/{id}", handler.DeleteCard)
	})
}
