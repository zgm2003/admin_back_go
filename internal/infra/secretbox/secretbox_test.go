package secretbox

import (
	"errors"
	"testing"
)

func TestBoxEncryptFailsWithoutKey(t *testing.T) {
	box := New(nil)
	_, err := box.Encrypt("plain")
	if err == nil || !errors.Is(err, ErrMissingKey) {
		t.Fatalf("expected missing key error, got %v", err)
	}
}

func TestBoxEncryptRejectsShortKey(t *testing.T) {
	box := New([]byte("short"))
	_, err := box.Encrypt("plain")
	if err == nil || !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("expected invalid key error, got %v", err)
	}
}

func TestBoxEncryptDecryptRoundTrip(t *testing.T) {
	box := New([]byte("12345678901234567890123456789012"))
	ciphertext, err := box.Encrypt("secret-value")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}
	if ciphertext == "" || ciphertext == "secret-value" {
		t.Fatalf("expected non-empty ciphertext different from plaintext, got %q", ciphertext)
	}
	plain, err := box.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt returned error: %v", err)
	}
	if plain != "secret-value" {
		t.Fatalf("expected secret-value, got %q", plain)
	}
}

func TestHint(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "short", in: "abc", want: "***abc"},
		{name: "four", in: "abcd", want: "***abcd"},
		{name: "long", in: "abcdef", want: "***cdef"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Hint(tt.in); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
