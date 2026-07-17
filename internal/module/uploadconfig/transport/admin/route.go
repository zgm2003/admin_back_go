package admin

import (
	"net/http"

	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/upload-drivers/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.DriverPageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/upload-drivers",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.DriverList)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/upload-drivers",
		Access: adminroute.Permission("system_uploadConfig_driverAdd"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "upload_driver",
			Action:  "create",
			Title:   "新增上传驱动",
		},
	}, handler.DriverCreate)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/upload-drivers/:id",
		Access: adminroute.Permission("system_uploadConfig_driverEdit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "upload_driver",
			Action:  "update",
			Title:   "编辑上传驱动",
		},
	}, handler.DriverUpdate)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/upload-drivers/:id",
		Access: adminroute.Permission("system_uploadConfig_driverDel"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "upload_driver",
			Action:  "delete",
			Title:   "删除上传驱动",
		},
	}, handler.DriverDeleteOne)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/upload-drivers",
		Access: adminroute.Permission("system_uploadConfig_driverDel"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "upload_driver",
			Action:  "delete_batch",
			Title:   "批量删除上传驱动",
		},
	}, handler.DriverDeleteBatch)

	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/upload-rules/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.RulePageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/upload-rules",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.RuleList)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/upload-rules",
		Access: adminroute.Permission("system_uploadConfig_ruleAdd"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "upload_rule",
			Action:  "create",
			Title:   "新增上传规则",
		},
	}, handler.RuleCreate)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/upload-rules/:id",
		Access: adminroute.Permission("system_uploadConfig_ruleEdit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "upload_rule",
			Action:  "update",
			Title:   "编辑上传规则",
		},
	}, handler.RuleUpdate)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/upload-rules/:id",
		Access: adminroute.Permission("system_uploadConfig_ruleDel"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "upload_rule",
			Action:  "delete",
			Title:   "删除上传规则",
		},
	}, handler.RuleDeleteOne)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/upload-rules",
		Access: adminroute.Permission("system_uploadConfig_ruleDel"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "upload_rule",
			Action:  "delete_batch",
			Title:   "批量删除上传规则",
		},
	}, handler.RuleDeleteBatch)

	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/upload-settings/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.SettingPageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/upload-settings",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.SettingList)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/upload-settings",
		Access: adminroute.Permission("system_uploadConfig_settingAdd"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "upload_setting",
			Action:  "create",
			Title:   "新增上传设置",
		},
	}, handler.SettingCreate)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/upload-settings/:id",
		Access: adminroute.Permission("system_uploadConfig_settingEdit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "upload_setting",
			Action:  "update",
			Title:   "编辑上传设置",
		},
	}, handler.SettingUpdate)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/upload-settings/:id/status",
		Access: adminroute.Permission("system_uploadConfig_settingStatus"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "upload_setting",
			Action:  "change_status",
			Title:   "修改上传设置状态",
		},
	}, handler.SettingChangeStatus)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/upload-settings/:id",
		Access: adminroute.Permission("system_uploadConfig_settingDel"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "upload_setting",
			Action:  "delete",
			Title:   "删除上传设置",
		},
	}, handler.SettingDeleteOne)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/upload-settings",
		Access: adminroute.Permission("system_uploadConfig_settingDel"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "upload_setting",
			Action:  "delete_batch",
			Title:   "批量删除上传设置",
		},
	}, handler.SettingDeleteBatch)
}
