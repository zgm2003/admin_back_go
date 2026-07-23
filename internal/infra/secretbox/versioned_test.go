package secretbox

import (
	"errors"
	"testing"
)

func TestNewVersionedValidatesCurrentKeyAndKeySet(t *testing.T) {
	validKey := []byte("12345678901234567890123456789012")
	tests := []struct {
		name      string
		currentID string
		keys      map[string][]byte
		wantErr   error
	}{
		{name: "missing current ID", keys: map[string][]byte{"current": validKey}, wantErr: ErrMissingCurrentKeyID},
		{name: "whitespace current ID", currentID: "  ", keys: map[string][]byte{"current": validKey}, wantErr: ErrMissingCurrentKeyID},
		{name: "current entry absent", currentID: "current", keys: map[string][]byte{"previous": validKey}, wantErr: ErrUnknownKeyID},
		{name: "nil keys", currentID: "current", wantErr: ErrUnknownKeyID},
		{name: "short key", currentID: "current", keys: map[string][]byte{"current": []byte("short")}, wantErr: ErrInvalidKey},
		{name: "empty key ID", currentID: "current", keys: map[string][]byte{"current": validKey, " ": validKey}, wantErr: ErrUnknownKeyID},
		{name: "duplicate trimmed ID", currentID: "current", keys: map[string][]byte{"current": validKey, " current ": validKey}, wantErr: ErrUnknownKeyID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewVersioned(tt.currentID, tt.keys); !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewVersioned() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestVersionedBoxEncryptsOnlyWithCurrentKey(t *testing.T) {
	currentKey := []byte("cccccccccccccccccccccccccccccccc")
	previousKey := []byte("pppppppppppppppppppppppppppppppp")
	box, err := NewVersioned(" current ", map[string][]byte{
		" current ":  currentKey,
		" previous ": previousKey,
	})
	if err != nil {
		t.Fatalf("NewVersioned returned error: %v", err)
	}
	if got := box.CurrentKeyID(); got != "current" {
		t.Fatalf("CurrentKeyID() = %q, want current", got)
	}

	keyID, firstCiphertext, err := box.Encrypt("verification-code")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}
	if keyID != "current" {
		t.Fatalf("Encrypt key ID = %q, want current", keyID)
	}
	plain, err := New(currentKey).Decrypt(firstCiphertext)
	if err != nil || plain != "verification-code" {
		t.Fatalf("current key failed to decrypt ciphertext: plain=%q err=%v", plain, err)
	}
	if _, err := New(previousKey).Decrypt(firstCiphertext); err == nil {
		t.Fatal("previous key unexpectedly decrypted current ciphertext")
	}

	_, secondCiphertext, err := box.Encrypt("verification-code")
	if err != nil {
		t.Fatalf("second Encrypt returned error: %v", err)
	}
	if secondCiphertext == firstCiphertext {
		t.Fatal("Encrypt reused a nonce")
	}
}

func TestVersionedBoxDecryptsByExactKeyID(t *testing.T) {
	currentKey := []byte("cccccccccccccccccccccccccccccccc")
	previousKey := []byte("pppppppppppppppppppppppppppppppp")
	box, err := NewVersioned("current", map[string][]byte{
		"current":  currentKey,
		"previous": previousKey,
	})
	if err != nil {
		t.Fatalf("NewVersioned returned error: %v", err)
	}
	previousCiphertext, err := New(previousKey).Encrypt("old-code")
	if err != nil {
		t.Fatalf("encrypt with previous key: %v", err)
	}

	plain, err := box.Decrypt("previous", previousCiphertext)
	if err != nil || plain != "old-code" {
		t.Fatalf("Decrypt previous ciphertext: plain=%q err=%v", plain, err)
	}
	for _, keyID := range []string{"", "unknown", " previous "} {
		if _, err := box.Decrypt(keyID, previousCiphertext); !errors.Is(err, ErrUnknownKeyID) {
			t.Fatalf("Decrypt(%q) error = %v, want ErrUnknownKeyID", keyID, err)
		}
	}
	if _, err := box.Decrypt("current", previousCiphertext); err == nil || errors.Is(err, ErrUnknownKeyID) {
		t.Fatalf("Decrypt with known wrong key error = %v, want authentication failure", err)
	}
}

func TestVersionedBoxClonesInputKeys(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	box, err := NewVersioned("current", map[string][]byte{"current": key})
	if err != nil {
		t.Fatalf("NewVersioned returned error: %v", err)
	}
	key[0] ^= 0xff

	_, ciphertext, err := box.Encrypt("plain")
	if err != nil {
		t.Fatalf("Encrypt after caller mutated key: %v", err)
	}
	plain, err := New([]byte("12345678901234567890123456789012")).Decrypt(ciphertext)
	if err != nil || plain != "plain" {
		t.Fatalf("cloned key did not decrypt ciphertext: plain=%q err=%v", plain, err)
	}
}

func TestVersionedBoxEmptyPlaintextStillIdentifiesCurrentKey(t *testing.T) {
	box, err := NewVersioned("current", map[string][]byte{
		"current": []byte("12345678901234567890123456789012"),
	})
	if err != nil {
		t.Fatalf("NewVersioned returned error: %v", err)
	}

	keyID, ciphertext, err := box.Encrypt("")
	if err != nil || keyID != "current" || ciphertext != "" {
		t.Fatalf("Encrypt empty = (%q, %q, %v), want (current, empty, nil)", keyID, ciphertext, err)
	}
	plain, err := box.Decrypt("current", "")
	if err != nil || plain != "" {
		t.Fatalf("Decrypt empty = (%q, %v), want (empty, nil)", plain, err)
	}
}

func TestVersionedBoxRejectsDamagedCiphertext(t *testing.T) {
	box, err := NewVersioned("current", map[string][]byte{
		"current": []byte("12345678901234567890123456789012"),
	})
	if err != nil {
		t.Fatalf("NewVersioned returned error: %v", err)
	}
	_, ciphertext, err := box.Encrypt("do-not-leak-this")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}
	damaged := ciphertext[:len(ciphertext)-1] + "A"
	if damaged == ciphertext {
		damaged = ciphertext[:len(ciphertext)-1] + "B"
	}
	plain, err := box.Decrypt("current", damaged)
	if err == nil || plain != "" {
		t.Fatalf("Decrypt damaged ciphertext = (%q, %v), want empty plaintext and error", plain, err)
	}
}
