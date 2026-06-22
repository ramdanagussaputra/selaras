// Package security holds the driven adapters that implement the auth crypto
// ports: argon2id password hashing, HS256 JWT access tokens, and random refresh
// tokens. Keeping them out of adapter/http leaves the HTTP layer about HTTP.
package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id cost parameters (spec 02-auth Business Rule 1; design D2). Parallelism
// is pinned to 1 (OWASP guidance, deterministic across environments) rather than
// the host core count. ADR-0002 records the rationale.
const (
	argon2Memory      = 64 * 1024 // KiB → 64 MiB
	argon2Iterations  = 2
	argon2Parallelism = 1
	argon2SaltLength  = 16
	argon2KeyLength   = 32
)

// errMalformedHash is returned when a stored hash is not a valid argon2id PHC
// string. It is internal; callers see a generic verification failure.
var errMalformedHash = errors.New("malformed argon2id hash")

// Argon2idHasher implements domain/auth.PasswordHasher.
type Argon2idHasher struct{}

// NewArgon2idHasher constructs the hasher.
func NewArgon2idHasher() Argon2idHasher { return Argon2idHasher{} }

// Hash returns a self-describing PHC string: the parameters travel with the hash,
// so a future cost bump re-hashes transparently on next login.
func (Argon2idHasher) Hash(password string) (string, error) {
	salt := make([]byte, argon2SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("reading salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, argon2Iterations, argon2Memory, argon2Parallelism, argon2KeyLength)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argon2Memory, argon2Iterations, argon2Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// Verify recomputes the hash with the parameters parsed from encodedHash and
// compares in constant time.
func (Argon2idHasher) Verify(password, encodedHash string) (bool, error) {
	memory, iterations, parallelism, salt, key, err := decodeHash(encodedHash)
	if err != nil {
		return false, err
	}

	candidate := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(key)))

	return subtle.ConstantTimeCompare(candidate, key) == 1, nil
}

func decodeHash(encodedHash string) (memory, iterations uint32, parallelism uint8, salt, key []byte, err error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return 0, 0, 0, nil, nil, errMalformedHash
	}

	var version int
	if _, err = fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return 0, 0, 0, nil, nil, errMalformedHash
	}

	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return 0, 0, 0, nil, nil, errMalformedHash
	}

	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return 0, 0, 0, nil, nil, errMalformedHash
	}

	key, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return 0, 0, 0, nil, nil, errMalformedHash
	}

	return memory, iterations, parallelism, salt, key, nil
}
