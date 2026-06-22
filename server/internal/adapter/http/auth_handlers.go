package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	appauth "github.com/ramdanaguss/selaras/server/internal/app/auth"
	domain "github.com/ramdanaguss/selaras/server/internal/domain/auth"
)

const (
	refreshCookieName = "refresh_token"
	refreshCookiePath = "/api/v1/auth"
)

// AuthHandler serves the auth endpoints. secureCookies is true in production so
// the refresh cookie carries the Secure attribute (omitted on plain-http dev).
type AuthHandler struct {
	service       *appauth.Service
	logger        *slog.Logger
	secureCookies bool
	refreshTTL    time.Duration
}

// NewAuthHandler constructs the handler.
func NewAuthHandler(service *appauth.Service, logger *slog.Logger, secureCookies bool, refreshTTL time.Duration) *AuthHandler {
	return &AuthHandler{service: service, logger: logger, secureCookies: secureCookies, refreshTTL: refreshTTL}
}

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
}

type userEnvelope struct {
	User userResponse `json:"user"`
}

type loginResponse struct {
	AccessToken string       `json:"accessToken"`
	User        userResponse `json:"user"`
}

type accessTokenResponse struct {
	AccessToken string `json:"accessToken"`
}

func toUserResponse(user domain.User) userResponse {
	return userResponse{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		CreatedAt:   user.CreatedAt,
	}
}

// Register handles POST /api/v1/auth/register.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var input registerRequest
	if !decodeJSON(w, r, &input) {
		return
	}

	user, err := h.service.Register(r.Context(), input.Email, input.Password, input.DisplayName)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}

	writeJSON(w, http.StatusCreated, userEnvelope{User: toUserResponse(user)})
}

// Login handles POST /api/v1/auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var input loginRequest
	if !decodeJSON(w, r, &input) {
		return
	}

	tokens, err := h.service.Login(r.Context(), input.Email, input.Password)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}

	h.setRefreshCookie(w, tokens.RefreshToken)
	writeJSON(w, http.StatusOK, loginResponse{
		AccessToken: tokens.AccessToken,
		User:        toUserResponse(tokens.User),
	})
}

// Refresh handles POST /api/v1/auth/refresh.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil {
		writeError(w, r, h.logger, domain.ErrTokenInvalid)
		return
	}

	result, err := h.service.Refresh(r.Context(), cookie.Value)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}

	h.setRefreshCookie(w, result.RefreshToken)
	writeJSON(w, http.StatusOK, accessTokenResponse{AccessToken: result.AccessToken})
}

// Logout handles POST /api/v1/auth/logout. Best-effort: a missing cookie still
// clears state and returns 204.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(refreshCookieName); err == nil {
		if logoutErr := h.service.Logout(r.Context(), cookie.Value); logoutErr != nil {
			writeError(w, r, h.logger, logoutErr)
			return
		}
	}

	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// Me handles GET /api/v1/me behind requireAuth.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, r, h.logger, domain.ErrTokenInvalid)
		return
	}

	user, err := h.service.Me(r.Context(), userID)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}

	writeJSON(w, http.StatusOK, userEnvelope{User: toUserResponse(user)})
}

func (h *AuthHandler) setRefreshCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    value,
		Path:     refreshCookiePath,
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.refreshTTL.Seconds()),
	})
}

func (h *AuthHandler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// decodeJSON decodes the request body, writing a 422 and returning false on
// malformed input.
func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	if err := json.NewDecoder(r.Body).Decode(destination); err != nil {
		writeErrorCode(w, http.StatusUnprocessableEntity, codeValidationFailed, "the request body was invalid")
		return false
	}
	return true
}
