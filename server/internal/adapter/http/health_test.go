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

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			handler := httpadapter.NewHealthHandler(
				fakePinger{err: testCase.pingErr},
				slog.New(slog.DiscardHandler),
			)

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

			if recorder.Code != testCase.wantCode {
				t.Errorf("status code = %d, want %d", recorder.Code, testCase.wantCode)
			}

			var body map[string]string
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not JSON: %v (body %q)", err, recorder.Body.String())
			}
			if body["status"] != testCase.wantStatus {
				t.Errorf(`body status = %q, want %q`, body["status"], testCase.wantStatus)
			}
		})
	}
}
