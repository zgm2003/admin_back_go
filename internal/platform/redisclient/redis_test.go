package redisclient

import (
	"testing"

	"admin_back_go/internal/config"
)

func TestOpenMapsConfigToOptions(t *testing.T) {
	client := Open(config.RedisConfig{
		Addr:     "127.0.0.1:6380",
		Password: "secret",
		DB:       2,
	})
	defer client.Close()

	if client.Redis == nil {
		t.Fatalf("expected redis handle")
	}

	options := client.Redis.Options()
	if options.Addr != "127.0.0.1:6380" {
		t.Fatalf("expected addr 127.0.0.1:6380, got %q", options.Addr)
	}
	if options.Password != "secret" {
		t.Fatalf("expected password secret, got %q", options.Password)
	}
	if options.DB != 2 {
		t.Fatalf("expected db 2, got %d", options.DB)
	}
}

func TestCloseIsSafeOnNilClient(t *testing.T) {
	var client *Client
	if err := client.Close(); err != nil {
		t.Fatalf("expected nil close to be safe, got %v", err)
	}
}
