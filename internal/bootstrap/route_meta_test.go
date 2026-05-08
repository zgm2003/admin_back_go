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
		{http.MethodPost, "/api/admin/v1/pay-channels", "pay_channel_add"},
		{http.MethodPut, "/api/admin/v1/pay-channels/:id", "pay_channel_edit"},
		{http.MethodPatch, "/api/admin/v1/pay-channels/:id/status", "pay_channel_status"},
		{http.MethodDelete, "/api/admin/v1/pay-channels/:id", "pay_channel_del"},
		{http.MethodGet, "/api/admin/v1/pay-notify-logs/page-init", "pay_notify_view"},
		{http.MethodGet, "/api/admin/v1/pay-notify-logs", "pay_notify_view"},
		{http.MethodGet, "/api/admin/v1/pay-notify-logs/:id", "pay_notify_view"},
		{http.MethodGet, "/api/admin/v1/pay-transactions/page-init", "pay_transaction_list"},
		{http.MethodGet, "/api/admin/v1/pay-transactions", "pay_transaction_list"},
		{http.MethodGet, "/api/admin/v1/pay-transactions/:id", "pay_transaction_list"},
		{http.MethodGet, "/api/admin/v1/pay-reconcile-tasks/page-init", "pay_reconcile_list"},
		{http.MethodGet, "/api/admin/v1/pay-reconcile-tasks", "pay_reconcile_list"},
		{http.MethodGet, "/api/admin/v1/pay-reconcile-tasks/:id", "pay_reconcile_list"},
		{http.MethodPatch, "/api/admin/v1/pay-reconcile-tasks/:id/retry", "pay_reconcile_retry"},
		{http.MethodGet, "/api/admin/v1/pay-reconcile-tasks/:id/files/:type", "pay_reconcile_download"},
		{http.MethodGet, "/api/admin/v1/pay-orders/page-init", "pay_recharge_list"},
		{http.MethodGet, "/api/admin/v1/pay-orders/status-count", "pay_recharge_list"},
		{http.MethodGet, "/api/admin/v1/pay-orders", "pay_recharge_list"},
		{http.MethodGet, "/api/admin/v1/pay-orders/:id", "pay_recharge_list"},
		{http.MethodPatch, "/api/admin/v1/pay-orders/:id/close", "pay_order_edit"},
		{http.MethodPatch, "/api/admin/v1/pay-orders/:id/remark", "pay_order_edit"},
		{http.MethodGet, "/api/admin/v1/wallets/page-init", "pay_wallet_list"},
		{http.MethodGet, "/api/admin/v1/wallets", "pay_wallet_list"},
		{http.MethodGet, "/api/admin/v1/wallet-transactions", "pay_wallet_list"},
		{http.MethodPost, "/api/admin/v1/wallet-adjustments", "pay_wallet_adjust"},
		{http.MethodPost, "/api/admin/v1/client-versions", "system_clientVersion_add"},
		{http.MethodPut, "/api/admin/v1/client-versions/:id", "system_clientVersion_edit"},
		{http.MethodPatch, "/api/admin/v1/client-versions/:id/latest", "system_clientVersion_setLatest"},
		{http.MethodPatch, "/api/admin/v1/client-versions/:id/force-update", "system_clientVersion_forceUpdate"},
		{http.MethodDelete, "/api/admin/v1/client-versions/:id", "system_clientVersion_del"},
		{http.MethodPost, "/api/admin/v1/ai-prompts", "ai_prompt_add"},
		{http.MethodPut, "/api/admin/v1/ai-prompts/:id", "ai_prompt_edit"},
		{http.MethodPatch, "/api/admin/v1/ai-prompts/:id/favorite", "ai_prompt_edit"},
		{http.MethodDelete, "/api/admin/v1/ai-prompts/:id", "ai_prompt_del"},
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
		{http.MethodGet, "/api/admin/v1/recharge-orders"},
		{http.MethodPost, "/api/admin/v1/recharge-orders"},
		{http.MethodGet, "/api/admin/v1/recharge-orders/:order_no/result"},
		{http.MethodPatch, "/api/admin/v1/recharge-orders/:order_no/cancel"},
		{http.MethodPost, "/api/admin/v1/recharge-orders/:order_no/pay-attempts"},
		{http.MethodGet, "/api/admin/v1/wallet/summary"},
		{http.MethodGet, "/api/admin/v1/wallet/bills"},
		{http.MethodPost, "/api/pay/notify/alipay"},
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
		{http.MethodGet, "/api/admin/v1/ai-models/page-init"},
		{http.MethodGet, "/api/admin/v1/ai-models"},
		{http.MethodPost, "/api/admin/v1/ai-models"},
		{http.MethodPut, "/api/admin/v1/ai-models/:id"},
		{http.MethodPatch, "/api/admin/v1/ai-models/:id/status"},
		{http.MethodDelete, "/api/admin/v1/ai-models/:id"},
		{http.MethodGet, "/api/admin/v1/ai-tools/page-init"},
		{http.MethodGet, "/api/admin/v1/ai-tools"},
		{http.MethodPost, "/api/admin/v1/ai-tools"},
		{http.MethodPut, "/api/admin/v1/ai-tools/:id"},
		{http.MethodPatch, "/api/admin/v1/ai-tools/:id/status"},
		{http.MethodDelete, "/api/admin/v1/ai-tools/:id"},
		{http.MethodGet, "/api/admin/v1/ai-tools/agent-options"},
		{http.MethodPut, "/api/admin/v1/ai-tools/agent-bindings/:agent_id"},
		{http.MethodGet, "/api/admin/v1/ai-prompts"},
		{http.MethodGet, "/api/admin/v1/ai-prompts/:id"},
		{http.MethodPost, "/api/admin/v1/ai-prompts/:id/use"},
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
		{http.MethodPost, "/api/admin/v1/pay-channels", "create"},
		{http.MethodPut, "/api/admin/v1/pay-channels/:id", "update"},
		{http.MethodPatch, "/api/admin/v1/pay-channels/:id/status", "change_status"},
		{http.MethodDelete, "/api/admin/v1/pay-channels/:id", "delete"},
		{http.MethodPatch, "/api/admin/v1/pay-orders/:id/close", "close"},
		{http.MethodPatch, "/api/admin/v1/pay-orders/:id/remark", "remark"},
		{http.MethodPatch, "/api/admin/v1/pay-reconcile-tasks/:id/retry", "retry"},
		{http.MethodPost, "/api/admin/v1/wallet-adjustments", "adjust"},
		{http.MethodPost, "/api/admin/v1/client-versions", "create"},
		{http.MethodPut, "/api/admin/v1/client-versions/:id", "update"},
		{http.MethodPatch, "/api/admin/v1/client-versions/:id/latest", "set_latest"},
		{http.MethodPatch, "/api/admin/v1/client-versions/:id/force-update", "force_update"},
		{http.MethodDelete, "/api/admin/v1/client-versions/:id", "delete"},
		{http.MethodPost, "/api/admin/v1/ai-models", "create"},
		{http.MethodPut, "/api/admin/v1/ai-models/:id", "update"},
		{http.MethodPatch, "/api/admin/v1/ai-models/:id/status", "change_status"},
		{http.MethodDelete, "/api/admin/v1/ai-models/:id", "delete"},
		{http.MethodPost, "/api/admin/v1/ai-tools", "create"},
		{http.MethodPut, "/api/admin/v1/ai-tools/:id", "update"},
		{http.MethodPatch, "/api/admin/v1/ai-tools/:id/status", "change_status"},
		{http.MethodDelete, "/api/admin/v1/ai-tools/:id", "delete"},
		{http.MethodPut, "/api/admin/v1/ai-tools/agent-bindings/:agent_id", "bind_agent_tools"},
		{http.MethodPost, "/api/admin/v1/ai-prompts", "create"},
		{http.MethodPut, "/api/admin/v1/ai-prompts/:id", "update"},
		{http.MethodDelete, "/api/admin/v1/ai-prompts/:id", "delete"},
		{http.MethodPatch, "/api/admin/v1/ai-prompts/:id/favorite", "toggle_favorite"},
		{http.MethodPost, "/api/admin/v1/ai-prompts/:id/use", "use"},
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

	walletRule := rules[middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/wallet-adjustments")]
	if walletRule.Module != "pay_wallet" || walletRule.Action != "adjust" || walletRule.Title != "钱包调账" {
		t.Fatalf("wallet adjustment operation rule mismatch: %#v", walletRule)
	}

	reconcileRetryRule := rules[middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/pay-reconcile-tasks/:id/retry")]
	if reconcileRetryRule.Module != "pay_reconcile" || reconcileRetryRule.Action != "retry" || reconcileRetryRule.Title != "重试对账任务" {
		t.Fatalf("pay reconcile retry operation rule mismatch: %#v", reconcileRetryRule)
	}

	for _, tt := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/admin/v1/recharge-orders"},
		{http.MethodPost, "/api/admin/v1/recharge-orders"},
		{http.MethodGet, "/api/admin/v1/recharge-orders/:order_no/result"},
		{http.MethodPatch, "/api/admin/v1/recharge-orders/:order_no/cancel"},
		{http.MethodPost, "/api/admin/v1/recharge-orders/:order_no/pay-attempts"},
		{http.MethodGet, "/api/admin/v1/wallet/summary"},
		{http.MethodGet, "/api/admin/v1/wallet/bills"},
		{http.MethodPost, "/api/pay/notify/alipay"},
		{http.MethodGet, "/api/admin/v1/pay-transactions/page-init"},
		{http.MethodGet, "/api/admin/v1/pay-transactions"},
		{http.MethodGet, "/api/admin/v1/pay-transactions/:id"},
		{http.MethodGet, "/api/admin/v1/pay-notify-logs/page-init"},
		{http.MethodGet, "/api/admin/v1/pay-notify-logs"},
		{http.MethodGet, "/api/admin/v1/pay-notify-logs/:id"},
		{http.MethodGet, "/api/admin/v1/pay-reconcile-tasks/page-init"},
		{http.MethodGet, "/api/admin/v1/pay-reconcile-tasks"},
		{http.MethodGet, "/api/admin/v1/pay-reconcile-tasks/:id"},
		{http.MethodGet, "/api/admin/v1/pay-reconcile-tasks/:id/files/:type"},
		{http.MethodGet, "/api/admin/v1/wallets/page-init"},
		{http.MethodGet, "/api/admin/v1/wallets"},
		{http.MethodGet, "/api/admin/v1/wallet-transactions"},
		{http.MethodPatch, "/api/admin/v1/notifications/:id/read"},
		{http.MethodPatch, "/api/admin/v1/notifications/read"},
		{http.MethodDelete, "/api/admin/v1/notifications/:id"},
		{http.MethodDelete, "/api/admin/v1/notifications"},
		{http.MethodGet, "/api/admin/v1/client-versions/page-init"},
		{http.MethodGet, "/api/admin/v1/client-versions"},
		{http.MethodGet, "/api/admin/v1/client-versions/update-json"},
		{http.MethodGet, "/api/admin/v1/client-versions/current-check"},
		{http.MethodGet, "/api/admin/v1/ai-models/page-init"},
		{http.MethodGet, "/api/admin/v1/ai-models"},
		{http.MethodGet, "/api/admin/v1/ai-tools/page-init"},
		{http.MethodGet, "/api/admin/v1/ai-tools"},
		{http.MethodGet, "/api/admin/v1/ai-tools/agent-options"},
		{http.MethodGet, "/api/admin/v1/ai-prompts"},
		{http.MethodGet, "/api/admin/v1/ai-prompts/:id"},
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
