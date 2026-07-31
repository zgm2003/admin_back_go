package capability

import (
	"reflect"
	"testing"

	aiprovider "admin_back_go/internal/module/ai/provider"
)

func TestResolveNativeFileCapabilityNeverWidensOfficialTruth(t *testing.T) {
	tests := []struct {
		name, mode, wantReason               string
		official, transport, route, platform bool
		wantEnabled                          bool
	}{
		{"enabled", aiprovider.FileInputModeChatCompletions, "", true, true, true, true, true},
		{"official", aiprovider.FileInputModeChatCompletions, NativeFileDisabledOfficialModel, false, true, true, true, false},
		{"transport", aiprovider.FileInputModeChatCompletions, NativeFileDisabledTransport, true, false, true, true, false},
		{"provider mode", aiprovider.FileInputModeDisabled, NativeFileDisabledProviderMode, true, true, true, true, false},
		{"provider route", aiprovider.FileInputModeChatCompletions, NativeFileDisabledProviderMode, true, true, false, true, false},
		{"platform", aiprovider.FileInputModeChatCompletions, NativeFileDisabledPlatform, true, true, true, false, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ResolveNativeFileCapability(NativeFileCapabilityInput{
				OfficialEnabled: test.official, TransportEnabled: test.transport,
				ProviderMode: test.mode, ProviderRouteEnabled: test.route,
				PlatformReady: test.platform, AcceptedExtensions: []string{"pdf", "md"},
			})
			if got.Enabled != test.wantEnabled || got.DisabledReason != test.wantReason {
				t.Fatalf("capability=%#v", got)
			}
			if got.Enabled && !reflect.DeepEqual(got.AcceptedExtensions, []string{"pdf", "md"}) {
				t.Fatalf("accepted extensions=%#v", got.AcceptedExtensions)
			}
		})
	}
}

func TestNativeFilePolicyIntersectsSystemUploadRule(t *testing.T) {
	got := AllowedNativeFileExtensions([]string{"go", "zip", "md", "psd", "pdf"})
	if !reflect.DeepEqual(got, []string{"pdf", "md", "go"}) {
		t.Fatalf("accepted extensions=%#v", got)
	}
}

func TestResolveNativeFileCapabilityCopiesAcceptedExtensions(t *testing.T) {
	input := []string{"pdf", "md"}
	got := ResolveNativeFileCapability(NativeFileCapabilityInput{
		OfficialEnabled: true, TransportEnabled: true,
		ProviderMode: aiprovider.FileInputModeChatCompletions, ProviderRouteEnabled: true,
		PlatformReady: true, AcceptedExtensions: input,
	})
	input[0] = "mutated"
	if !reflect.DeepEqual(got.AcceptedExtensions, []string{"pdf", "md"}) {
		t.Fatalf("capability aliases caller input: %#v", got)
	}
}
