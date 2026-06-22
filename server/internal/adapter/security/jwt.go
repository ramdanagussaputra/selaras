package security

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	domain "github.com/ramdanaguss/selaras/server/internal/domain/auth"
)

// AccessTokenIssuer implements domain/auth.AccessTokenIssuer with HS256 JWTs.
type AccessTokenIssuer struct {
	secret []byte
	ttl    time.Duration
}

// NewAccessTokenIssuer constructs the issuer from the signing secret and TTL.
func NewAccessTokenIssuer(secret string, ttl time.Duration) *AccessTokenIssuer {
	return &AccessTokenIssuer{secret: []byte(secret), ttl: ttl}
}

// Issue mints an HS256 token whose claims are exactly sub, iat, and exp.
func (i *AccessTokenIssuer) Issue(userID string, now time.Time) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("signing access token: %w", err)
	}
	return signed, nil
}

// Verify parses and validates the token, pinning the algorithm to HS256 to close
// the alg-confusion / "none" attack class. Returns the subject, or
// domain.ErrTokenExpired / domain.ErrTokenInvalid.
func (i *AccessTokenIssuer) Verify(tokenString string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(
		tokenString, claims,
		func(*jwt.Token) (any, error) { return i.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", domain.ErrTokenExpired
		}
		return "", domain.ErrTokenInvalid
	}

	if claims.Subject == "" {
		return "", domain.ErrTokenInvalid
	}
	return claims.Subject, nil
}
