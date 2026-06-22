package auth_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	appauth "github.com/ramdanaguss/selaras/server/internal/app/auth"
	domain "github.com/ramdanaguss/selaras/server/internal/domain/auth"
)

// --- fakes -----------------------------------------------------------------

type fakeClock struct{ currentTime time.Time }

func (c fakeClock) Now() time.Time { return c.currentTime }

type fakeHasher struct{}

func (fakeHasher) Hash(password string) (string, error) { return "argon:" + password, nil }
func (fakeHasher) Verify(password, encodedHash string) (bool, error) {
	return encodedHash == "argon:"+password, nil
}

type fakeAccessTokens struct{}

func (fakeAccessTokens) Issue(userID string, _ time.Time) (string, error) {
	return "access:" + userID, nil
}
func (fakeAccessTokens) Verify(token string) (string, error) {
	const prefix = "access:"
	if len(token) <= len(prefix) || token[:len(prefix)] != prefix {
		return "", domain.ErrTokenInvalid
	}
	return token[len(prefix):], nil
}

type fakeRefreshFactory struct{ counter int }

func (f *fakeRefreshFactory) Generate() (string, []byte, error) {
	f.counter++
	raw := fmt.Sprintf("raw-%d", f.counter)
	return raw, []byte(raw), nil
}
func (f *fakeRefreshFactory) Hash(raw string) []byte { return []byte(raw) }

type fakeUserRepo struct {
	byEmail map[string]domain.User
	byID    map[string]domain.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byEmail: map[string]domain.User{}, byID: map[string]domain.User{}}
}
func (r *fakeUserRepo) Create(_ context.Context, user domain.User) (domain.User, error) {
	if _, exists := r.byEmail[user.Email]; exists {
		return domain.User{}, domain.ErrEmailTaken
	}
	r.byEmail[user.Email] = user
	r.byID[user.ID] = user
	return user, nil
}
func (r *fakeUserRepo) FindByEmail(_ context.Context, email string) (domain.User, error) {
	if user, ok := r.byEmail[email]; ok {
		return user, nil
	}
	return domain.User{}, domain.ErrUserNotFound
}
func (r *fakeUserRepo) FindByID(_ context.Context, id string) (domain.User, error) {
	if user, ok := r.byID[id]; ok {
		return user, nil
	}
	return domain.User{}, domain.ErrUserNotFound
}

type fakeTokenRepo struct {
	byID map[string]*domain.RefreshToken
}

func newFakeTokenRepo() *fakeTokenRepo {
	return &fakeTokenRepo{byID: map[string]*domain.RefreshToken{}}
}
func (r *fakeTokenRepo) Create(_ context.Context, token domain.RefreshToken) error {
	stored := token
	r.byID[token.ID] = &stored
	return nil
}
func (r *fakeTokenRepo) FindByHash(_ context.Context, tokenHash []byte) (domain.RefreshToken, error) {
	for _, token := range r.byID {
		if bytes.Equal(token.TokenHash, tokenHash) {
			return *token, nil
		}
	}
	return domain.RefreshToken{}, domain.ErrTokenInvalid
}
func (r *fakeTokenRepo) Revoke(_ context.Context, id string, currentTime time.Time) error {
	if token, ok := r.byID[id]; ok && token.RevokedAt == nil {
		instant := currentTime
		token.RevokedAt = &instant
	}
	return nil
}
func (r *fakeTokenRepo) RevokeFamily(_ context.Context, familyID string, currentTime time.Time) error {
	for _, token := range r.byID {
		if token.FamilyID == familyID && token.RevokedAt == nil {
			instant := currentTime
			token.RevokedAt = &instant
		}
	}
	return nil
}
func (r *fakeTokenRepo) FamilyHasLiveToken(_ context.Context, familyID string, currentTime time.Time) (bool, error) {
	for _, token := range r.byID {
		if token.FamilyID == familyID && token.RevokedAt == nil && token.ExpiresAt.After(currentTime) {
			return true, nil
		}
	}
	return false, nil
}
func (r *fakeTokenRepo) PurgeExpiredFamilies(_ context.Context, userID string, currentTime time.Time) error {
	for id, token := range r.byID {
		if token.UserID == userID && !token.ExpiresAt.After(currentTime) {
			delete(r.byID, id)
		}
	}
	return nil
}

