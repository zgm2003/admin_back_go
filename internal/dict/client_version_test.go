package dict

import (
	"testing"

	"admin_back_go/internal/enum"
)

func TestClientVersionPlatformOptionsUseEnumOrder(t *testing.T) {
	options := ClientVersionPlatformOptions()
	if len(options) != 2 {
		t.Fatalf("expected two client version platform options, got %#v", options)
	}
	if options[0].Value != enum.ClientPlatformWindowsX8664 || options[0].Label != "Windows" {
		t.Fatalf("unexpected first platform option: %#v", options[0])
	}
	if options[1].Value != enum.ClientPlatformDarwinX8664 || options[1].Label != "macOS" {
		t.Fatalf("unexpected second platform option: %#v", options[1])
	}
}
