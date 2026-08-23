package admincontract

import "sort"

type ViewsDocument struct {
	SchemaVersion string          `json:"schema_version"`
	UsersMe       UsersMeContract `json:"users_me"`
	MenuCodes     []string        `json:"menu_codes"`
	Views         []View          `json:"views"`
}

type UsersMeContract struct {
	Method         string         `json:"method"`
	Path           string         `json:"path"`
	OperationID    string         `json:"operation_id"`
	ResponseSchema map[string]any `json:"response_schema"`
}

type View struct {
	Path            string   `json:"path"`
	ViewKey         string   `json:"view_key"`
	I18nKey         string   `json:"i18n_key"`
	ShowMenu        int      `json:"show_menu"`
	PermissionCodes []string `json:"permission_codes"`
}

func buildViewsDocument() ViewsDocument {
	views := []View{
		{Path: "/ai/agents", ViewKey: "ai/agents", I18nKey: "menu.ai_agents", ShowMenu: 1},
		{Path: "/ai/chat", ViewKey: "ai/chat", I18nKey: "menu.ai_chat", ShowMenu: 1},
		{Path: "/ai/context", ViewKey: "ai/context", I18nKey: "menu.ai_context", ShowMenu: 1, PermissionCodes: []string{"ai_context_document_manage", "ai_context_evaluate", "ai_context_manage", "ai_context_profile_manage", "ai_context_view"}},
		{Path: "/ai/official-models", ViewKey: "ai/official-models", I18nKey: "menu.ai_official_models", ShowMenu: 1, PermissionCodes: []string{"ai_official_model_list"}},
		{Path: "/ai/providers", ViewKey: "ai/providers", I18nKey: "menu.ai_providers", ShowMenu: 1},
		{Path: "/ai/runs", ViewKey: "ai/runs", I18nKey: "menu.ai_runs", ShowMenu: 1, PermissionCodes: []string{"ai_run_list"}},
		{Path: "/ai/tools", ViewKey: "ai/tools", I18nKey: "menu.ai_tools", ShowMenu: 1},
		{Path: "/component/display", ViewKey: "component/display", I18nKey: "menu.component_display", ShowMenu: 1},
		{Path: "/component/download", ViewKey: "component/download", I18nKey: "menu.component_download", ShowMenu: 1},
		{Path: "/component/effect", ViewKey: "component/effect", I18nKey: "menu.component_effect", ShowMenu: 1},
		{Path: "/component/form", ViewKey: "component/form", I18nKey: "menu.component_form", ShowMenu: 1},
		{Path: "/component/upload", ViewKey: "component/upload", I18nKey: "menu.component_upload", ShowMenu: 1},
		{Path: "/notification", ViewKey: "notification", I18nKey: "menu.notification", ShowMenu: 2},
		{Path: "/payment/config", ViewKey: "payment/config", I18nKey: "menu.payment_config", ShowMenu: 1, PermissionCodes: []string{"payment_config_list"}},
		{Path: "/payment/ledger", ViewKey: "payment/ledger", I18nKey: "menu.payment_ledger", ShowMenu: 1, PermissionCodes: []string{"payment_ledger_list"}},
		{Path: "/payment/recharge", ViewKey: "payment/recharge", I18nKey: "menu.payment_recharge", ShowMenu: 2, PermissionCodes: []string{"payment_recharge_add", "payment_recharge_list", "payment_recharge_pay"}},
		{Path: "/payment/redeem-codes", ViewKey: "payment/redeem-codes", I18nKey: "menu.payment_redeem_codes", ShowMenu: 1, PermissionCodes: []string{"payment_redeem_code_generate", "payment_redeem_code_list", "payment_redeem_code_void"}},
		{Path: "/payment/wallets", ViewKey: "payment/wallets", I18nKey: "menu.payment_wallets", ShowMenu: 1, PermissionCodes: []string{"payment_wallet_list"}},
		{Path: "/permission/authPlatform", ViewKey: "permission/authPlatform", I18nKey: "menu.permission_authPlatform", ShowMenu: 1, PermissionCodes: []string{"permission_authPlatform"}},
		{Path: "/permission/permission", ViewKey: "permission/permission", I18nKey: "menu.permission_permission", ShowMenu: 1, PermissionCodes: []string{"permission_permission"}},
		{Path: "/permission/role", ViewKey: "permission/role", I18nKey: "menu.permission_role", ShowMenu: 1, PermissionCodes: []string{"permission_role"}},
		{Path: "/personal", ViewKey: "personal", I18nKey: "menu.personal", ShowMenu: 2},
		{Path: "/profile/wallet", ViewKey: "profile/wallet", I18nKey: "menu.profile_wallet", ShowMenu: 2, PermissionCodes: []string{"profile_wallet"}},
		{Path: "/system/cronTask", ViewKey: "system/cronTask", I18nKey: "menu.system_cronTask", ShowMenu: 1},
		{Path: "/system/exportTask", ViewKey: "system/exportTask", I18nKey: "menu.system_exportTask", ShowMenu: 1},
		{Path: "/system/log", ViewKey: "system/log", I18nKey: "menu.system_log", ShowMenu: 1},
		{Path: "/system/mail", ViewKey: "system/mail", I18nKey: "menu.system_mail", ShowMenu: 1, PermissionCodes: []string{"system_mail"}},
		{Path: "/system/notificationTask", ViewKey: "system/notificationTask", I18nKey: "menu.system_notificationTask", ShowMenu: 1},
		{Path: "/system/operationLog", ViewKey: "system/operationLog", I18nKey: "menu.system_operationLog", ShowMenu: 1},
		{Path: "/system/queueMonitor", ViewKey: "system/queueMonitor", I18nKey: "menu.system_queueMonitor", ShowMenu: 1, PermissionCodes: []string{"devTools_queueMonitor_list"}},
		{Path: "/system/setting", ViewKey: "system/setting", I18nKey: "menu.system_setting", ShowMenu: 1},
		{Path: "/system/sms", ViewKey: "system/sms", I18nKey: "menu.system_sms", ShowMenu: 1, PermissionCodes: []string{"system_sms"}},
		{Path: "/system/uploadConfig", ViewKey: "system/uploadConfig", I18nKey: "menu.system_uploadConfig", ShowMenu: 1},
		{Path: "/user/userManager", ViewKey: "user/userManager", I18nKey: "menu.user_userManager", ShowMenu: 1, PermissionCodes: []string{"user_userManager"}},
		{Path: "/user/usersLoginLog", ViewKey: "user/usersLoginLog", I18nKey: "menu.user_usersLoginLog", ShowMenu: 1},
	}
	for index := range views {
		views[index].PermissionCodes = sortedUnique(views[index].PermissionCodes)
	}
	sort.Slice(views, func(left int, right int) bool {
		return views[left].ViewKey < views[right].ViewKey
	})
	return ViewsDocument{
		SchemaVersion: ViewSchemaVersion,
		UsersMe: UsersMeContract{
			Method:         "GET",
			Path:           "/api/admin/v1/users/me",
			OperationID:    "get_api_admin_v1_users_me",
			ResponseSchema: usersMeResponseSchema(viewKeys(views), nil),
		},
		MenuCodes: []string{"payment"},
		Views:     views,
	}
}

