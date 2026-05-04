package secretbox

import "testing"

func TestBoxEncryptFailsWithoutKey(t *testing.T) {
	box := New("")

	_, err := box.Encrypt("plain")

	if err == nil {
		t.Fatalf("expected missing key error")
	}
}

func TestBoxEncryptDecryptRoundTrip(t *testing.T) {
	box := New("round-trip-key")

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
		t.Fatalf("expected decrypted secret-value, got %q", plain)
	}
}

func TestBoxDecryptLegacyFormat(t *testing.T) {
	box := New("legacy-vault-key")
	const legacyCiphertext = "MTIzNDU2Nzg5MDEyfr/0r0C2Iyw+lQA9xDPrfmR6xMfzk1IhUNbtdwo="

	plain, err := box.Decrypt(legacyCiphertext)
	if err != nil {
		t.Fatalf("Decrypt legacy ciphertext returned error: %v", err)
	}
	if plain != "legacy-secret" {
		t.Fatalf("expected legacy-secret, got %q", plain)
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
