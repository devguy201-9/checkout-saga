package jwt_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"

	"github.com/devguy201-9/checkout-saga/pkg/jwt"
)

// _testSecret is >= 32 chars so it mirrors what config validation guarantees.
const (
	_testSecret = "test-secret-that-is-at-least-32b!!"
	_testUserID = "11111111-1111-1111-1111-111111111111"
	_expiry     = time.Hour
)

func TestManager_GenerateParse_RoundTrip(t *testing.T) {
	t.Parallel()

	mgr := jwt.NewManager(_testSecret, _expiry)

	token, err := mgr.Generate(_testUserID, time.Now())
	if err != nil {
		t.Fatalf("Generate: unexpected error: %v", err)
	}

	got, err := mgr.Parse(token)
	if err != nil {
		t.Fatalf("Parse: unexpected error: %v", err)
	}

	if got != _testUserID {
		t.Fatalf("subject mismatch: want %q, got %q", _testUserID, got)
	}
}

func TestManager_Parse_RejectsExpired(t *testing.T) {
	t.Parallel()

	// Negative expiry => exp is in the past the moment it is signed. Deterministic,
	// no time.Sleep.
	mgr := jwt.NewManager(_testSecret, -time.Minute)

	token, err := mgr.Generate(_testUserID, time.Now())
	if err != nil {
		t.Fatalf("Generate: unexpected error: %v", err)
	}

	if _, err := mgr.Parse(token); !errors.Is(err, jwt.ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken for expired token, got %v", err)
	}
}

func TestManager_Parse_RejectsTampered(t *testing.T) {
	t.Parallel()

	mgr := jwt.NewManager(_testSecret, _expiry)

	token, err := mgr.Generate(_testUserID, time.Now())
	if err != nil {
		t.Fatalf("Generate: unexpected error: %v", err)
	}

	// Mutate the FIRST char of the signature segment to a different base64url
	// char. The first char always carries meaningful signature bits (unlike the
	// last, whose trailing bits are padding), so this reliably breaks the
	// signature regardless of the run-to-run token.
	parts := strings.Split(token, ".")
	sig := []byte(parts[2])
	if sig[0] == 'A' {
		sig[0] = 'B'
	} else {
		sig[0] = 'A'
	}
	tampered := parts[0] + "." + parts[1] + "." + string(sig)

	if _, err := mgr.Parse(tampered); !errors.Is(err, jwt.ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken for tampered token, got %v", err)
	}
}

func TestManager_Parse_RejectsWrongSecret(t *testing.T) {
	t.Parallel()

	issuer := jwt.NewManager(_testSecret, _expiry)
	verifier := jwt.NewManager("another-secret-that-is-32-chars-x!", _expiry)

	token, err := issuer.Generate(_testUserID, time.Now())
	if err != nil {
		t.Fatalf("Generate: unexpected error: %v", err)
	}

	if _, err := verifier.Parse(token); !errors.Is(err, jwt.ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken for wrong-secret token, got %v", err)
	}
}

// TestManager_Parse_RejectsAlgNone is the security-critical case: a token signed
// with "alg: none" (unsigned) must be rejected, or anyone can forge identities.
func TestManager_Parse_RejectsAlgNone(t *testing.T) {
	t.Parallel()

	mgr := jwt.NewManager(_testSecret, _expiry)

	claims := jwtv5.RegisteredClaims{
		Subject:   _testUserID,
		ExpiresAt: jwtv5.NewNumericDate(time.Now().Add(_expiry)),
	}
	unsigned := jwtv5.NewWithClaims(jwtv5.SigningMethodNone, claims)

	// UnsafeAllowNoneSignatureType is the library's explicit opt-in to produce an
	// unsigned token — used here only to construct the attack input.
	tokenString, err := unsigned.SignedString(jwtv5.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("build alg:none token: %v", err)
	}

	if _, err := mgr.Parse(tokenString); !errors.Is(err, jwt.ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken for alg:none token, got %v", err)
	}
}

func TestManager_Parse_RejectsGarbage(t *testing.T) {
	t.Parallel()

	mgr := jwt.NewManager(_testSecret, _expiry)

	if _, err := mgr.Parse("not-a-jwt"); !errors.Is(err, jwt.ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken for garbage input, got %v", err)
	}
}
