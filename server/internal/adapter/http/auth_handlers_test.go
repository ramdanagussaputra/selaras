package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adapterhttp "github.com/ramdanaguss/selaras/server/internal/adapter/http"
	"github.com/ramdanaguss/selaras/server/internal/adapter/security"
	appauth "github.com/ramdanaguss/selaras/server/internal/app/auth"
	domain "github.com/ramdanaguss/selaras/server/internal/domain/auth"
)

// --- in-memory repositories (no DB; exercise the HTTP layer end to end) -----

type memUserRepo struct {
	byEmail map[string]domain.User
	byID    map[string]domain.User
}

func newMemUserRepo() *memUserRepo {
	return &memUserRepo{byEmail: map[string]domain.User{}, byID: map[string]domain.User{}}
}
func (m *memUserRepo) Create(_ context.Context, user domain.User) (domain.User, error) {
	if _, exists := m.byEmail[strings.ToLower(user.Email)]; exists {
		return domain.User{}, domain.ErrEmailTaken
	}
	m.byEmail[strings.ToLower(user.Email)] = user
	m.byID[user.ID] = user
	return user, nil
}
func (m *memUserRepo) FindByEmail(_ context.Context, email string) (domain.User, error) {
	if user, ok := m.byEmail[strings.ToLower(email)]; ok {
		return user, nil
	}
	return domain.User{}, domain.ErrUserNotFound
}
func (m *memUserRepo) FindByID(_ context.Context, id string) (domain.User, error) {
	if user, ok := m.byID[id]; ok {
		return user, nil
	}
	return domain.User{}, domain.ErrUserNotFound
}

type memTokenRepo struct {
	byID map[string]*domain.RefreshToken
}

func newMemTokenRepo() *memTokenRepo {
	return &memTokenRepo{byID: map[string]*domain.RefreshToken{}}
}
func (m *memTokenRepo) Create(_ context.Context, token domain.RefreshToken) error {
	stored := token
	m.byID[token.ID] = &stored
	return nil
}
func (m *memTokenRepo) FindByHash(_ context.Context, tokenHash []byte) (domain.RefreshToken, error) {
	for _, token := range m.byID {
		if string(token.TokenHash) == string(tokenHash) {
			return *token, nil
		}
	}
	return domain.RefreshToken{}, domain.ErrTokenInvalid
}
func (m *memTokenRepo) Revoke(_ context.Context, id string, currentTime time.Time) error {
	if token, ok := m.byID[id]; ok && token.RevokedAt == nil {
		instant := currentTime
		token.RevokedAt = &instant
	}
	return nil
}
func (m *memTokenRepo) RevokeFamily(_ context.Context, familyID string, currentTime time.Time) error {
	for _, token := range m.byID {
		if token.FamilyID == familyID && token.RevokedAt == nil {
			instant := currentTime
			token.RevokedAt = &instant
		}
	}
	return nil
}
func (m *memTokenRepo) FamilyHasLiveToken(_ context.Context, familyID string, currentTime time.Time) (bool, error) {
	for _, token := range m.byID {
		if token.FamilyID == familyID && token.RevokedAt == nil && token.ExpiresAt.After(currentTime) {
			return true, nil
		}
	}
	return false, nil
}
func (m *memTokenRepo) PurgeExpiredFamilies(_ context.Context, _ string, _ time.Time) error {
	return nil
}

// --- harness ---------------------------------------------------------------

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	issuer := security.NewAccessTokenIssuer("0123456789abcdef0123456789abcdef", 15*time.Minute)
	service, err := appauth.NewService(
		newMemUserRepo(), newMemTokenRepo(),
		security.NewArgon2idHasher(), issuer, security.NewRefreshTokenFactory(),
		security.SystemClock{}, 168*time.Hour,
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	return adapterhttp.NewRouter(adapterhttp.RouterConfig{
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		AuthService:     service,
		SecureCookies:   false,
		RefreshTokenTTL: 168 * time.Hour,
	})
}

func do(t *testing.T, handler http.Handler, method, target, body string, cookies []*http.Cookie, bearer string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, target, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder.Result()
}

func errorCode(t *testing.T, resp *http.Response) string {
	t.Helper()
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decoding error envelope: %v", err)
	}
	return envelope.Error.Code
}

func refreshCookie(resp *http.Response) *http.Cookie {
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "refresh_token" {
			return cookie
		}
	}
	return nil
}

const registerBody = `{"email":"user@example.com","password":"longenough","displayName":"User"}`

// --- tests -----------------------------------------------------------------

func TestRegisterEndpoint(t *testing.T) {
	t.Run("success returns 201", func(t *testing.T) {
		handler := newTestRouter(t)
		resp := do(t, handler, http.MethodPost, "/api/v1/auth/register", registerBody, nil, "")
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want 201", resp.StatusCode)
		}
	})

	t.Run("duplicate email returns 409 EMAIL_TAKEN", func(t *testing.T) {
		handler := newTestRouter(t)
		do(t, handler, http.MethodPost, "/api/v1/auth/register", registerBody, nil, "")
		resp := do(t, handler, http.MethodPost, "/api/v1/auth/register", registerBody, nil, "")
		if resp.StatusCode != http.StatusConflict || errorCode(t, resp) != "EMAIL_TAKEN" {
			t.Fatalf("status = %d, want 409 EMAIL_TAKEN", resp.StatusCode)
		}
	})

	t.Run("short password returns 422 VALIDATION_FAILED", func(t *testing.T) {
		handler := newTestRouter(t)
		body := `{"email":"user@example.com","password":"short","displayName":"User"}`
		resp := do(t, handler, http.MethodPost, "/api/v1/auth/register", body, nil, "")
		if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, resp) != "VALIDATION_FAILED" {
			t.Fatalf("status = %d, want 422 VALIDATION_FAILED", resp.StatusCode)
		}
	})

	t.Run("malformed JSON returns 422", func(t *testing.T) {
		handler := newTestRouter(t)
		resp := do(t, handler, http.MethodPost, "/api/v1/auth/register", `{not json`, nil, "")
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", resp.StatusCode)
		}
	})
}

