package admin

import (
	"net/http"

	smsmodule "admin_back_go/internal/module/sms"
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
		Path:   "/api/admin/v1/sms/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: smsmodule.PageInitResponse{},
		},
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/sms/config",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: smsmodule.ConfigResponse{},
		},
	}, handler.Config)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/sms/config",
		Access: adminroute.Permission("system_sms_configEdit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "sms",
			Action:  "update_config",
			Title:   "编辑短信配置",
		},
		Contract: &adminroute.HTTPContract{
			Request:  saveConfigRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.SaveConfig)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/sms/config",
		Access: adminroute.Permission("system_sms_configDel"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "sms",
			Action:  "delete_config",
			Title:   "删除短信配置",
		},
		Contract: &adminroute.HTTPContract{
			Response: adminroute.EmptyData{},
		},
	}, handler.DeleteConfig)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/sms/test",
		Access: adminroute.Permission("system_sms_test"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "sms",
			Action:  "test_send",
			Title:   "发送测试短信",
		},
		Contract: &adminroute.HTTPContract{
			Request:  testRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.TestSend)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/sms/templates",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: []smsmodule.TemplateDTO{},
		},
	}, handler.Templates)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/sms/templates",
		Access: adminroute.Permission("system_sms_templateAdd"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "sms",
			Action:  "create_template",
			Title:   "新增短信模板",
		},
		Contract: &adminroute.HTTPContract{
			Request:  templateRequest{},
			Response: adminroute.IDData{},
		},
	}, handler.CreateTemplate)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/sms/templates/:id",
		Access: adminroute.Permission("system_sms_templateEdit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "sms",
			Action:  "update_template",
			Title:   "编辑短信模板",
		},
		Contract: &adminroute.HTTPContract{
			Request:  templateRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.UpdateTemplate)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/sms/templates/:id/status",
		Access: adminroute.Permission("system_sms_templateStatus"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "sms",
			Action:  "change_template_status",
			Title:   "修改短信模板状态",
		},
		Contract: &adminroute.HTTPContract{
			Request:  statusRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.ChangeTemplateStatus)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/sms/templates/:id",
		Access: adminroute.Permission("system_sms_templateDel"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "sms",
			Action:  "delete_template",
			Title:   "删除短信模板",
		},
		Contract: &adminroute.HTTPContract{
			Response: adminroute.EmptyData{},
		},
	}, handler.DeleteTemplate)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/sms/logs",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Query:    logListRequest{},
			Response: smsmodule.LogListResponse{},
		},
	}, handler.Logs)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/sms/logs/:id",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: smsmodule.LogDTO{},
		},
	}, handler.Log)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/sms/logs/:id",
		Access: adminroute.Permission("system_sms_logDel"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "sms",
			Action:  "delete_log",
			Title:   "删除短信日志",
		},
		Contract: &adminroute.HTTPContract{
			Response: adminroute.EmptyData{},
		},
	}, handler.DeleteLog)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/sms/logs",
		Access: adminroute.Permission("system_sms_logDel"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "sms",
			Action:  "delete_logs",
			Title:   "批量删除短信日志",
		},
		Contract: &adminroute.HTTPContract{
			Request:  deleteLogsRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.DeleteLogs)
}
