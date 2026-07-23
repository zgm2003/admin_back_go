package secretkey

import (
	"bytes"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
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

func TestKeyRingDerivesStableMailDiagnosticKeyAndID(t *testing.T) {
	root := strings.Repeat("d", 64)

	first, err := NewKeyRing(root)
	if err != nil {
		t.Fatalf("NewKeyRing returned error: %v", err)
	}
	second, err := NewKeyRing(root)
	if err != nil {
		t.Fatalf("NewKeyRing second call returned error: %v", err)
	}

	wantKey, err := hkdf.Key(sha256.New, []byte(root), nil, "admin_go:mail-verification-diagnostic:v1", 32)
	if err != nil {
		t.Fatalf("derive expected diagnostic key: %v", err)
	}
	wantDigest := sha256.Sum256(wantKey)
	wantID := "mail-diagnostic-v1-" + base64.RawURLEncoding.EncodeToString(wantDigest[:16])

	if !bytes.Equal(first.MailDiagnosticKey(), wantKey) {
		t.Fatal("mail diagnostic key was not derived with the required purpose")
	}
	if !bytes.Equal(first.MailDiagnosticKey(), second.MailDiagnosticKey()) {
		t.Fatal("mail diagnostic key derivation is not deterministic")
	}
	if got := first.MailDiagnosticKeyID(); got != wantID {
		t.Fatalf("MailDiagnosticKeyID() = %q, want %q", got, wantID)
	}
	if bytes.Equal(first.MailDiagnosticKey(), first.SecretboxKey()) {
		t.Fatal("mail diagnostic key must be separated from the general secretbox key")
	}
}

func TestKeyRingCarriesCurrentAndPreviousMailDiagnosticKeys(t *testing.T) {
	currentRoot := strings.Repeat("c", 64)
	previousRoot := strings.Repeat("p", 64)

	ring, err := NewKeyRingWithPrevious(currentRoot, []string{previousRoot})
	if err != nil {
		t.Fatalf("NewKeyRingWithPrevious returned error: %v", err)
	}
	previousRing, err := NewKeyRing(previousRoot)
	if err != nil {
		t.Fatalf("NewKeyRing(previous) returned error: %v", err)
	}

	keys := ring.MailDiagnosticDecryptionKeys()
	if len(keys) != 2 {
		t.Fatalf("MailDiagnosticDecryptionKeys() length = %d, want 2", len(keys))
	}
	if !bytes.Equal(keys[ring.MailDiagnosticKeyID()], ring.MailDiagnosticKey()) {
		t.Fatal("current mail diagnostic key is absent from decryption keys")
	}
	if !bytes.Equal(keys[previousRing.MailDiagnosticKeyID()], previousRing.MailDiagnosticKey()) {
		t.Fatal("previous mail diagnostic key is absent from decryption keys")
	}
}

func TestMailDiagnosticAccessorsReturnClones(t *testing.T) {
	ring, err := NewKeyRingWithPrevious(strings.Repeat("c", 64), []string{strings.Repeat("p", 64)})
	if err != nil {
		t.Fatalf("NewKeyRingWithPrevious returned error: %v", err)
	}

	key := ring.MailDiagnosticKey()
	key[0] ^= 0xff
	if bytes.Equal(key, ring.MailDiagnosticKey()) {
		t.Fatal("MailDiagnosticKey exposed internal key bytes")
	}

	keys := ring.MailDiagnosticDecryptionKeys()
	currentID := ring.MailDiagnosticKeyID()
	keys[currentID][0] ^= 0xff
	delete(keys, currentID)
	fresh := ring.MailDiagnosticDecryptionKeys()
	if len(fresh) != 2 || !bytes.Equal(fresh[currentID], ring.MailDiagnosticKey()) {
		t.Fatal("MailDiagnosticDecryptionKeys exposed internal map or key bytes")
	}
}

func TestNilKeyRingMailDiagnosticAccessors(t *testing.T) {
	var ring *KeyRing
	if ring.MailDiagnosticKey() != nil {
		t.Fatal("nil KeyRing returned a diagnostic key")
	}
	if ring.MailDiagnosticKeyID() != "" {
		t.Fatal("nil KeyRing returned a diagnostic key ID")
	}
	if ring.MailDiagnosticDecryptionKeys() != nil {
		t.Fatal("nil KeyRing returned diagnostic decryption keys")
	}
}
