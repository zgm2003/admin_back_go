package config

import "testing"

func TestLoadReadsSecretboxKey(t *testing.T) {
	t.Setenv("VAULT_KEY", "vault-secret")

	cfg := Load()

	if cfg.Secretbox.Key != "vault-secret" {
		t.Fatalf("expected vault-secret, got %q", cfg.Secretbox.Key)
	}
}
