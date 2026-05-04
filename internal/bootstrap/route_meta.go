package bootstrap

import (
	"net/http"

	"admin_back_go/internal/middleware"
)

func permissionRouteRules() map[middleware.RouteKey]string {
	return map[middleware.RouteKey]string{
		middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/permissions"):                  "permission_permission_add",
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/permissions/:id"):               "permission_permission_edit",
		middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/permissions/:id/status"):      "permission_permission_status",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/permissions/:id"):            "permission_permission_del",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/permissions"):                "permission_permission_del",
		middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/roles"):                        "permission_role_add",
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/roles/:id"):                     "permission_role_edit",
		middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/roles/:id/default"):           "permission_role_setDefault",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/roles/:id"):                  "permission_role_del",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/roles"):                      "permission_role_del",
		middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/auth-platforms"):               "permission_authPlatform_add",
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/auth-platforms/:id"):            "permission_authPlatform_edit",
		middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/auth-platforms/:id/status"):   "permission_authPlatform_status",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/auth-platforms/:id"):         "permission_authPlatform_del",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/auth-platforms"):             "permission_authPlatform_del",
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/users/:id"):                     "user_userManager_edit",
		middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/users/:id/status"):            "user_userManager_edit",
		middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/users"):                       "user_userManager_batchEdit",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/users/:id"):                  "user_userManager_del",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/users"):                      "user_userManager_del",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/operation-logs/:id"):         "devTools_operationLog_del",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/operation-logs"):             "devTools_operationLog_del",
		middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/system-settings"):              "system_setting_add",
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/system-settings/:id"):           "system_setting_edit",
		middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/system-settings/:id/status"):  "system_setting_status",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/system-settings/:id"):        "system_setting_del",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/system-settings"):            "system_setting_del",
		middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/upload-drivers"):               "system_uploadConfig_driverAdd",
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/upload-drivers/:id"):            "system_uploadConfig_driverEdit",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/upload-drivers/:id"):         "system_uploadConfig_driverDel",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/upload-drivers"):             "system_uploadConfig_driverDel",
		middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/upload-rules"):                 "system_uploadConfig_ruleAdd",
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/upload-rules/:id"):              "system_uploadConfig_ruleEdit",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/upload-rules/:id"):           "system_uploadConfig_ruleDel",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/upload-rules"):               "system_uploadConfig_ruleDel",
		middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/upload-settings"):              "system_uploadConfig_settingAdd",
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/upload-settings/:id"):           "system_uploadConfig_settingEdit",
		middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/upload-settings/:id/status"):  "system_uploadConfig_settingStatus",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/upload-settings/:id"):        "system_uploadConfig_settingDel",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/upload-settings"):            "system_uploadConfig_settingDel",
		middleware.NewRouteKey(http.MethodGet, "/api/admin/v1/system-logs/files"):             "system_log_files",
		middleware.NewRouteKey(http.MethodGet, "/api/admin/v1/system-logs/files/:name/lines"): "system_log_content",
	}
}