func usersMeResponseSchema(viewKeys []string, permissionCodes []string) map[string]any {
	stringProperty := map[string]any{"type": "string"}
	buttonCodeItems := map[string]any{"type": "string"}
	if len(permissionCodes) > 0 {
		buttonCodeItems["enum"] = append([]string(nil), permissionCodes...)
	}
	menuItem := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"index", "label", "path", "icon", "children", "i18n_key", "sort", "show_menu", "parent_id"},
		"properties": map[string]any{
			"index":     stringProperty,
			"label":     stringProperty,
			"path":      stringProperty,
			"icon":      stringProperty,
			"children":  map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/menu_item"}},
			"i18n_key":  stringProperty,
			"sort":      map[string]any{"type": "integer"},
			"show_menu": map[string]any{"type": "integer", "enum": []int{1, 2}},
			"parent_id": map[string]any{"type": "integer", "format": "int64", "minimum": 0},
		},
	}
	routeItem := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"name", "path", "view_key", "meta"},
		"properties": map[string]any{
			"name":     stringProperty,
			"path":     stringProperty,
			"view_key": map[string]any{"type": "string", "enum": viewKeys},
			"meta": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
			},
		},
	}
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://contracts.admin.local/v1/users-me-response.schema.json",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"user_id", "username", "avatar", "role_name", "permissions", "router", "buttonCodes"},
		"properties": map[string]any{
			"user_id":     map[string]any{"type": "integer", "format": "int64", "minimum": 1},
			"username":    stringProperty,
			"avatar":      stringProperty,
			"role_name":   stringProperty,
			"permissions": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/menu_item"}},
			"router":      map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/route_item"}},
			"buttonCodes": map[string]any{"type": "array", "items": buttonCodeItems, "uniqueItems": true},
		},
		"$defs": map[string]any{
			"menu_item":  menuItem,
			"route_item": routeItem,
		},
	}
}

func viewKeys(views []View) []string {
	keys := make([]string, 0, len(views))
	for _, view := range views {
		keys = append(keys, view.ViewKey)
	}
	return keys
}

func viewPermissionCodes(document ViewsDocument) []string {
	codes := append([]string(nil), document.MenuCodes...)
	for _, view := range document.Views {
		codes = append(codes, view.PermissionCodes...)
	}
	return sortedUnique(codes)
}

func sortedUnique(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