func TestLoginEndpoint(t *testing.T) {
	t.Run("success returns token and refresh cookie", func(t *testing.T) {
		handler := newTestRouter(t)
		do(t, handler, http.MethodPost, "/api/v1/auth/register", registerBody, nil, "")
		resp := do(t, handler, http.MethodPost, "/api/v1/auth/login",
			`{"email":"user@example.com","password":"longenough"}`, nil, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if refreshCookie(resp) == nil {
			t.Error("expected a refresh_token cookie")
		}
	})

	t.Run("wrong password returns 401 INVALID_CREDENTIALS", func(t *testing.T) {
		handler := newTestRouter(t)
		do(t, handler, http.MethodPost, "/api/v1/auth/register", registerBody, nil, "")
		resp := do(t, handler, http.MethodPost, "/api/v1/auth/login",
			`{"email":"user@example.com","password":"wrongpass"}`, nil, "")
		if resp.StatusCode != http.StatusUnauthorized || errorCode(t, resp) != "INVALID_CREDENTIALS" {
			t.Fatalf("status = %d, want 401 INVALID_CREDENTIALS", resp.StatusCode)
		}
	})
}

func TestRefreshEndpoint(t *testing.T) {
	t.Run("rotates and sets a new cookie", func(t *testing.T) {
		handler := newTestRouter(t)
		do(t, handler, http.MethodPost, "/api/v1/auth/register", registerBody, nil, "")
		login := do(t, handler, http.MethodPost, "/api/v1/auth/login",
			`{"email":"user@example.com","password":"longenough"}`, nil, "")
		cookie := refreshCookie(login)

		resp := do(t, handler, http.MethodPost, "/api/v1/auth/refresh", "", []*http.Cookie{cookie}, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if refreshCookie(resp) == nil || refreshCookie(resp).Value == cookie.Value {
			t.Error("expected a rotated refresh cookie")
		}
	})

	t.Run("missing cookie returns 401 TOKEN_INVALID", func(t *testing.T) {
		handler := newTestRouter(t)
		resp := do(t, handler, http.MethodPost, "/api/v1/auth/refresh", "", nil, "")
		if resp.StatusCode != http.StatusUnauthorized || errorCode(t, resp) != "TOKEN_INVALID" {
			t.Fatalf("status = %d, want 401 TOKEN_INVALID", resp.StatusCode)
		}
	})
}

func TestMeEndpoint(t *testing.T) {
	t.Run("returns the user with a valid token", func(t *testing.T) {
		handler := newTestRouter(t)
		do(t, handler, http.MethodPost, "/api/v1/auth/register", registerBody, nil, "")
		login := do(t, handler, http.MethodPost, "/api/v1/auth/login",
			`{"email":"user@example.com","password":"longenough"}`, nil, "")
		var loginBody struct {
			AccessToken string `json:"accessToken"`
		}
		if err := json.NewDecoder(login.Body).Decode(&loginBody); err != nil {
			t.Fatalf("decode login: %v", err)
		}

		resp := do(t, handler, http.MethodGet, "/api/v1/me", "", nil, loginBody.AccessToken)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	})

	t.Run("missing token returns 401 TOKEN_INVALID", func(t *testing.T) {
		handler := newTestRouter(t)
		resp := do(t, handler, http.MethodGet, "/api/v1/me", "", nil, "")
		if resp.StatusCode != http.StatusUnauthorized || errorCode(t, resp) != "TOKEN_INVALID" {
			t.Fatalf("status = %d, want 401 TOKEN_INVALID", resp.StatusCode)
		}
	})
}

func TestLogoutEndpoint(t *testing.T) {
	handler := newTestRouter(t)
	do(t, handler, http.MethodPost, "/api/v1/auth/register", registerBody, nil, "")
	login := do(t, handler, http.MethodPost, "/api/v1/auth/login",
		`{"email":"user@example.com","password":"longenough"}`, nil, "")
	cookie := refreshCookie(login)

	resp := do(t, handler, http.MethodPost, "/api/v1/auth/logout", "", []*http.Cookie{cookie}, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}

	// The revoked token must no longer refresh.
	after := do(t, handler, http.MethodPost, "/api/v1/auth/refresh", "", []*http.Cookie{cookie}, "")
	if after.StatusCode != http.StatusUnauthorized {
		t.Errorf("refresh after logout status = %d, want 401", after.StatusCode)
	}
}

func TestRateLimit(t *testing.T) {
	handler := newTestRouter(t)

	// Probe via /refresh with no cookie: it is rejected fast (no argon2 work), so
	// the per-IP burst is exhausted before the token bucket can refill — keeping
	// the assertion deterministic even under the race detector's slowdown.
	var lastStatus int
	for attempt := 0; attempt < rateLimitProbe; attempt++ {
		resp := do(t, handler, http.MethodPost, "/api/v1/auth/refresh", "", nil, "")
		lastStatus = resp.StatusCode
		_ = resp.Body.Close()
	}
	if lastStatus != http.StatusTooManyRequests {
		t.Errorf("attempt %d status = %d, want 429", rateLimitProbe, lastStatus)
	}
}

const rateLimitProbe = 11
