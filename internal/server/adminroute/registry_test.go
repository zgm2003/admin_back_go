package adminroute

import (
	"errors"
	"net/http"
	"testing"
)

func TestRegistryRejectsUnclassifiedMutation(t *testing.T) {
	registry := NewRegistry()
	err := registry.Add(Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/widgets",
		Access: Authenticated(),
	})
	if !errors.Is(err, ErrAuditDecisionRequired) {
		t.Fatalf("Add error=%v, want %v", err, ErrAuditDecisionRequired)
	}
}

func TestRegistryRejectsUnknownPermissionAndDuplicateRoute(t *testing.T) {
	registry := NewRegistry(WithPermissionCatalog(map[string]struct{}{"widget_create": {}}))
	mustAdd(t, registry, Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/widgets",
		Access: Permission("widget_read"),
		Audit:  NoAudit("read-only"),
	})
	if err := registry.Compile(); !errors.Is(err, ErrUnknownPermission) {
		t.Fatalf("Compile error=%v, want %v", err, ErrUnknownPermission)
	}

	err := registry.Add(Definition{
		Method: " get ",
		Path:   "/api/admin/v1/widgets/",
		Access: Authenticated(),
		Audit:  NoAudit("read-only"),
	})
	if !errors.Is(err, ErrDuplicateRoute) {
		t.Fatalf("duplicate Add error=%v, want %v", err, ErrDuplicateRoute)
	}
}

func TestRegistryRequiresPermissionCode(t *testing.T) {
	registry := NewRegistry()
	err := registry.Add(Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/widgets",
		Access: Permission("  "),
		Audit:  NoAudit("read-only"),
	})
	if !errors.Is(err, ErrPermissionCodeRequired) {
		t.Fatalf("Add error=%v, want %v", err, ErrPermissionCodeRequired)
	}
}

func TestRegistryRejectsAuditOnPublicProviderCallback(t *testing.T) {
	registry := NewRegistry()
	err := registry.Add(Definition{
		Method: http.MethodPost,
		Path:   "/api/payment/callbacks/alipay",
		Access: Public(),
		Audit:  Audit("payment", "callback", "Alipay callback"),
	})
	if !errors.Is(err, ErrPublicCallbackAudit) {
		t.Fatalf("Add error=%v, want %v", err, ErrPublicCallbackAudit)
	}
}

func TestRegistryReturnsNormalizedDeterministicDefinitions(t *testing.T) {
	registry := NewRegistry()
	mustAdd(t, registry, Definition{Method: " post ", Path: " /api/admin/v1/widgets/ ", OperationID: "createWidget", Access: Authenticated(), Audit: Audit("widget", "create", "Create widget")})
	mustAdd(t, registry, Definition{Method: http.MethodGet, Path: "/api/admin/v1/widgets", OperationID: "listWidgets", Access: Authenticated(), Audit: NoAudit("read-only")})
	if err := registry.Compile(); err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}

	definitions := registry.Definitions()
	if len(definitions) != 2 {
		t.Fatalf("definitions=%+v", definitions)
	}
	if definitions[0].Method != http.MethodGet || definitions[0].Path != "/api/admin/v1/widgets" {
		t.Fatalf("first definition not normalized/sorted: %+v", definitions[0])
	}
	if definitions[1].Method != http.MethodPost || definitions[1].Path != "/api/admin/v1/widgets" {
		t.Fatalf("second definition not normalized/sorted: %+v", definitions[1])
	}
}

func mustAdd(t *testing.T, registry *Registry, definition Definition) {
	t.Helper()
	if err := registry.Add(definition); err != nil {
		t.Fatalf("Add(%s %s): %v", definition.Method, definition.Path, err)
	}
}
