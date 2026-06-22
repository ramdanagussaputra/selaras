package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "github.com/ramdanaguss/selaras/server/internal/domain/auth"
)

// uniqueViolation is Postgres' SQLSTATE for a unique-constraint violation.
const uniqueViolation = "23505"

// UserRepository implements domain/auth.UserRepository over a pgx pool.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository wraps a pool as a user repository.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Create inserts a user, translating a duplicate-email constraint violation into
// domain.ErrEmailTaken.
func (r *UserRepository) Create(ctx context.Context, user domain.User) (domain.User, error) {
	const query = `
		INSERT INTO users (id, email, password_hash, display_name, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, email::text, password_hash, display_name, created_at`

	created, err := scanUser(r.pool.QueryRow(ctx, query,
		user.ID, user.Email, user.PasswordHash, user.DisplayName, user.CreatedAt,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return domain.User{}, domain.ErrEmailTaken
		}
		return domain.User{}, fmt.Errorf("inserting user: %w", err)
	}

	return created, nil
}

// FindByEmail returns the user with the given email (case-insensitive via citext),
// or domain.ErrUserNotFound.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (domain.User, error) {
	const query = `
		SELECT id::text, email::text, password_hash, display_name, created_at
		FROM users WHERE email = $1`

	user, err := scanUser(r.pool.QueryRow(ctx, query, email))
	return mapUserNotFound(user, err)
}

// FindByID returns the user with the given id, or domain.ErrUserNotFound.
func (r *UserRepository) FindByID(ctx context.Context, id string) (domain.User, error) {
	const query = `
		SELECT id::text, email::text, password_hash, display_name, created_at
		FROM users WHERE id = $1`

	user, err := scanUser(r.pool.QueryRow(ctx, query, id))
	return mapUserNotFound(user, err)
}

// row is the minimal surface of pgx.Row needed for scanning, so helpers can be
// shared across QueryRow call sites.
type row interface {
	Scan(dest ...any) error
}

func scanUser(source row) (domain.User, error) {
	var user domain.User
	err := source.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.DisplayName, &user.CreatedAt)
	return user, err
}

func mapUserNotFound(user domain.User, err error) (domain.User, error) {
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, domain.ErrUserNotFound
		}
		return domain.User{}, fmt.Errorf("querying user: %w", err)
	}
	return user, nil
}