// --- harness ---------------------------------------------------------------

const (
	testFamily = "fam"
	testUser   = "user"
)

type harness struct {
	service *appauth.Service
	users   *fakeUserRepo
	tokens  *fakeTokenRepo
	clock   fakeClock
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	users := newFakeUserRepo()
	tokens := newFakeTokenRepo()
	clock := fakeClock{currentTime: time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)}

	service, err := appauth.NewService(
		users, tokens, fakeHasher{}, fakeAccessTokens{}, &fakeRefreshFactory{}, clock, 168*time.Hour,
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return &harness{service: service, users: users, tokens: tokens, clock: clock}
}

// seedToken inserts a refresh token directly so refresh paths can be exercised.
// All seeded tokens share one user and family so head/replay scenarios interact.
func seedToken(t *testing.T, fixture *harness, raw string, revokedAgo, expiresIn time.Duration) {
	t.Helper()
	token := domain.RefreshToken{
		ID:        "id-" + raw,
		UserID:    testUser,
		FamilyID:  testFamily,
		TokenHash: []byte(raw),
		ExpiresAt: fixture.clock.currentTime.Add(expiresIn),
		CreatedAt: fixture.clock.currentTime.Add(-time.Minute),
	}
	if revokedAgo > 0 {
		instant := fixture.clock.currentTime.Add(-revokedAgo)
		token.RevokedAt = &instant
	}
	if err := fixture.tokens.Create(context.Background(), token); err != nil {
		t.Fatalf("seedToken: %v", err)
	}
}

// --- tests -----------------------------------------------------------------

func TestRegister(t *testing.T) {
	ctx := context.Background()

	t.Run("creates a user with a hashed password", func(t *testing.T) {
		fixture := newHarness(t)
		user, err := fixture.service.Register(ctx, "user@example.com", "longenough", "User")
		if err != nil {
			t.Fatalf("Register: %v", err)
		}
		if user.PasswordHash != "argon:longenough" {
			t.Errorf("password hash = %q, want %q", user.PasswordHash, "argon:longenough")
		}
	})

	t.Run("rejects a duplicate email", func(t *testing.T) {
		fixture := newHarness(t)
		if _, err := fixture.service.Register(ctx, "user@example.com", "longenough", "User"); err != nil {
			t.Fatalf("first Register: %v", err)
		}
		_, err := fixture.service.Register(ctx, "user@example.com", "longenough", "Other")
		if !errors.Is(err, domain.ErrEmailTaken) {
			t.Errorf("err = %v, want ErrEmailTaken", err)
		}
	})

	t.Run("rejects invalid input", func(t *testing.T) {
		fixture := newHarness(t)
		_, err := fixture.service.Register(ctx, "user@example.com", "short", "User")
		var validationErr *domain.ValidationError
		if !errors.As(err, &validationErr) {
			t.Errorf("err = %v, want *ValidationError", err)
		}
	})
}

func TestLogin(t *testing.T) {
	ctx := context.Background()

	seedUser := func(fixture *harness) domain.User {
		user, err := fixture.service.Register(ctx, "user@example.com", "longenough", "User")
		if err != nil {
			t.Fatalf("seed Register: %v", err)
		}
		return user
	}

	t.Run("issues tokens on valid credentials", func(t *testing.T) {
		fixture := newHarness(t)
		user := seedUser(fixture)

		tokens, err := fixture.service.Login(ctx, "user@example.com", "longenough")
		if err != nil {
			t.Fatalf("Login: %v", err)
		}
		if tokens.AccessToken != "access:"+user.ID {
			t.Errorf("access token = %q", tokens.AccessToken)
		}
		if tokens.RefreshToken == "" {
			t.Error("refresh token is empty")
		}
		if len(fixture.tokens.byID) != 1 {
			t.Errorf("refresh tokens stored = %d, want 1", len(fixture.tokens.byID))
		}
	})

	t.Run("wrong password is rejected", func(t *testing.T) {
		fixture := newHarness(t)
		seedUser(fixture)
		_, err := fixture.service.Login(ctx, "user@example.com", "wrongpass")
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Errorf("err = %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("unknown email is rejected identically", func(t *testing.T) {
		fixture := newHarness(t)
		_, err := fixture.service.Login(ctx, "nobody@example.com", "whatever")
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Errorf("err = %v, want ErrInvalidCredentials", err)
		}
	})
}

