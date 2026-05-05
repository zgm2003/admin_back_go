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
		{http.MethodPost, "/api/admin/v1/permissions", "permission_permission_add"},
		{http.MethodPut, "/api/admin/v1/permissions/:id", "permission_permission_edit"},
		{http.MethodPatch, "/api/admin/v1/permissions/:id/status", "permission_permission_status"},
		{http.MethodDelete, "/api/admin/v1/permissions/:id", "permission_permission_del"},
		{http.MethodDelete, "/api/admin/v1/permissions", "permission_permission_del"},
		{http.MethodPost, "/api/admin/v1/roles", "permission_role_add"},
		{http.MethodPut, "/api/admin/v1/roles/:id", "permission_role_edit"},
		{http.MethodPatch, "/api/admin/v1/roles/:id/default", "permission_role_setDefault"},
		{http.MethodDelete, "/api/admin/v1/roles/:id", "permission_role_del"},
		{http.MethodDelete, "/api/admin/v1/roles", "permission_role_del"},
		{http.MethodPost, "/api/admin/v1/auth-platforms", "permission_authPlatform_add"},
		{http.MethodPut, "/api/admin/v1/auth-platforms/:id", "permission_authPlatform_edit"},
		{http.MethodPatch, "/api/admin/v1/auth-platforms/:id/status", "permission_authPlatform_status"},
		{http.MethodDelete, "/api/admin/v1/auth-platforms/:id", "permission_authPlatform_del"},
		{http.MethodDelete, "/api/admin/v1/auth-platforms", "permission_authPlatform_del"},
		{http.MethodPut, "/api/admin/v1/users/:id", "user_userManager_edit"},
		{http.MethodPatch, "/api/admin/v1/users/:id/status", "user_userManager_edit"},
		{http.MethodPatch, "/api/admin/v1/users", "user_userManager_batchEdit"},
		{http.MethodDelete, "/api/admin/v1/users/:id", "user_userManager_del"},
		{http.MethodDelete, "/api/admin/v1/users", "user_userManager_del"},
		{http.MethodDelete, "/api/admin/v1/operation-logs/:id", "devTools_operationLog_del"},
		{http.MethodDelete, "/api/admin/v1/operation-logs", "devTools_operationLog_del"},
		{http.MethodPost, "/api/admin/v1/system-settings", "system_setting_add"},
		{http.MethodPut, "/api/admin/v1/system-settings/:id", "system_setting_edit"},
		{http.MethodPatch, "/api/admin/v1/system-settings/:id/status", "system_setting_status"},
		{http.MethodDelete, "/api/admin/v1/system-settings/:id", "system_setting_del"},
		{http.MethodDelete, "/api/admin/v1/system-settings", "system_setting_del"},
		{http.MethodPost, "/api/admin/v1/upload-drivers", "system_uploadConfig_driverAdd"},
		{http.MethodPut, "/api/admin/v1/upload-drivers/:id", "system_uploadConfig_driverEdit"},
		{http.MethodDelete, "/api/admin/v1/upload-drivers/:id", "system_uploadConfig_driverDel"},
		{http.MethodDelete, "/api/admin/v1/upload-drivers", "system_uploadConfig_driverDel"},
		{http.MethodPost, "/api/admin/v1/upload-rules", "system_uploadConfig_ruleAdd"},
		{http.MethodPut, "/api/admin/v1/upload-rules/:id", "system_uploadConfig_ruleEdit"},
		{http.MethodDelete, "/api/admin/v1/upload-rules/:id", "system_uploadConfig_ruleDel"},
		{http.MethodDelete, "/api/admin/v1/upload-rules", "system_uploadConfig_ruleDel"},
		{http.MethodPost, "/api/admin/v1/upload-settings", "system_uploadConfig_settingAdd"},
		{http.MethodPut, "/api/admin/v1/upload-settings/:id", "system_uploadConfig_settingEdit"},
		{http.MethodPatch, "/api/admin/v1/upload-settings/:id/status", "system_uploadConfig_settingStatus"},
		{http.MethodDelete, "/api/admin/v1/upload-settings/:id", "system_uploadConfig_settingDel"},
		{http.MethodDelete, "/api/admin/v1/upload-settings", "system_uploadConfig_settingDel"},
		{http.MethodPost, "/api/admin/v1/upload-tokens", "system_uploadToken_create"},
		{http.MethodGet, "/api/admin/v1/system-logs/files", "system_log_files"},
		{http.MethodGet, "/api/admin/v1/system-logs/files/:name/lines", "system_log_content"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			got := rules[middleware.NewRouteKey(tt.method, tt.path)]
			if got != tt.code {
				t.Fatalf("expected %q, got %q", tt.code, got)
			}
		})
	}

	if _, ok := rules[middleware.NewRouteKey(http.MethodGet, "/api/admin/v1/permissions")]; ok {
		t.Fatalf("permission list must not gain an implicit button-rule fallback")
	}
	if _, ok := rules[middleware.NewRouteKey(http.MethodGet, "/api/admin/v1/roles")]; ok {
		t.Fatalf("role list must not gain an implicit button-rule fallback")
	}
	if _, ok := rules[middleware.NewRouteKey(http.MethodPost, "/api/admin/Permission/add")]; ok {
		t.Fatalf("new route rules must not carry legacy all-post endpoints")
	}
	if _, ok := rules[middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/profile")]; ok {
		t.Fatalf("current profile update must not require user-manager button permission")
	}
	for _, path := range []string{
		"/api/admin/v1/profile/security/password",
		"/api/admin/v1/profile/security/email",
		"/api/admin/v1/profile/security/phone",
	} {
		if _, ok := rules[middleware.NewRouteKey(http.MethodPut, path)]; ok {
			t.Fatalf("current profile security route %s must not require user-manager button permission", path)
		}
	}
	for _, tt := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/admin/v1/notifications/init"},
		{http.MethodGet, "/api/admin/v1/notifications"},
		{http.MethodGet, "/api/admin/v1/notifications/unread-count"},
		{http.MethodPatch, "/api/admin/v1/notifications/:id/read"},
		{http.MethodPatch, "/api/admin/v1/notifications/read"},
		{http.MethodDelete, "/api/admin/v1/notifications/:id"},
		{http.MethodDelete, "/api/admin/v1/notifications"},
	} {
		if _, ok := rules[middleware.NewRouteKey(tt.method, tt.path)]; ok {
			t.Fatalf("current-user notification route %s %s must not require RBAC button permission", tt.method, tt.path)
		}
	}
}

