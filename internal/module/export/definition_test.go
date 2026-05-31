package exporttask

import (
	"context"
	"strings"
	"testing"
)

type registryProvider struct{}

func (registryProvider) BuildExportData(ctx context.Context, input BuildInput) (*FileData, error) {
	return &FileData{Prefix: input.Kind, Headers: []Column{{Key: "id", Title: "ID"}}, Rows: []map[string]string{{"id": "1"}}}, nil
}

func TestRegistryResolvesKnownKindAndRejectsUnknown(t *testing.T) {
	registry, err := NewRegistry(Definition{Kind: KindUserList, Title: "用户列表", Provider: registryProvider{}})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}
	def, ok := registry.Resolve(" user_list ")
	if !ok || def.Kind != KindUserList || def.Title != "用户列表" {
		t.Fatalf("unexpected definition: ok=%v def=%#v", ok, def)
	}
	if _, ok := registry.Resolve("payment_orders"); ok {
		t.Fatalf("expected unknown kind to be rejected")
	}
	if _, ok := registry.Resolve(""); ok {
		t.Fatalf("expected empty kind to be rejected")
	}
	if _, ok := registry.Resolve("   "); ok {
		t.Fatalf("expected blank kind to be rejected")
	}
}

func TestRegistryRejectsDuplicateOrEmptyDefinition(t *testing.T) {
	_, err := NewRegistry(Definition{})
	if err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("expected empty kind error, got %v", err)
	}
	_, err = NewRegistry(
		Definition{Kind: KindUserList, Title: "用户列表", Provider: registryProvider{}},
		Definition{Kind: " user_list ", Title: "用户列表2", Provider: registryProvider{}},
	)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate kind error, got %v", err)
	}
}

func TestRegistryRejectsEmptyProvider(t *testing.T) {
	_, err := NewRegistry(Definition{Kind: KindUserList, Title: "用户列表"})
	if err == nil || !strings.Contains(err.Error(), "provider") {
		t.Fatalf("expected empty provider error, got %v", err)
	}
}
