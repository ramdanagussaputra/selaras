package auth

import (
	"fmt"
	"net/mail"
	"strings"
)

// MinPasswordLength is the minimum accepted password length (spec 02-auth
// Business Rule 1). Hashing parameters live in the adapter; this is the pure rule.
const MinPasswordLength = 8

// ValidateRegistration checks the registration inputs and returns the first
// failing field as a *ValidationError, or nil when all are acceptable.
func ValidateRegistration(email, password, displayName string) error {
	if err := ValidateEmail(email); err != nil {
		return err
	}
	if err := ValidatePassword(password); err != nil {
		return err
	}
	return ValidateDisplayName(displayName)
}

// ValidateEmail requires a syntactically valid, single email address.
func ValidateEmail(email string) error {
	if _, err := mail.ParseAddress(email); err != nil {
		return &ValidationError{Field: "email", Message: "must be a valid email address"}
	}
	return nil
}

// ValidatePassword enforces the minimum length policy.
func ValidatePassword(password string) error {
	if len(password) < MinPasswordLength {
		return &ValidationError{
			Field:   "password",
			Message: fmt.Sprintf("must be at least %d characters", MinPasswordLength),
		}
	}
	return nil
}

// ValidateDisplayName requires a non-blank display name.
func ValidateDisplayName(displayName string) error {
	if strings.TrimSpace(displayName) == "" {
		return &ValidationError{Field: "displayName", Message: "must not be empty"}
	}
	return nil
}
