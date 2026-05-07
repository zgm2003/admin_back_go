package enum

import "testing"

func TestClientVersionPlatformMembership(t *testing.T) {
	if !IsClientPlatform(ClientPlatformWindowsX8664) {
		t.Fatalf("expected windows client platform to be valid")
	}
	if !IsClientPlatform(ClientPlatformDarwinX8664) {
		t.Fatalf("expected darwin client platform to be valid")
	}
	if IsClientPlatform("linux-x86_64") {
		t.Fatalf("linux-x86_64 is not supported in v1")
	}
}

func TestClientVersionPlatformName(t *testing.T) {
	if got := ClientPlatformName(ClientPlatformWindowsX8664); got != "Windows" {
		t.Fatalf("expected Windows label, got %q", got)
	}
	if got := ClientPlatformName(ClientPlatformDarwinX8664); got != "macOS" {
		t.Fatalf("expected macOS label, got %q", got)
	}
	if got := ClientPlatformName("linux-x86_64"); got != "" {
		t.Fatalf("expected empty unknown label, got %q", got)
	}
}