func TestRefresh(t *testing.T) {
	ctx := context.Background()

	t.Run("rotates a live token", func(t *testing.T) {
		fixture := newHarness(t)
		seedToken(t, fixture, "live", 0, time.Hour)

		result, err := fixture.service.Refresh(ctx, "live")
		if err != nil {
			t.Fatalf("Refresh: %v", err)
		}
		if result.AccessToken != "access:"+testUser || result.RefreshToken == "" {
			t.Errorf("unexpected result %+v", result)
		}
		if fixture.tokens.byID["id-live"].RevokedAt == nil {
			t.Error("presented token should be revoked after rotation")
		}
		if len(fixture.tokens.byID) != 2 {
			t.Errorf("token count = %d, want 2 (old + successor)", len(fixture.tokens.byID))
		}
	})

	t.Run("invalid token is rejected", func(t *testing.T) {
		fixture := newHarness(t)
		_, err := fixture.service.Refresh(ctx, "does-not-exist")
		if !errors.Is(err, domain.ErrTokenInvalid) {
			t.Errorf("err = %v, want ErrTokenInvalid", err)
		}
	})

	t.Run("expired token is rejected", func(t *testing.T) {
		fixture := newHarness(t)
		seedToken(t, fixture, "old", 0, -time.Second) // already expired
		_, err := fixture.service.Refresh(ctx, "old")
		if !errors.Is(err, domain.ErrTokenInvalid) {
			t.Errorf("err = %v, want ErrTokenInvalid", err)
		}
	})

	t.Run("reuse outside grace revokes the family", func(t *testing.T) {
		fixture := newHarness(t)
		seedToken(t, fixture, "head", 0, time.Hour)           // live head keeps family intact
		seedToken(t, fixture, "stolen", time.Hour, time.Hour) // revoked long ago (outside grace)

		_, err := fixture.service.Refresh(ctx, "stolen")
		if !errors.Is(err, domain.ErrTokenReused) {
			t.Fatalf("err = %v, want ErrTokenReused", err)
		}
		if fixture.tokens.byID["id-head"].RevokedAt == nil {
			t.Error("family head should be revoked after reuse detection")
		}
	})

	t.Run("within-grace replay on an intact family is tolerated", func(t *testing.T) {
		fixture := newHarness(t)
		seedToken(t, fixture, "head", 0, time.Hour)              // live successor
		seedToken(t, fixture, "raced", 2*time.Second, time.Hour) // just-revoked, within grace

		result, err := fixture.service.Refresh(ctx, "raced")
		if err != nil {
			t.Fatalf("Refresh tolerated: %v", err)
		}
		if result.RefreshToken == "" {
			t.Error("expected a fresh refresh token on tolerated replay")
		}
		if fixture.tokens.byID["id-head"].RevokedAt != nil {
			t.Error("family must NOT be revoked on a tolerated replay")
		}
	})
}

func TestLogout(t *testing.T) {
	ctx := context.Background()

	t.Run("revokes a known token", func(t *testing.T) {
		fixture := newHarness(t)
		seedToken(t, fixture, "live", 0, time.Hour)
		if err := fixture.service.Logout(ctx, "live"); err != nil {
			t.Fatalf("Logout: %v", err)
		}
		if fixture.tokens.byID["id-live"].RevokedAt == nil {
			t.Error("token should be revoked after logout")
		}
	})

	t.Run("unknown token is a no-op", func(t *testing.T) {
		fixture := newHarness(t)
		if err := fixture.service.Logout(ctx, "nope"); err != nil {
			t.Errorf("Logout unknown = %v, want nil", err)
		}
	})
}

func TestAuthenticateAndMe(t *testing.T) {
	ctx := context.Background()
	fixture := newHarness(t)
	user, err := fixture.service.Register(ctx, "user@example.com", "longenough", "User")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	userID, err := fixture.service.Authenticate("access:" + user.ID)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if userID != user.ID {
		t.Errorf("userID = %q, want %q", userID, user.ID)
	}

	loaded, err := fixture.service.Me(ctx, userID)
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if loaded.Email != "user@example.com" {
		t.Errorf("email = %q", loaded.Email)
	}
}
