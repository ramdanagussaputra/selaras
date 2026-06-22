package security_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ramdanaguss/selaras/server/internal/adapter/security"
	domain "github.com/ramdanaguss/selaras/server/internal/domain/auth"
)

func TestArgon2idHasher(t *testing.T) {
	hasher := security.NewArgon2idHasher()

	encoded, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	t.Run("correct password verifies", func(t *testing.T) {
		match, err := hasher.Verify("correct horse battery staple", encoded)
		if err != nil || !match {
			t.Errorf("Verify = %v, %v; want true, nil", match, err)
		}
	})

	t.Run("wrong password does not verify", func(t *testing.T) {
		match, err := hasher.Verify("wrong password", encoded)
		if err != nil || match {
			t.Errorf("Verify = %v, %v; want false, nil", match, err)
		}
	})

	t.Run("malformed hash errors", func(t *testing.T) {
		if _, err := hasher.Verify("password", "not-a-phc-string"); err == nil {
			t.Error("expected error for malformed hash")
		}
	})

	t.Run("salts differ between hashes", func(t *testing.T) {
		other, err := hasher.Hash("correct horse battery staple")
		if err != nil {
			t.Fatalf("Hash: %v", err)
		}
		if other == encoded {
			t.Error("two hashes of the same password should differ (random salt)")
		}
	})
}

func TestAccessTokenIssuer(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef"
	issuer := security.NewAccessTokenIssuer(secret, time.Hour)
	currentTime := time.Now()

	t.Run("issue then verify round-trips the subject", func(t *testing.T) {
		token, err := issuer.Issue("user-123", currentTime)
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		userID, err := issuer.Verify(token)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if userID != "user-123" {
			t.Errorf("subject = %q, want user-123", userID)
		}
	})

	t.Run("expired token is reported as expired", func(t *testing.T) {
		expired, err := issuer.Issue("user-123", currentTime.Add(-2*time.Hour)) // exp 1h in the past
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		_, err = issuer.Verify(expired)
		if !errors.Is(err, domain.ErrTokenExpired) {
			t.Errorf("err = %v, want ErrTokenExpired", err)
		}
	})

	t.Run("garbage token is invalid", func(t *testing.T) {
		if _, err := issuer.Verify("not.a.jwt"); !errors.Is(err, domain.ErrTokenInvalid) {
			t.Errorf("err = %v, want ErrTokenInvalid", err)
		}
	})

	t.Run("alg none is rejected (alg-confusion defense)", func(t *testing.T) {
		unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{Subject: "attacker"}).
			SignedString(jwt.UnsafeAllowNoneSignatureType)
		if err != nil {
			t.Fatalf("crafting none-token: %v", err)
		}
		if _, err := issuer.Verify(unsigned); !errors.Is(err, domain.ErrTokenInvalid) {
			t.Errorf("err = %v, want ErrTokenInvalid", err)
		}
	})
}

func TestRefreshTokenFactory(t *testing.T) {
	factory := security.NewRefreshTokenFactory()

	raw, hash, err := factory.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if raw == "" {
		t.Error("raw token is empty")
	}
	if len(hash) != 32 {
		t.Errorf("hash length = %d, want 32 (sha256)", len(hash))
	}
	if !bytes.Equal(hash, factory.Hash(raw)) {
		t.Error("Hash(raw) should match the hash returned by Generate")
	}

	otherRaw, _, err := factory.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if otherRaw == raw {
		t.Error("two generated tokens should differ")
	}
}
