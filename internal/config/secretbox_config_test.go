package config

import (
	"strings"
	"testing"
)

func TestLoadDoesNotReadLegacyVaultKey(t *testing.T) {
	t.Setenv("VAULT_KEY", "vault-secret")
	t.Setenv("APP_SECRET", strings.Repeat("a", 64))

	cfg := Load()

	if cfg.App.Secret != strings.Repeat("a", 64) {
		t.Fatalf("expected APP_SECRET to be loaded, got %q", cfg.App.Secret)
	}
}
