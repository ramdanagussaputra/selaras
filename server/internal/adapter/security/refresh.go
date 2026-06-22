package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// refreshTokenBytes is the entropy of a raw refresh token (spec: 32 random bytes).
const refreshTokenBytes = 32

// RefreshTokenFactory implements domain/auth.RefreshTokenFactory. The raw token
// is base64url-encoded random bytes; only its SHA-256 is ever stored.
type RefreshTokenFactory struct{}

// NewRefreshTokenFactory constructs the factory.
func NewRefreshTokenFactory() RefreshTokenFactory { return RefreshTokenFactory{} }

// Generate returns a fresh raw token and its SHA-256 hash.
func (RefreshTokenFactory) Generate() (string, []byte, error) {
	randomBytes := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", nil, fmt.Errorf("reading random bytes: %w", err)
	}

	raw := base64.RawURLEncoding.EncodeToString(randomBytes)
	hash := sha256.Sum256([]byte(raw))
	return raw, hash[:], nil
}

// Hash returns the SHA-256 of a raw token, for looking up a presented cookie.
func (RefreshTokenFactory) Hash(raw string) []byte {
	hash := sha256.Sum256([]byte(raw))
	return hash[:]
}
