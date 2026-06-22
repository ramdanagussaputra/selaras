package auth_test

import (
	"errors"
	"testing"

	"github.com/ramdanaguss/selaras/server/internal/domain/auth"
)

func TestValidateRegistration(t *testing.T) {
	tests := []struct {
		name        string
		email       string
		password    string
		displayName string
		wantField   string // "" means no error expected
	}{
		{name: "valid", email: "user@example.com", password: "longenough", displayName: "User", wantField: ""},
		{name: "bad email", email: "not-an-email", password: "longenough", displayName: "User", wantField: "email"},
		{name: "short password", email: "user@example.com", password: "short", displayName: "User", wantField: "password"},
		{name: "blank display name", email: "user@example.com", password: "longenough", displayName: "   ", wantField: "displayName"},
		{name: "email checked before password", email: "bad", password: "short", displayName: "User", wantField: "email"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := auth.ValidateRegistration(testCase.email, testCase.password, testCase.displayName)

			if testCase.wantField == "" {
				if err != nil {
					t.Fatalf("ValidateRegistration() = %v, want nil", err)
				}
				return
			}

			var validationErr *auth.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("ValidateRegistration() = %v, want *ValidationError", err)
			}
			if validationErr.Field != testCase.wantField {
				t.Errorf("field = %q, want %q", validationErr.Field, testCase.wantField)
			}
		})
	}
}
