package http

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	appauth "github.com/ramdanaguss/selaras/server/internal/app/auth"
	domain "github.com/ramdanaguss/selaras/server/internal/domain/auth"
)

// contextKey is an unexported type so auth context values can't collide with
// keys set by other packages.
type contextKey int

const userIDContextKey contextKey = iota

// userIDFromContext returns the authenticated user id injected by requireAuth.
func userIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDContextKey).(string)
	return userID, ok
}

// requireAuth verifies the Bearer access token and injects the user id into the
// request context. Verification is stateless (no DB read); missing or invalid
// tokens short-circuit with 401 and the protected handler never runs.
func requireAuth(service *appauth.Service, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				writeError(w, r, logger, domain.ErrTokenInvalid)
				return
			}

			userID, err := service.Authenticate(token)
			if err != nil {
				writeError(w, r, logger, err)
				return
			}

			ctx := context.WithValue(r.Context(), userIDContextKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	return header[len(prefix):], true
}
