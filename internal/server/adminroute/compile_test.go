package adminroute

import (
	"errors"
	"net/http"
	"testing"

	"admin_back_go/internal/middleware"
)

func TestCompileRoutesRejectsUnclassifiedActualMutation(t *testing.T) {
	registry := NewRegistry()
	err := registry.CompileRoutes([]Route{{Method: http.MethodPost, Path: "/api/admin/v1/widgets"}})
	if !errors.Is(err, ErrAuditDecisionRequired) {
		t.Fatalf("CompileRoutes error=%v, want %v", err, ErrAuditDecisionRequired)
	}
}

func TestCompileRoutesRejectsDefinitionWithoutActualRoute(t *testing.T) {
	registry := NewRegistry()
	mustAdd(t, registry, Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/removed",
		Access: Authenticated(),
		Audit:  NoAudit("read-only"),
	})
	err := registry.CompileRoutes([]Route{{Method: http.MethodGet, Path: "/api/admin/v1/widgets"}})
	if !errors.Is(err, ErrUnknownRouteDefinition) {
		t.Fatalf("CompileRoutes error=%v, want %v", err, ErrUnknownRouteDefinition)
	}
}

func TestCompileRoutesBuildsRuntimeMiddlewareMaps(t *testing.T) {
	key := middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/widgets")
	registry, err := NewLegacyRegistry(
		map[middleware.RouteKey]string{key: "widget_create"},
		map[middleware.RouteKey]middleware.OperationRule{key: {
			Module: "widget", Action: "create", Title: "Create widget", SkipRequestPayload: true,
		}},
		map[string]struct{}{"/health": {}},
		nil,
	)
	if err != nil {
		t.Fatalf("NewLegacyRegistry: %v", err)
	}
	err = registry.CompileRoutes([]Route{
		{Method: http.MethodGet, Path: "/health"},
		{Method: http.MethodGet, Path: "/api/admin/v1/widgets"},
		{Method: http.MethodPost, Path: "/api/admin/v1/widgets"},
	})
	if err != nil {
		t.Fatalf("CompileRoutes: %v", err)
	}

	if _, exists := registry.PublicPaths()["/health"]; !exists {
		t.Fatalf("public paths=%v", registry.PublicPaths())
	}
	if got := registry.PermissionRules()[key]; got != "widget_create" {
		t.Fatalf("permission rule=%q", got)
	}
	operation := registry.OperationRules()[key]
	if operation.Module != "widget" || operation.Action != "create" || !operation.SkipRequestPayload {
		t.Fatalf("operation rule=%+v", operation)
	}

	definitions := registry.Definitions()
	if len(definitions) != 3 {
		t.Fatalf("definitions=%+v", definitions)
	}
	for _, definition := range definitions {
		if definition.OperationID == "" {
			t.Fatalf("missing operation ID: %+v", definition)
		}
		if !definition.Audit.Enabled && definition.Audit.Reason == "" {
			t.Fatalf("missing explicit no-audit reason: %+v", definition)
		}
	}
}

func TestCompileRoutesUsesExplicitNoAuditReasonForMutation(t *testing.T) {
	key := middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/auth/logout")
	registry, err := NewLegacyRegistry(nil, nil, nil, map[middleware.RouteKey]string{
		key: "self-service session state",
	})
	if err != nil {
		t.Fatalf("NewLegacyRegistry: %v", err)
	}
	if err := registry.CompileRoutes([]Route{{Method: key.Method, Path: key.Path}}); err != nil {
		t.Fatalf("CompileRoutes: %v", err)
	}
	definition := registry.Definitions()[0]
	if definition.Audit.Enabled || definition.Audit.Reason != "self-service session state" {
		t.Fatalf("definition=%+v", definition)
	}
}

func TestNewLegacyRegistryNormalizesLegacyKeys(t *testing.T) {
	legacyKey := middleware.RouteKey{Method: " post ", Path: " /api/admin/v1/widgets/ "}
	registry, err := NewLegacyRegistry(
		map[middleware.RouteKey]string{legacyKey: "widget_create"},
		map[middleware.RouteKey]middleware.OperationRule{legacyKey: {
			Module: "widget", Action: "create", Title: "Create widget",
		}},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewLegacyRegistry: %v", err)
	}
	if err := registry.CompileRoutes([]Route{{Method: http.MethodPost, Path: "/api/admin/v1/widgets"}}); err != nil {
		t.Fatalf("CompileRoutes: %v", err)
	}
	key := middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/widgets")
	if registry.PermissionRules()[key] != "widget_create" || registry.OperationRules()[key].Action != "create" {
		t.Fatalf("legacy metadata was not normalized")
	}
}

func TestMailLogReadsRequirePermissionAndAuditLegacyAdapterPreservesRequired(t *testing.T) {
	key := middleware.NewRouteKey(http.MethodGet, "/api/admin/v1/mail/logs")
	registry, err := NewLegacyRegistry(
		map[middleware.RouteKey]string{key: "system_mail_logView"},
		map[middleware.RouteKey]middleware.OperationRule{key: {
			Module: "mail", Action: "list_logs", Title: "查看邮件日志及验证码",
			SkipRequestPayload: true, SkipResponsePayload: true, Required: true,
		}},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewLegacyRegistry: %v", err)
	}
	if err := registry.CompileRoutes([]Route{{Method: key.Method, Path: key.Path}}); err != nil {
		t.Fatalf("CompileRoutes: %v", err)
	}

	definition := registry.Definitions()[0]
	if !definition.Audit.Required {
		t.Fatalf("legacy definition dropped required audit: %+v", definition.Audit)
	}
	operation := registry.OperationRules()[key]
	if !operation.Required || !operation.SkipRequestPayload || !operation.SkipResponsePayload {
		t.Fatalf("legacy runtime rule dropped required payload-free audit: %+v", operation)
	}
}
