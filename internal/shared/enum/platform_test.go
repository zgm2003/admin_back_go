package enum

import (
	"reflect"
	"testing"
)

func TestRegisteredPlatformsExposeOnlyCurrentAdapters(t *testing.T) {
	if got := RegisteredPlatforms(); !reflect.DeepEqual(got, []string{PlatformAdmin}) {
		t.Fatalf("registered adapters mismatch: %#v", got)
	}
	if !IsRegisteredPlatform(PlatformAdmin) {
		t.Fatal("admin adapter must be registered")
	}
	for _, retired := range []string{PlatformApp, PlatformCanvas} {
		if IsRegisteredPlatform(retired) {
			t.Fatalf("retired adapter %q must not be registered", retired)
		}
	}
}

func TestRegisteredPlatformsReturnsDefensiveCopy(t *testing.T) {
	first := RegisteredPlatforms()
	first[0] = "mutated"
	if got := RegisteredPlatforms(); len(got) != 1 || got[0] != PlatformAdmin {
		t.Fatalf("registry was mutated through returned slice: %#v", got)
	}
}

func TestNotificationAudiencePlatformsKeepAllSeparateFromAdapters(t *testing.T) {
	want := []string{PlatformAll, PlatformAdmin}
	if got := NotificationAudiencePlatforms(); !reflect.DeepEqual(got, want) {
		t.Fatalf("notification audiences mismatch: %#v", got)
	}
	if !IsNotificationAudiencePlatform(PlatformAll) || !IsNotificationAudiencePlatform(PlatformAdmin) {
		t.Fatal("documented notification audiences must be accepted")
	}
	for _, invalid := range []string{PlatformApp, PlatformCanvas, "partner_portal"} {
		if IsNotificationAudiencePlatform(invalid) {
			t.Fatalf("unregistered notification audience %q must fail closed", invalid)
		}
	}
}