func operationRouteRules() map[middleware.RouteKey]middleware.OperationRule {
	return map[middleware.RouteKey]middleware.OperationRule{
		middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/permissions"): {
			Module: "permission",
			Action: "create",
			Title:  "新增权限",
		},
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/permissions/:id"): {
			Module: "permission",
			Action: "update",
			Title:  "编辑权限",
		},
		middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/permissions/:id/status"): {
			Module: "permission",
			Action: "change_status",
			Title:  "修改权限状态",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/permissions/:id"): {
			Module: "permission",
			Action: "delete",
			Title:  "删除权限",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/permissions"): {
			Module: "permission",
			Action: "delete_batch",
			Title:  "批量删除权限",
		},
		middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/roles"): {
			Module: "role",
			Action: "create",
			Title:  "新增角色",
		},
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/roles/:id"): {
			Module: "role",
			Action: "update",
			Title:  "编辑角色",
		},
		middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/roles/:id/default"): {
			Module: "role",
			Action: "set_default",
			Title:  "设置默认角色",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/roles/:id"): {
			Module: "role",
			Action: "delete",
			Title:  "删除角色",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/roles"): {
			Module: "role",
			Action: "delete_batch",
			Title:  "批量删除角色",
		},
		middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/auth-platforms"): {
			Module: "auth_platform",
			Action: "create",
			Title:  "新增认证平台",
		},
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/auth-platforms/:id"): {
			Module: "auth_platform",
			Action: "update",
			Title:  "编辑认证平台",
		},
		middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/auth-platforms/:id/status"): {
			Module: "auth_platform",
			Action: "change_status",
			Title:  "修改认证平台状态",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/auth-platforms/:id"): {
			Module: "auth_platform",
			Action: "delete",
			Title:  "删除认证平台",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/auth-platforms"): {
			Module: "auth_platform",
			Action: "delete_batch",
			Title:  "批量删除认证平台",
		},
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/users/:id"): {
			Module: "user",
			Action: "update",
			Title:  "编辑用户",
		},
		middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/users/:id/status"): {
			Module: "user",
			Action: "change_status",
			Title:  "修改用户状态",
		},
		middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/users"): {
			Module: "user",
			Action: "batch_update_profile",
			Title:  "批量修改用户资料",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/users/:id"): {
			Module: "user",
			Action: "delete",
			Title:  "删除用户",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/users"): {
			Module: "user",
			Action: "delete_batch",
			Title:  "批量删除用户",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/operation-logs/:id"): {
			Module: "operation_log",
			Action: "delete",
			Title:  "删除操作日志",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/operation-logs"): {
			Module: "operation_log",
			Action: "delete_batch",
			Title:  "批量删除操作日志",
		},
		middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/system-settings"): {
			Module: "system_setting",
			Action: "create",
			Title:  "新增系统设置",
		},
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/system-settings/:id"): {
			Module: "system_setting",
			Action: "update",
			Title:  "编辑系统设置",
		},
		middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/system-settings/:id/status"): {
			Module: "system_setting",
			Action: "change_status",
			Title:  "修改系统设置状态",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/system-settings/:id"): {
			Module: "system_setting",
			Action: "delete",
			Title:  "删除系统设置",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/system-settings"): {
			Module: "system_setting",
			Action: "delete_batch",
			Title:  "批量删除系统设置",
		},
		middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/upload-drivers"): {
			Module: "upload_driver",
			Action: "create",
			Title:  "新增上传驱动",
		},
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/upload-drivers/:id"): {
			Module: "upload_driver",
			Action: "update",
			Title:  "编辑上传驱动",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/upload-drivers/:id"): {
			Module: "upload_driver",
			Action: "delete",
			Title:  "删除上传驱动",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/upload-drivers"): {
			Module: "upload_driver",
			Action: "delete_batch",
			Title:  "批量删除上传驱动",
		},
		middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/upload-rules"): {
			Module: "upload_rule",
			Action: "create",
			Title:  "新增上传规则",
		},
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/upload-rules/:id"): {
			Module: "upload_rule",
			Action: "update",
			Title:  "编辑上传规则",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/upload-rules/:id"): {
			Module: "upload_rule",
			Action: "delete",
			Title:  "删除上传规则",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/upload-rules"): {
			Module: "upload_rule",
			Action: "delete_batch",
			Title:  "批量删除上传规则",
		},
		middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/upload-settings"): {
			Module: "upload_setting",
			Action: "create",
			Title:  "新增上传设置",
		},
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/upload-settings/:id"): {
			Module: "upload_setting",
			Action: "update",
			Title:  "编辑上传设置",
		},
		middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/upload-settings/:id/status"): {
			Module: "upload_setting",
			Action: "change_status",
			Title:  "修改上传设置状态",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/upload-settings/:id"): {
			Module: "upload_setting",
			Action: "delete",
			Title:  "删除上传设置",
		},
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/upload-settings"): {
			Module: "upload_setting",
			Action: "delete_batch",
			Title:  "批量删除上传设置",
		},
	}
}
