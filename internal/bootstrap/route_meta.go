package bootstrap

import (
	"net/http"

	"admin_back_go/internal/middleware"
)

func permissionRouteRules() map[middleware.RouteKey]string {
	return map[middleware.RouteKey]string{
		middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/permissions"):             "permission_permission_add",
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/permissions/:id"):          "permission_permission_edit",
		middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/permissions/:id/status"): "permission_permission_status",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/permissions/:id"):       "permission_permission_del",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/permissions"):           "permission_permission_del",
		middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/roles"):                   "permission_role_add",
		middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/roles/:id"):                "permission_role_edit",
		middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/roles/:id/default"):      "permission_role_setDefault",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/roles/:id"):             "permission_role_del",
		middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/roles"):                 "permission_role_del",
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
	}
}

