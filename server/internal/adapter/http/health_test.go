package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	httpadapter "github.com/ramdanaguss/selaras/server/internal/adapter/http"
)

// fakePinger implements the health.Pinger port without a database.
type fakePinger struct {
	err error
}

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestHealthHandler(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		wantCode   int
		wantStatus string
	}{
		{
			name:       "healthy database",
			pingErr:    nil,
			wantCode:   http.StatusOK,
			wantStatus: "ok",
		},
		{
			name:       "database unreachable",
			pingErr:    errors.New("connection refused"),
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "degraded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := httpadapter.NewHealthHandler(
				fakePinger{err: tt.pingErr},
				slog.New(slog.DiscardHandler),
			)

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

			if rec.Code != tt.wantCode {
				t.Errorf("status code = %d, want %d", rec.Code, tt.wantCode)
			}

			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not JSON: %v (body %q)", err, rec.Body.String())
			}
			if body["status"] != tt.wantStatus {
				t.Errorf(`body status = %q, want %q`, body["status"], tt.wantStatus)
			}
		})
	}
}
