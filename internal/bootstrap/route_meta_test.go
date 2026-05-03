package bootstrap

import (
	"net/http"
	"testing"

	"admin_back_go/internal/middleware"
)

func TestPermissionRouteRulesUseExplicitRESTPatterns(t *testing.T) {
	rules := permissionRouteRules()

	tests := []struct {
		method string
		path   string
		code   string
	}{
		{http.MethodPost, "/api/v1/permissions", "permission_permission_add"},
		{http.MethodPut, "/api/v1/permissions/:id", "permission_permission_edit"},
		{http.MethodPatch, "/api/v1/permissions/:id/status", "permission_permission_status"},
		{http.MethodDelete, "/api/v1/permissions/:id", "permission_permission_del"},
		{http.MethodDelete, "/api/v1/permissions", "permission_permission_del"},
		{http.MethodPost, "/api/v1/roles", "permission_role_add"},
		{http.MethodPut, "/api/v1/roles/:id", "permission_role_edit"},
		{http.MethodPatch, "/api/v1/roles/:id/default", "permission_role_setDefault"},
		{http.MethodDelete, "/api/v1/roles/:id", "permission_role_del"},
		{http.MethodDelete, "/api/v1/roles", "permission_role_del"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			got := rules[middleware.NewRouteKey(tt.method, tt.path)]
			if got != tt.code {
				t.Fatalf("expected %q, got %q", tt.code, got)
			}
		})
	}

	if _, ok := rules[middleware.NewRouteKey(http.MethodGet, "/api/v1/permissions")]; ok {
		t.Fatalf("permission list must not gain an implicit button-rule fallback")
	}
	if _, ok := rules[middleware.NewRouteKey(http.MethodGet, "/api/v1/roles")]; ok {
		t.Fatalf("role list must not gain an implicit button-rule fallback")
	}
	if _, ok := rules[middleware.NewRouteKey(http.MethodPost, "/api/admin/Permission/add")]; ok {
		t.Fatalf("new route rules must not carry legacy all-post endpoints")
	}
}

func TestOperationRouteRulesUseExplicitRESTPatterns(t *testing.T) {
	rules := operationRouteRules()

	tests := []struct {
		method string
		path   string
		action string
	}{
		{http.MethodPost, "/api/v1/permissions", "create"},
		{http.MethodPut, "/api/v1/permissions/:id", "update"},
		{http.MethodPatch, "/api/v1/permissions/:id/status", "change_status"},
		{http.MethodDelete, "/api/v1/permissions/:id", "delete"},
		{http.MethodDelete, "/api/v1/permissions", "delete_batch"},
		{http.MethodPost, "/api/v1/roles", "create"},
		{http.MethodPut, "/api/v1/roles/:id", "update"},
		{http.MethodPatch, "/api/v1/roles/:id/default", "set_default"},
		{http.MethodDelete, "/api/v1/roles/:id", "delete"},
		{http.MethodDelete, "/api/v1/roles", "delete_batch"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			got := rules[middleware.NewRouteKey(tt.method, tt.path)]
			if got.Action != tt.action || got.Title == "" {
				t.Fatalf("unexpected operation rule: %#v", got)
			}
		})
	}
}
