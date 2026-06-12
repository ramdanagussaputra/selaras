// Package http is the driver adapter: chi router, middleware, and handlers.
package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/ramdanaguss/selaras/server/internal/domain/health"
)

// HealthHandler answers GET /healthz through the health.Pinger port, so it is
// unit-testable with a fake and never touches pgx directly (design D4).
type HealthHandler struct {
	pinger health.Pinger
	logger *slog.Logger
}

// NewHealthHandler wires the handler to a pinger port.
func NewHealthHandler(pinger health.Pinger, logger *slog.Logger) *HealthHandler {
	return &HealthHandler{pinger: pinger, logger: logger}
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	status, body := http.StatusOK, map[string]string{"status": "ok"}

	if err := h.pinger.Ping(r.Context()); err != nil {
		h.logger.ErrorContext(r.Context(), "health ping failed", "error", err)
		status, body = http.StatusServiceUnavailable, map[string]string{"status": "degraded"}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		h.logger.ErrorContext(r.Context(), "writing health response", "error", err)
	}
}
