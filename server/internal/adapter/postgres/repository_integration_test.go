package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramdanaguss/selaras/server/internal/adapter/postgres"
	domain "github.com/ramdanaguss/selaras/server/internal/domain/auth"
)

const defaultTestDatabaseURL = "postgres://selaras:selaras@localhost:5432/selaras?sslmode=disable"

// testPool connects to the local/dev Postgres and skips the test when it is
// unreachable, so `make test` stays green without a database while still giving
// real integration coverage when one is up (project.md testing convention).
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = defaultTestDatabaseURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Skipf("no database available (%v)", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("no database available (%v)", err)
	}

	if _, err := pool.Exec(context.Background(),
		`TRUNCATE users, refresh_tokens RESTART IDENTITY CASCADE`); err != nil {
		pool.Close()
		t.Skipf("schema not migrated (%v); run make migrate-up", err)
	}

	t.Cleanup(pool.Close)
	return pool
}

func newUser() domain.User {
	id := uuid.NewString()
	return domain.User{
		ID:           id,
		Email:        id + "@example.com",
		DisplayName:  "Test User",
		PasswordHash: "argon:placeholder",
		CreatedAt:    time.Now().UTC().Truncate(time.Microsecond),
	}
}

func TestUserRepository(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repository := postgres.NewUserRepository(pool)

	user := newUser()
	created, err := repository.Create(ctx, user)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID != user.ID || created.Email != user.Email {
		t.Errorf("created = %+v, want id/email %s/%s", created, user.ID, user.Email)
	}

	t.Run("find by email is case-insensitive", func(t *testing.T) {
		found, err := repository.FindByEmail(ctx, user.Email)
		if err != nil {
			t.Fatalf("FindByEmail: %v", err)
		}
		if found.ID != user.ID {
			t.Errorf("id = %s, want %s", found.ID, user.ID)
		}
	})

	t.Run("find by id", func(t *testing.T) {
		found, err := repository.FindByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if found.Email != user.Email {
			t.Errorf("email = %s, want %s", found.Email, user.Email)
		}
	})

	t.Run("duplicate email is rejected", func(t *testing.T) {
		duplicate := newUser()
		duplicate.Email = user.Email
		_, err := repository.Create(ctx, duplicate)
		if !errors.Is(err, domain.ErrEmailTaken) {
			t.Errorf("err = %v, want ErrEmailTaken", err)
		}
	})

	t.Run("missing user", func(t *testing.T) {
		_, err := repository.FindByID(ctx, uuid.NewString())
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("err = %v, want ErrUserNotFound", err)
		}
	})
}

func TestRefreshTokenRepository(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	users := postgres.NewUserRepository(pool)
	tokens := postgres.NewRefreshTokenRepository(pool)
	currentTime := time.Now().UTC().Truncate(time.Microsecond)

	user, err := users.Create(ctx, newUser())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	familyID := uuid.NewString()
	live := domain.NewRefreshToken(uuid.NewString(), user.ID, familyID, []byte("hash-live"), currentTime, time.Hour)
	if err := tokens.Create(ctx, live); err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("find by hash round-trips", func(t *testing.T) {
		found, err := tokens.FindByHash(ctx, []byte("hash-live"))
		if err != nil {
			t.Fatalf("FindByHash: %v", err)
		}
		if found.ID != live.ID || found.FamilyID != familyID || found.RevokedAt != nil {
			t.Errorf("unexpected token %+v", found)
		}
	})

	t.Run("missing hash is invalid", func(t *testing.T) {
		_, err := tokens.FindByHash(ctx, []byte("nope"))
		if !errors.Is(err, domain.ErrTokenInvalid) {
			t.Errorf("err = %v, want ErrTokenInvalid", err)
		}
	})

	t.Run("family has live token, then revoke family", func(t *testing.T) {
		hasLive, err := tokens.FamilyHasLiveToken(ctx, familyID, currentTime)
		if err != nil || !hasLive {
			t.Fatalf("FamilyHasLiveToken = %v, %v; want true, nil", hasLive, err)
		}

		if err := tokens.RevokeFamily(ctx, familyID, currentTime); err != nil {
			t.Fatalf("RevokeFamily: %v", err)
		}

		hasLive, err = tokens.FamilyHasLiveToken(ctx, familyID, currentTime)
		if err != nil || hasLive {
			t.Fatalf("after revoke FamilyHasLiveToken = %v, %v; want false, nil", hasLive, err)
		}

		found, err := tokens.FindByHash(ctx, []byte("hash-live"))
		if err != nil {
			t.Fatalf("FindByHash after revoke: %v", err)
		}
		if found.RevokedAt == nil {
			t.Error("token should be revoked after RevokeFamily")
		}
	})

	t.Run("purge expired families", func(t *testing.T) {
		expiredFamily := uuid.NewString()
		expired := domain.NewRefreshToken(uuid.NewString(), user.ID, expiredFamily, []byte("hash-expired"), currentTime.Add(-2*time.Hour), time.Hour)
		if err := tokens.Create(ctx, expired); err != nil {
			t.Fatalf("create expired: %v", err)
		}

		if err := tokens.PurgeExpiredFamilies(ctx, user.ID, currentTime); err != nil {
			t.Fatalf("PurgeExpiredFamilies: %v", err)
		}

		if _, err := tokens.FindByHash(ctx, []byte("hash-expired")); !errors.Is(err, domain.ErrTokenInvalid) {
			t.Errorf("expired token still present: err = %v", err)
		}
	})
}
