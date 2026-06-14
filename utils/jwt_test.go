package utils

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

// These are the project's only true unit tests: pure logic, no DB, no HTTP, no
// Fiber. They exist to demonstrate the layer is in the toolkit. The seam worth
// isolating here is JWT minting — GenerateJWT is self-contained and its output
// is consumed verbatim by the auth middleware, so a generate→parse round-trip
// pins down the token contract the rest of the app relies on.
//
// (Slug generation, the plan's other unit-test candidate, turned out to be a
// one-line call to the third-party gosimple/slug in handler/request.go — there
// is no first-party logic to isolate, so it isn't unit-tested here.)

// parse verifies the signature with the production secret and returns the claims.
func parse(t *testing.T, token string) jwt.MapClaims {
	t.Helper()
	parsed, err := jwt.Parse(token, func(tok *jwt.Token) (any, error) {
		// Reject any token not signed with the HMAC method GenerateJWT uses —
		// the canonical guard against alg-substitution (e.g. "none") attacks.
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return secretString, nil
	})
	require.NoError(t, err)
	require.True(t, parsed.Valid)
	return parsed.Claims.(jwt.MapClaims)
}

func TestGenerateJWT_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		id   uint
	}{
		{"typical id", 42},
		{"first user", 1},
		{"large id", 4294967295}, // max uint32; ids are stored/read as uint
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claims := parse(t, GenerateJWT(tc.id))

			// JWT claims serialise through JSON, so a numeric "id" always comes
			// back as float64 — this is exactly the cast userIDFromToken makes
			// in the handler, and asserting it here documents that contract.
			gotID, ok := claims["id"].(float64)
			require.True(t, ok, "id claim should decode as float64")
			require.Equal(t, tc.id, uint(gotID))
		})
	}
}

func TestGenerateJWT_SetsExpiryWindow(t *testing.T) {
	// GenerateJWT hard-codes a 72h expiry. Assert it lands in a window rather
	// than an exact instant so the test isn't clock-flaky.
	before := time.Now().Add(72 * time.Hour)
	claims := parse(t, GenerateJWT(7))
	after := time.Now().Add(72 * time.Hour)

	exp, ok := claims["exp"].(float64)
	require.True(t, ok, "exp claim should decode as float64")
	expAt := time.Unix(int64(exp), 0)

	require.False(t, expAt.Before(before.Add(-time.Second)), "expiry too early: %v", expAt)
	require.False(t, expAt.After(after.Add(time.Second)), "expiry too late: %v", expAt)
}

func TestGenerateJWT_RejectsWrongSecret(t *testing.T) {
	// A token minted by GenerateJWT must not verify under a different key —
	// guards against the signature check being a no-op.
	_, err := jwt.Parse(GenerateJWT(1), func(*jwt.Token) (any, error) {
		return []byte("not-the-secret"), nil
	})
	require.Error(t, err)
}
