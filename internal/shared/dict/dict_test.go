package dict

import (
	"reflect"
	"testing"
)

func TestRegistryContainsInitialProviders(t *testing.T) {
	registry := NewRegistry()

	tests := []struct {
		name string
		want any
	}{
		{name: ProviderCommonStatus, want: CommonStatusOptions()},
		{name: ProviderCommonYesNo, want: CommonYesNoOptions()},
		{name: ProviderPlatform, want: PlatformOptions()},
		{name: ProviderSystemSettingValueType, want: SystemSettingValueTypeOptions()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := registry.Options(tt.name)
			if !ok {
				t.Fatalf("expected provider %q to be registered", tt.name)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("provider %q payload mismatch:\n got: %#v\nwant: %#v", tt.name, got, tt.want)
			}
		})
	}
}

func TestServiceUsesDefaultRegistryAndPreservesCompatibilityHelpers(t *testing.T) {
	service := NewService(nil)

	got, ok := service.Options(ProviderSystemSettingValueType)
	if !ok {
		t.Fatalf("expected default service registry to include %q", ProviderSystemSettingValueType)
	}
	if !reflect.DeepEqual(got, SystemSettingValueTypeOptions()) {
		t.Fatalf("service payload must match helper payload, got %#v", got)
	}
}
