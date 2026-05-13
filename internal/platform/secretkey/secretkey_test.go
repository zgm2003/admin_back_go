package secretkey

import (
	"bytes"
	"strings"
	"testing"
)

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
