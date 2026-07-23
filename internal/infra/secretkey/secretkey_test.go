package secretkey

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

var authCacheKeyMethod = regexp.MustCompile(`(?m)^func\s+\(s\s+\*SessionRevocationService\)\s+sessionCacheKey\s*\(\s*sessionID\s+int64\s*\)\s*string\s*\{`)

func TestKeyRingDoesNotDeriveSessionCacheCapability(t *testing.T) {
	secretkeySource, err := os.ReadFile("secretkey.go")
	if err != nil {
		t.Fatal("forbidden token")
	}
	for _, forbidden := range []string{
		"sessionCache" + "Key",
		"SessionCache" + "Key",
		"admin_go:session-" + "cache:v1",
	} {
		if strings.Contains(string(secretkeySource), forbidden) {
			t.Fatal("forbidden token")
		}
	}

	authSessionCacheSource, err := os.ReadFile("../../module/auth/session_cache.go")
	if err != nil || !authCacheKeyMethod.Match(authSessionCacheSource) {
		t.Fatal("forbidden token")
	}
}

func TestNewKeyRingDerivesStableSeparatedKeys(t *testing.T) {
	root := strings.Repeat("a", 64)

	first, err := NewKeyRing(root)
	if err != nil {
		t.Fatalf("NewKeyRing returned error: %v", err)
	}
	second, err := NewKeyRing(root)
	if err != nil {
		t.Fatalf("NewKeyRing second call returned error: %v", err)
	}

	if len(first.SecretboxKey()) != 32 || len(first.JWTSigningKey()) != 32 {
		t.Fatalf("expected 32-byte derived keys")
	}
	if !bytes.Equal(first.SecretboxKey(), second.SecretboxKey()) {
		t.Fatalf("expected stable secretbox derivation")
	}
	if bytes.Equal(first.SecretboxKey(), first.JWTSigningKey()) {
		t.Fatalf("expected secretbox and JWT keys to differ")
	}
	if first.TokenPepper() == "" {
		t.Fatalf("expected non-empty token pepper")
	}
}

func TestNewKeyRingRejectsUnsafeSecrets(t *testing.T) {
	for _, secret := range []string{"", "short", "change_me_to_at_least_64_random_chars"} {
		if _, err := NewKeyRing(secret); err == nil {
			t.Fatalf("expected %q to be rejected", secret)
		}
	}
}

func TestNewKeyRingWithPreviousCarriesDistinctJWTKeyIDs(t *testing.T) {
	current := strings.Repeat("c", 64)
	previous := strings.Repeat("p", 64)

	ring, err := NewKeyRingWithPrevious(current, []string{previous})
	if err != nil {
		t.Fatalf("NewKeyRingWithPrevious returned error: %v", err)
	}
	if ring.JWTSigningKeyID() == "" {
		t.Fatal("current JWT key ID is empty")
	}
	keys := ring.JWTVerificationKeys()
	if len(keys) != 2 {
		t.Fatalf("JWT verification key count = %d, want 2", len(keys))
	}
	if _, ok := keys[ring.JWTSigningKeyID()]; !ok {
		t.Fatalf("current JWT key ID %q is absent from verification keys", ring.JWTSigningKeyID())
	}

	oldRing, err := NewKeyRing(previous)
	if err != nil {
		t.Fatalf("NewKeyRing(previous) returned error: %v", err)
	}
	if _, ok := keys[oldRing.JWTSigningKeyID()]; !ok {
		t.Fatalf("previous JWT key ID %q is absent from verification keys", oldRing.JWTSigningKeyID())
	}
	if oldRing.JWTSigningKeyID() == ring.JWTSigningKeyID() {
		t.Fatal("current and previous JWT key IDs are equal")
	}
}

func TestNewKeyRingWithPreviousRejectsInvalidRotation(t *testing.T) {
	current := strings.Repeat("c", 64)
	tests := []struct {
		name     string
		previous []string
	}{
		{name: "same key", previous: []string{current}},
		{name: "more than one previous", previous: []string{strings.Repeat("a", 64), strings.Repeat("b", 64)}},
		{name: "unsafe previous", previous: []string{"short"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewKeyRingWithPrevious(current, test.previous); err == nil {
				t.Fatal("expected rotation configuration to be rejected")
			}
		})
	}
}
