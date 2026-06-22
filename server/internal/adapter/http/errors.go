package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	domain "github.com/ramdanaguss/selaras/server/internal/domain/auth"
)

// Machine-readable error codes (spec 02-auth contract table).
const (
	codeValidationFailed   = "VALIDATION_FAILED"
	codeEmailTaken         = "EMAIL_TAKEN"
	codeInvalidCredentials = "INVALID_CREDENTIALS"
	codeTokenInvalid       = "TOKEN_INVALID"
	codeTokenExpired       = "TOKEN_EXPIRED"
	codeTokenReused        = "TOKEN_REUSED"
	codeRateLimited        = "RATE_LIMITED"
	codeInternal           = "INTERNAL"
)

// errorEnvelope is the single error response shape: {"error":{"code","message"}}.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

func writeErrorCode(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: message}})
}

// writeError is the single translation point from a domain error to the HTTP
// envelope: it maps the error, logs server-side at the right level (reuse at
// WARN, 5xx at ERROR), and writes the response — honoring the single-handling
// rule (an error is logged or returned, not both, at lower layers).
func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var validationErr *domain.ValidationError
	if errors.As(err, &validationErr) {
		writeErrorCode(w, http.StatusUnprocessableEntity, codeValidationFailed,
			validationErr.Field+" "+validationErr.Message)
		return
	}

	status, code := mapError(err)

	switch {
	case errors.Is(err, domain.ErrTokenReused):
		logger.WarnContext(r.Context(), "refresh token reuse detected",
			"request_id", middleware.GetReqID(r.Context()))
	case status >= http.StatusInternalServerError:
		logger.ErrorContext(r.Context(), "request failed",
			"request_id", middleware.GetReqID(r.Context()), "error", err.Error())
	}

	writeErrorCode(w, status, code, userMessage(code))
}

func mapError(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrEmailTaken):
		return http.StatusConflict, codeEmailTaken
	case errors.Is(err, domain.ErrInvalidCredentials):
		return http.StatusUnauthorized, codeInvalidCredentials
	case errors.Is(err, domain.ErrTokenExpired):
		return http.StatusUnauthorized, codeTokenExpired
	case errors.Is(err, domain.ErrTokenReused):
		return http.StatusUnauthorized, codeTokenReused
	case errors.Is(err, domain.ErrTokenInvalid):
		return http.StatusUnauthorized, codeTokenInvalid
	case errors.Is(err, domain.ErrUserNotFound):
		// A valid token whose user no longer exists is a dead session.
		return http.StatusUnauthorized, codeTokenInvalid
	default:
		return http.StatusInternalServerError, codeInternal
	}
}

// userMessage returns a generic, non-technical message for a code; technical
// detail is logged, never returned (spec: never expose internals to users).
func userMessage(code string) string {
	switch code {
	case codeEmailTaken:
		return "that email is already registered"
	case codeInvalidCredentials:
		return "invalid email or password"
	case codeTokenExpired:
		return "the access token has expired"
	case codeTokenReused:
		return "the session has been revoked"
	case codeTokenInvalid:
		return "the token is invalid"
	case codeRateLimited:
		return "too many requests, please slow down"
	default:
		return "an unexpected error occurred"
	}
}
