// Package jwt issues and verifies stateless HS256 access tokens.
//
// Design (ADR-0006):
//   - Symmetric HS256: one issuer verifies its own tokens. RS256 is what you'd
//     reach for when services verify tokens they did not issue.
//   - No global state: a Manager is constructed from (secret, expiry) and
//     injected, matching the DI rule in docs/code-standards.md.
//   - Parse enforces the signing method is HMAC, closing the classic
//     alg-confusion / "alg:none" forgery hole.
package jwt

import (
	"errors"
	"fmt"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken is the single sentinel returned for every verification
// failure (bad signature, expired, malformed, wrong alg, ...). Callers branch
// on this instead of the library's internal error zoo, so the HTTP layer stays
// decoupled from golang-jwt specifics.
var ErrInvalidToken = errors.New("jwt: invalid token")

// Manager generates and verifies tokens with one shared secret.
type Manager struct {
	secret []byte
	expiry time.Duration
}

// NewManager builds a Manager. The secret length is validated at config load
// (min=32); a zero/negative expiry is allowed only so tests can mint an already
// expired token deterministically (no sleeping).
func NewManager(secret string, expiry time.Duration) *Manager {
	return &Manager{secret: []byte(secret), expiry: expiry}
}

// Generate issues an HS256 token whose subject is userID. now is passed in so
// callers (and tests) control the clock rather than reaching for time.Now
// inside the package.
func (m *Manager) Generate(userID string, now time.Time) (string, error) {
	claims := jwtv5.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwtv5.NewNumericDate(now),
		ExpiresAt: jwtv5.NewNumericDate(now.Add(m.expiry)),
	}

	token := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, claims)

	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("jwt.Manager.Generate: sign: %w", err)
	}

	return signed, nil
}

// Parse verifies the signature and expiry and returns the subject (user id).
// Any failure collapses to ErrInvalidToken.
func (m *Manager) Parse(tokenString string) (string, error) {
	// keyFunc rejects the token BEFORE the signature is checked if its alg is
	// not HMAC. A parser that trusts the token's own alg header lets an attacker
	// swap HS256 for "none" (unsigned) or RS256 (verify with the public key as
	// an HMAC secret) — the well-known JWT forgery.
	keyFunc := func(token *jwtv5.Token) (any, error) {
		if _, ok := token.Method.(*jwtv5.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("jwt.Manager.Parse: unexpected signing method: %v", token.Header["alg"])
		}

		return m.secret, nil
	}

	token, err := jwtv5.ParseWithClaims(
		tokenString, &jwtv5.RegisteredClaims{}, keyFunc,
		// Belt-and-suspenders: also constrain the accepted algs at the parser
		// level, so a future keyFunc change cannot silently widen them.
		jwtv5.WithValidMethods([]string{jwtv5.SigningMethodHS256.Alg()}),
	)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(*jwtv5.RegisteredClaims)
	if !ok || !token.Valid || claims.Subject == "" {
		return "", ErrInvalidToken
	}

	return claims.Subject, nil
}
