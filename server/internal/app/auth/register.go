package auth

import (
	"context"
	"fmt"

	domain "github.com/ramdanaguss/selaras/server/internal/domain/auth"
)

// Register validates the inputs, hashes the password, and creates the user.
// Returns domain.ErrEmailTaken on a duplicate email and a *domain.ValidationError
// on bad input.
func (s *Service) Register(ctx context.Context, email, password, displayName string) (domain.User, error) {
	if err := domain.ValidateRegistration(email, password, displayName); err != nil {
		return domain.User{}, err
	}

	passwordHash, err := s.hasher.Hash(password)
	if err != nil {
		return domain.User{}, fmt.Errorf("hashing password: %w", err)
	}

	user := domain.User{
		ID:           s.newID(),
		Email:        email,
		DisplayName:  displayName,
		PasswordHash: passwordHash,
		CreatedAt:    s.clock.Now(),
	}

	created, err := s.users.Create(ctx, user)
	if err != nil {
		return domain.User{}, fmt.Errorf("creating user: %w", err)
	}

	return created, nil
}
