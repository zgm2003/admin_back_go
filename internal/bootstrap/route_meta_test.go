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
		{http.MethodPost, "/api/admin/v1/users/export", "user_userManager_export"},
		{http.MethodDelete, "/api/admin/v1/users/:id", "user_userManager_del"},
		{http.MethodDelete, "/api/admin/v1/users", "user_userManager_del"},
		{http.MethodPatch, "/api/admin/v1/user-sessions/:id/revoke", "user_userManager_kick"},
		{http.MethodPatch, "/api/admin/v1/user-sessions/revoke", "user_userManager_kick"},
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
		{http.MethodGet, "/api/admin/v1/system-logs/files", "system_log_files"},
		{http.MethodGet, "/api/admin/v1/system-logs/files/:name/lines", "system_log_content"},
		{http.MethodPost, "/api/admin/v1/notification-tasks", "system_notificationTask_add"},
		{http.MethodPatch, "/api/admin/v1/notification-tasks/:id/cancel", "system_notificationTask_cancel"},
		{http.MethodDelete, "/api/admin/v1/notification-tasks/:id", "system_notificationTask_del"},
		{http.MethodPost, "/api/admin/v1/cron-tasks", "devTools_cronTask_add"},
		{http.MethodPut, "/api/admin/v1/cron-tasks/:id", "devTools_cronTask_edit"},
		{http.MethodPatch, "/api/admin/v1/cron-tasks/:id/status", "devTools_cronTask_status"},
		{http.MethodDelete, "/api/admin/v1/cron-tasks/:id", "devTools_cronTask_del"},
		{http.MethodDelete, "/api/admin/v1/cron-tasks", "devTools_cronTask_del"},
		{http.MethodGet, "/api/admin/v1/cron-tasks/:id/logs", "devTools_cronTask_logs"},
		{http.MethodGet, "/api/admin/v1/payment/channels/page-init", "payment_channel_list"},
		{http.MethodGet, "/api/admin/v1/payment/channels", "payment_channel_list"},
		{http.MethodPost, "/api/admin/v1/payment/channels", "payment_channel_add"},
		{http.MethodPut, "/api/admin/v1/payment/channels/:id", "payment_channel_edit"},
		{http.MethodPatch, "/api/admin/v1/payment/channels/:id/status", "payment_channel_status"},
		{http.MethodDelete, "/api/admin/v1/payment/channels/:id", "payment_channel_del"},
		{http.MethodGet, "/api/admin/v1/payment/orders/page-init", "payment_order_list"},
		{http.MethodGet, "/api/admin/v1/payment/orders", "payment_order_list"},
		{http.MethodGet, "/api/admin/v1/payment/orders/:order_no", "payment_order_list"},
		{http.MethodGet, "/api/admin/v1/payment/orders/page-init", "payment_order_list"},
		{http.MethodGet, "/api/admin/v1/payment/orders", "payment_order_list"},
		{http.MethodGet, "/api/admin/v1/payment/orders/:order_no", "payment_order_list"},
		{http.MethodPatch, "/api/admin/v1/payment/orders/:order_no/close", "payment_order_close"},
		{http.MethodGet, "/api/admin/v1/payment/events", "payment_event_list"},
		{http.MethodGet, "/api/admin/v1/payment/events/:id", "payment_event_list"},
		{http.MethodGet, "/api/admin/v1/payment/events", "payment_event_list"},
		{http.MethodGet, "/api/admin/v1/payment/events/:id", "payment_event_list"},
		{http.MethodPost, "/api/admin/v1/client-versions", "system_clientVersion_add"},
		{http.MethodPut, "/api/admin/v1/client-versions/:id", "system_clientVersion_edit"},
		{http.MethodPatch, "/api/admin/v1/client-versions/:id/latest", "system_clientVersion_setLatest"},
		{http.MethodPatch, "/api/admin/v1/client-versions/:id/force-update", "system_clientVersion_forceUpdate"},
		{http.MethodDelete, "/api/admin/v1/client-versions/:id", "system_clientVersion_del"},
		{http.MethodPost, "/api/admin/v1/ai-providers/model-options", "ai_provider_test"},
		{http.MethodPost, "/api/admin/v1/ai-providers", "ai_provider_add"},
		{http.MethodPut, "/api/admin/v1/ai-providers/:id", "ai_provider_edit"},
		{http.MethodPost, "/api/admin/v1/ai-providers/:id/test", "ai_provider_test"},
		{http.MethodPost, "/api/admin/v1/ai-providers/:id/sync-models", "ai_provider_test"},
		{http.MethodPut, "/api/admin/v1/ai-providers/:id/models", "ai_provider_edit"},
		{http.MethodPatch, "/api/admin/v1/ai-providers/:id/status", "ai_provider_status"},
		{http.MethodDelete, "/api/admin/v1/ai-providers/:id", "ai_provider_del"},
		{http.MethodPost, "/api/admin/v1/ai-agents", "ai_agent_add"},
		{http.MethodPut, "/api/admin/v1/ai-agents/:id", "ai_agent_edit"},
		{http.MethodPost, "/api/admin/v1/ai-agents/:id/test", "ai_agent_test"},
		{http.MethodPatch, "/api/admin/v1/ai-agents/:id/status", "ai_agent_status"},
		{http.MethodDelete, "/api/admin/v1/ai-agents/:id", "ai_agent_del"},
		{http.MethodPost, "/api/admin/v1/ai-knowledge-bases", "ai_knowledge_add"},
		{http.MethodPut, "/api/admin/v1/ai-knowledge-bases/:id", "ai_knowledge_edit"},
		{http.MethodPatch, "/api/admin/v1/ai-knowledge-bases/:id/status", "ai_knowledge_status"},
		{http.MethodPost, "/api/admin/v1/ai-knowledge-documents/:id/reindex", "ai_knowledge_reindex"},
		{http.MethodDelete, "/api/admin/v1/ai-knowledge-bases/:id", "ai_knowledge_del"},
		{http.MethodPost, "/api/admin/v1/ai-knowledge-bases/:id/documents", "ai_knowledge_document_add"},
		{http.MethodPatch, "/api/admin/v1/ai-knowledge-documents/:id/status", "ai_knowledge_document_status"},
		{http.MethodPut, "/api/admin/v1/ai-knowledge-documents/:id", "ai_knowledge_document_edit"},
		{http.MethodDelete, "/api/admin/v1/ai-knowledge-documents/:id", "ai_knowledge_document_del"},
		{http.MethodPost, "/api/admin/v1/ai-tools/generate-draft", "ai_tool_generate"},
		{http.MethodPost, "/api/admin/v1/ai-tools", "ai_tool_add"},
		{http.MethodPut, "/api/admin/v1/ai-tools/:id", "ai_tool_edit"},
		{http.MethodPatch, "/api/admin/v1/ai-tools/:id/status", "ai_tool_status"},
		{http.MethodDelete, "/api/admin/v1/ai-tools/:id", "ai_tool_del"},
		{http.MethodPut, "/api/admin/v1/ai-agents/:id/tools", "ai_agent_edit"},
		{http.MethodPost, "/api/admin/v1/ai-knowledge-bases/:id/retrieval-tests", "ai_knowledge_retrieval_test"},
		{http.MethodPut, "/api/admin/v1/ai-agents/:id/knowledge-bases", "ai_agent_binding_add"},
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
	for _, tt := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/admin/v1/ai-knowledge-maps"},
		{http.MethodPut, "/api/admin/v1/ai-knowledge-maps/:id"},
		{http.MethodPost, "/api/admin/v1/ai-knowledge-maps/:id/sync"},
	} {
		if _, ok := rules[middleware.NewRouteKey(tt.method, tt.path)]; ok {
			t.Fatalf("retired AI route must not have permission metadata: %s %s", tt.method, tt.path)
		}
	}
	if _, ok := rules[middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/profile")]; ok {
		t.Fatalf("current profile update must not require user-manager button permission")
	}
	for _, tt := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/admin/v1/payment/orders"},
		{http.MethodGet, "/api/admin/v1/payment/orders/:order_no/result"},
		{http.MethodPost, "/api/admin/v1/payment/orders/:order_no/pay"},
		{http.MethodPatch, "/api/admin/v1/payment/orders/:order_no/cancel"},
		{http.MethodPost, "/api/payment/notify/alipay"},
	} {
		if _, ok := rules[middleware.NewRouteKey(tt.method, tt.path)]; ok {
			t.Fatalf("payment current-user/read/notify route %s %s must not require RBAC button permission", tt.method, tt.path)
		}
	}
	for _, tt := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/admin/v1/pay-channels"},
		{http.MethodGet, "/api/admin/v1/pay-orders/page-init"},
		{http.MethodPost, "/api/admin/v1/wallet-adjustments"},
	} {
		if _, ok := rules[middleware.NewRouteKey(tt.method, tt.path)]; ok {
			t.Fatalf("legacy pay/wallet route metadata must not remain: %s %s", tt.method, tt.path)
		}
	}

	for _, tt := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/admin/v1/payment/orders"},
		{http.MethodGet, "/api/admin/v1/payment/orders/:order_no/result"},
		{http.MethodPost, "/api/admin/v1/payment/orders/:order_no/pay"},
		{http.MethodPatch, "/api/admin/v1/payment/orders/:order_no/cancel"},
		{http.MethodPost, "/api/payment/notify/alipay"},
	} {
		if _, ok := rules[middleware.NewRouteKey(tt.method, tt.path)]; ok {
			t.Fatalf("payment read/current-user/notify route %s %s must not be operation-logged", tt.method, tt.path)
		}
	}
	for _, tt := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/admin/v1/pay-channels"},
		{http.MethodPatch, "/api/admin/v1/pay-orders/:id/close"},
		{http.MethodPost, "/api/admin/v1/wallet-adjustments"},
	} {
		if _, ok := rules[middleware.NewRouteKey(tt.method, tt.path)]; ok {
			t.Fatalf("legacy pay/wallet operation metadata must not remain: %s %s", tt.method, tt.path)
		}
	}

	if _, ok := rules[middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/upload-tokens")]; ok {
		t.Fatalf("upload token create must be current-user capability and must not require RBAC button permission")
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
		{http.MethodPost, "/api/payment/notify/alipay"},
		{http.MethodGet, "/api/admin/v1/notifications/init"},
		{http.MethodGet, "/api/admin/v1/notifications"},
		{http.MethodGet, "/api/admin/v1/notifications/unread-count"},
		{http.MethodPatch, "/api/admin/v1/notifications/:id/read"},
		{http.MethodPatch, "/api/admin/v1/notifications/read"},
		{http.MethodDelete, "/api/admin/v1/notifications/:id"},
		{http.MethodDelete, "/api/admin/v1/notifications"},
		{http.MethodGet, "/api/admin/v1/client-versions/page-init"},
		{http.MethodGet, "/api/admin/v1/client-versions"},
		{http.MethodGet, "/api/admin/v1/client-versions/update-json"},
		{http.MethodGet, "/api/admin/v1/client-versions/current-check"},
		{http.MethodGet, "/api/admin/v1/ai-providers/page-init"},
		{http.MethodGet, "/api/admin/v1/ai-providers"},
		{http.MethodGet, "/api/admin/v1/ai-providers/:id/models"},
		{http.MethodGet, "/api/admin/v1/ai-agents/page-init"},
		{http.MethodGet, "/api/admin/v1/ai-agents/provider-models/:id"},
		{http.MethodGet, "/api/admin/v1/ai-agents"},
		{http.MethodGet, "/api/admin/v1/ai-agents/options"},
		{http.MethodGet, "/api/admin/v1/ai-agents/:id"},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-bases/page-init"},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-bases"},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-bases/:id"},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-bases/:id/documents"},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-documents/:id"},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-documents/:id/chunks"},
		{http.MethodGet, "/api/admin/v1/ai-agents/:id/knowledge-bases"},
		{http.MethodGet, "/api/admin/v1/ai-tools/page-init"},
		{http.MethodGet, "/api/admin/v1/ai-tools/generate/page-init"},
		{http.MethodGet, "/api/admin/v1/ai-tools"},
		{http.MethodGet, "/api/admin/v1/ai-agents/:id/tools"},
		{http.MethodGet, "/api/admin/v1/ai-conversations"},
		{http.MethodGet, "/api/admin/v1/ai-conversations/:id"},
		{http.MethodPost, "/api/admin/v1/ai-conversations"},
		{http.MethodDelete, "/api/admin/v1/ai-conversations/:id"},
		{http.MethodGet, "/api/admin/v1/ai-conversations/:id/messages"},
		{http.MethodPost, "/api/admin/v1/ai-conversations/:id/messages"},
		{http.MethodGet, "/api/admin/v1/ai-runs/page-init"},
		{http.MethodGet, "/api/admin/v1/ai-runs"},
		{http.MethodGet, "/api/admin/v1/ai-runs/:id"},
		{http.MethodGet, "/api/admin/v1/ai-runs/stats"},
		{http.MethodGet, "/api/admin/v1/ai-runs/stats/by-date"},
		{http.MethodGet, "/api/admin/v1/ai-runs/stats/by-agent"},
		{http.MethodGet, "/api/admin/v1/ai-runs/stats/by-user"},
	} {
		if _, ok := rules[middleware.NewRouteKey(tt.method, tt.path)]; ok {
			t.Fatalf("read/current-user route %s %s must not require RBAC button permission", tt.method, tt.path)
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
		{http.MethodPost, "/api/admin/v1/users/export", "export"},
		{http.MethodDelete, "/api/admin/v1/users/:id", "delete"},
		{http.MethodDelete, "/api/admin/v1/users", "delete_batch"},
		{http.MethodPatch, "/api/admin/v1/user-sessions/:id/revoke", "revoke"},
		{http.MethodPatch, "/api/admin/v1/user-sessions/revoke", "revoke_batch"},
		{http.MethodDelete, "/api/admin/v1/export-tasks/:id", "delete"},
		{http.MethodDelete, "/api/admin/v1/export-tasks", "delete_batch"},
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
		{http.MethodPost, "/api/admin/v1/notification-tasks", "create"},
		{http.MethodPatch, "/api/admin/v1/notification-tasks/:id/cancel", "cancel"},
		{http.MethodDelete, "/api/admin/v1/notification-tasks/:id", "delete"},
		{http.MethodPost, "/api/admin/v1/cron-tasks", "create"},
		{http.MethodPut, "/api/admin/v1/cron-tasks/:id", "update"},
		{http.MethodPatch, "/api/admin/v1/cron-tasks/:id/status", "change_status"},
		{http.MethodDelete, "/api/admin/v1/cron-tasks/:id", "delete"},
		{http.MethodDelete, "/api/admin/v1/cron-tasks", "delete_batch"},
		{http.MethodPost, "/api/admin/v1/payment/channels", "create"},
		{http.MethodPut, "/api/admin/v1/payment/channels/:id", "update"},
		{http.MethodPatch, "/api/admin/v1/payment/channels/:id/status", "change_status"},
		{http.MethodDelete, "/api/admin/v1/payment/channels/:id", "delete"},
		{http.MethodPatch, "/api/admin/v1/payment/orders/:order_no/close", "close"},
		{http.MethodPost, "/api/admin/v1/client-versions", "create"},
		{http.MethodPut, "/api/admin/v1/client-versions/:id", "update"},
		{http.MethodPatch, "/api/admin/v1/client-versions/:id/latest", "set_latest"},
		{http.MethodPatch, "/api/admin/v1/client-versions/:id/force-update", "force_update"},
		{http.MethodDelete, "/api/admin/v1/client-versions/:id", "delete"},
		{http.MethodPost, "/api/admin/v1/ai-providers/model-options", "preview_models"},
		{http.MethodPost, "/api/admin/v1/ai-providers", "create"},
		{http.MethodPut, "/api/admin/v1/ai-providers/:id", "update"},
		{http.MethodPost, "/api/admin/v1/ai-providers/:id/test", "test"},
		{http.MethodPost, "/api/admin/v1/ai-providers/:id/sync-models", "sync_models"},
		{http.MethodPut, "/api/admin/v1/ai-providers/:id/models", "update_models"},
		{http.MethodPatch, "/api/admin/v1/ai-providers/:id/status", "change_status"},
		{http.MethodDelete, "/api/admin/v1/ai-providers/:id", "delete"},
		{http.MethodPost, "/api/admin/v1/ai-agents", "create"},
		{http.MethodPut, "/api/admin/v1/ai-agents/:id", "update"},
		{http.MethodPost, "/api/admin/v1/ai-agents/:id/test", "test"},
		{http.MethodPatch, "/api/admin/v1/ai-agents/:id/status", "change_status"},
		{http.MethodDelete, "/api/admin/v1/ai-agents/:id", "delete"},
		{http.MethodPost, "/api/admin/v1/ai-knowledge-bases", "create"},
		{http.MethodPut, "/api/admin/v1/ai-knowledge-bases/:id", "update"},
		{http.MethodPatch, "/api/admin/v1/ai-knowledge-bases/:id/status", "change_status"},
		{http.MethodPost, "/api/admin/v1/ai-knowledge-documents/:id/reindex", "reindex"},
		{http.MethodDelete, "/api/admin/v1/ai-knowledge-bases/:id", "delete"},
		{http.MethodPost, "/api/admin/v1/ai-knowledge-bases/:id/documents", "create"},
		{http.MethodPatch, "/api/admin/v1/ai-knowledge-documents/:id/status", "change_status"},
		{http.MethodPut, "/api/admin/v1/ai-knowledge-documents/:id", "update"},
		{http.MethodDelete, "/api/admin/v1/ai-knowledge-documents/:id", "delete"},
		{http.MethodPost, "/api/admin/v1/ai-tools/generate-draft", "generate_draft"},
		{http.MethodPost, "/api/admin/v1/ai-tools", "create"},
		{http.MethodPut, "/api/admin/v1/ai-tools/:id", "update"},
		{http.MethodPatch, "/api/admin/v1/ai-tools/:id/status", "change_status"},
		{http.MethodDelete, "/api/admin/v1/ai-tools/:id", "delete"},
		{http.MethodPut, "/api/admin/v1/ai-agents/:id/tools", "update_binding"},
		{http.MethodPost, "/api/admin/v1/ai-knowledge-bases/:id/retrieval-tests", "retrieval_test"},
		{http.MethodPut, "/api/admin/v1/ai-agents/:id/knowledge-bases", "update_binding"},
		{http.MethodPost, "/api/admin/v1/ai-conversations", "create"},
		{http.MethodDelete, "/api/admin/v1/ai-conversations/:id", "delete"},
		{http.MethodPost, "/api/admin/v1/ai-conversations/:id/messages", "send"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			got := rules[middleware.NewRouteKey(tt.method, tt.path)]
			if got.Action != tt.action || got.Title == "" {
				t.Fatalf("unexpected operation rule: %#v", got)
			}
		})
	}

	if _, ok := rules[middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/upload-tokens")]; ok {
		t.Fatalf("upload token create must not be operation-logged because the response contains temporary STS credentials")
	}
	for _, tt := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/admin/v1/ai-knowledge-maps"},
		{http.MethodPut, "/api/admin/v1/ai-knowledge-maps/:id"},
		{http.MethodPost, "/api/admin/v1/ai-knowledge-maps/:id/sync"},
	} {
		if _, ok := rules[middleware.NewRouteKey(tt.method, tt.path)]; ok {
			t.Fatalf("retired AI route must not be operation-logged: %s %s", tt.method, tt.path)
		}
	}

	for _, tt := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/payment/notify/alipay"},
		{http.MethodPatch, "/api/admin/v1/notifications/:id/read"},
		{http.MethodPatch, "/api/admin/v1/notifications/read"},
		{http.MethodDelete, "/api/admin/v1/notifications/:id"},
		{http.MethodDelete, "/api/admin/v1/notifications"},
		{http.MethodGet, "/api/admin/v1/client-versions/page-init"},
		{http.MethodGet, "/api/admin/v1/client-versions"},
		{http.MethodGet, "/api/admin/v1/client-versions/update-json"},
		{http.MethodGet, "/api/admin/v1/client-versions/current-check"},
		{http.MethodGet, "/api/admin/v1/ai-agents/page-init"},
		{http.MethodGet, "/api/admin/v1/ai-agents/provider-models/:id"},
		{http.MethodGet, "/api/admin/v1/ai-agents"},
		{http.MethodGet, "/api/admin/v1/ai-agents/options"},
		{http.MethodGet, "/api/admin/v1/ai-agents/:id"},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-bases/page-init"},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-bases"},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-bases/:id"},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-bases/:id/documents"},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-documents/:id"},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-documents/:id/chunks"},
		{http.MethodGet, "/api/admin/v1/ai-agents/:id/knowledge-bases"},
		{http.MethodGet, "/api/admin/v1/ai-tools/page-init"},
		{http.MethodGet, "/api/admin/v1/ai-tools/generate/page-init"},
		{http.MethodGet, "/api/admin/v1/ai-tools"},
		{http.MethodGet, "/api/admin/v1/ai-agents/:id/tools"},
		{http.MethodGet, "/api/admin/v1/ai-conversations"},
		{http.MethodGet, "/api/admin/v1/ai-conversations/:id"},
		{http.MethodGet, "/api/admin/v1/ai-conversations/:id/messages"},
		{http.MethodGet, "/api/admin/v1/ai-runs/page-init"},
		{http.MethodGet, "/api/admin/v1/ai-runs"},
		{http.MethodGet, "/api/admin/v1/ai-runs/:id"},
		{http.MethodGet, "/api/admin/v1/ai-runs/stats"},
		{http.MethodGet, "/api/admin/v1/ai-runs/stats/by-date"},
		{http.MethodGet, "/api/admin/v1/ai-runs/stats/by-agent"},
		{http.MethodGet, "/api/admin/v1/ai-runs/stats/by-user"},
		{http.MethodGet, "/api/admin/v1/ai-chat/runs/:run_id/events"},
	} {
		if _, ok := rules[middleware.NewRouteKey(tt.method, tt.path)]; ok {
			t.Fatalf("read/current-user route %s %s must not write operation log by implicit metadata", tt.method, tt.path)
		}
	}

	setLatestRule := rules[middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/client-versions/:id/latest")]
	if setLatestRule.Module != "client_version" || setLatestRule.Action != "set_latest" || setLatestRule.Title != "设为最新版本" {
		t.Fatalf("client version set-latest operation rule mismatch: %#v", setLatestRule)
	}
}
