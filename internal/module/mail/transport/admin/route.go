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
		Path:   "/api/admin/v1/mail/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/mail/config",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Config)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/mail/config",
		Access: adminroute.Permission("system_mail_configEdit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "mail",
			Action:  "update_config",
			Title:   "编辑邮件配置",
		},
	}, handler.SaveConfig)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/mail/config",
		Access: adminroute.Permission("system_mail_configDel"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "mail",
			Action:  "delete_config",
			Title:   "删除邮件配置",
		},
	}, handler.DeleteConfig)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/mail/test",
		Access: adminroute.Permission("system_mail_test"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "mail",
			Action:  "test_send",
			Title:   "发送测试邮件",
		},
	}, handler.TestSend)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/mail/templates",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Templates)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/mail/templates",
		Access: adminroute.Permission("system_mail_templateAdd"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "mail",
			Action:  "create_template",
			Title:   "新增邮件模板",
		},
	}, handler.CreateTemplate)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/mail/templates/:id",
		Access: adminroute.Permission("system_mail_templateEdit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "mail",
			Action:  "update_template",
			Title:   "编辑邮件模板",
		},
	}, handler.UpdateTemplate)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/mail/templates/:id/status",
		Access: adminroute.Permission("system_mail_templateStatus"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "mail",
			Action:  "change_template_status",
			Title:   "修改邮件模板状态",
		},
	}, handler.ChangeTemplateStatus)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/mail/templates/:id",
		Access: adminroute.Permission("system_mail_templateDel"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "mail",
			Action:  "delete_template",
			Title:   "删除邮件模板",
		},
	}, handler.DeleteTemplate)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/mail/logs",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Logs)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/mail/logs/:id",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Log)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/mail/logs/:id",
		Access: adminroute.Permission("system_mail_logDel"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "mail",
			Action:  "delete_log",
			Title:   "删除邮件日志",
		},
	}, handler.DeleteLog)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/mail/logs",
		Access: adminroute.Permission("system_mail_logDel"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "mail",
			Action:  "delete_logs",
			Title:   "批量删除邮件日志",
		},
	}, handler.DeleteLogs)
}
