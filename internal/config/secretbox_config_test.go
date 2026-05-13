package config

import (
	"strings"
	"testing"
)

func TestLoadReadsAppSecretWithoutLegacyVaultConfig(t *testing.T) {
	t.Setenv("APP_SECRET", strings.Repeat("a", 64))

	cfg := Load()

	if cfg.App.Secret != strings.Repeat("a", 64) {
		t.Fatalf("expected APP_SECRET to be loaded, got %q", cfg.App.Secret)
	}
}