func TestOperationRouteRulesUseExplicitRESTPatterns(t *testing.T) {
	rules := operationRouteRules()

	tests := []struct {
		method string
		path   string
		action string
	}{
		{http.MethodPost, "/api/admin/v1/permissions", "create"},
		{http.MethodPut, "/api/admin/v1/permissions/:id", "update"},
		{http.MethodPatch, "/api/admin/v1/permissions/:id/status", "change_status"},
		{http.MethodDelete, "/api/admin/v1/permissions/:id", "delete"},
		{http.MethodDelete, "/api/admin/v1/permissions", "delete_batch"},
		{http.MethodPost, "/api/admin/v1/roles", "create"},
		{http.MethodPut, "/api/admin/v1/roles/:id", "update"},
		{http.MethodPatch, "/api/admin/v1/roles/:id/default", "set_default"},
		{http.MethodDelete, "/api/admin/v1/roles/:id", "delete"},
		{http.MethodDelete, "/api/admin/v1/roles", "delete_batch"},
		{http.MethodPost, "/api/admin/v1/auth-platforms", "create"},
		{http.MethodPut, "/api/admin/v1/auth-platforms/:id", "update"},
		{http.MethodPatch, "/api/admin/v1/auth-platforms/:id/status", "change_status"},
		{http.MethodDelete, "/api/admin/v1/auth-platforms/:id", "delete"},
		{http.MethodDelete, "/api/admin/v1/auth-platforms", "delete_batch"},
		{http.MethodPut, "/api/admin/v1/users/:id", "update"},
		{http.MethodPatch, "/api/admin/v1/users/:id/status", "change_status"},
		{http.MethodPatch, "/api/admin/v1/users", "batch_update_profile"},
		{http.MethodDelete, "/api/admin/v1/users/:id", "delete"},
		{http.MethodDelete, "/api/admin/v1/users", "delete_batch"},
		{http.MethodPut, "/api/admin/v1/profile", "update_profile"},
		{http.MethodPut, "/api/admin/v1/profile/security/password", "update_password"},
		{http.MethodPut, "/api/admin/v1/profile/security/email", "update_email"},
		{http.MethodPut, "/api/admin/v1/profile/security/phone", "update_phone"},
		{http.MethodDelete, "/api/admin/v1/operation-logs/:id", "delete"},
		{http.MethodDelete, "/api/admin/v1/operation-logs", "delete_batch"},
		{http.MethodPost, "/api/admin/v1/system-settings", "create"},
		{http.MethodPut, "/api/admin/v1/system-settings/:id", "update"},
		{http.MethodPatch, "/api/admin/v1/system-settings/:id/status", "change_status"},
		{http.MethodDelete, "/api/admin/v1/system-settings/:id", "delete"},
		{http.MethodDelete, "/api/admin/v1/system-settings", "delete_batch"},
		{http.MethodPost, "/api/admin/v1/upload-drivers", "create"},
		{http.MethodPut, "/api/admin/v1/upload-drivers/:id", "update"},
		{http.MethodDelete, "/api/admin/v1/upload-drivers/:id", "delete"},
		{http.MethodDelete, "/api/admin/v1/upload-drivers", "delete_batch"},
		{http.MethodPost, "/api/admin/v1/upload-rules", "create"},
		{http.MethodPut, "/api/admin/v1/upload-rules/:id", "update"},
		{http.MethodDelete, "/api/admin/v1/upload-rules/:id", "delete"},
		{http.MethodDelete, "/api/admin/v1/upload-rules", "delete_batch"},
		{http.MethodPost, "/api/admin/v1/upload-settings", "create"},
		{http.MethodPut, "/api/admin/v1/upload-settings/:id", "update"},
		{http.MethodPatch, "/api/admin/v1/upload-settings/:id/status", "change_status"},
		{http.MethodDelete, "/api/admin/v1/upload-settings/:id", "delete"},
		{http.MethodDelete, "/api/admin/v1/upload-settings", "delete_batch"},
		{http.MethodPost, "/api/admin/v1/upload-tokens", "create"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			got := rules[middleware.NewRouteKey(tt.method, tt.path)]
			if got.Action != tt.action || got.Title == "" {
				t.Fatalf("unexpected operation rule: %#v", got)
			}
		})
	}

	for _, tt := range []struct {
		method string
		path   string
	}{
		{http.MethodPatch, "/api/admin/v1/notifications/:id/read"},
		{http.MethodPatch, "/api/admin/v1/notifications/read"},
		{http.MethodDelete, "/api/admin/v1/notifications/:id"},
		{http.MethodDelete, "/api/admin/v1/notifications"},
	} {
		if _, ok := rules[middleware.NewRouteKey(tt.method, tt.path)]; ok {
			t.Fatalf("current-user notification route %s %s must not write operation log by implicit metadata", tt.method, tt.path)
		}
	}
}
