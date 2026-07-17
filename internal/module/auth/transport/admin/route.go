package admin

import (
	"net/http"

	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service auth.SessionService, captchaService auth.CaptchaHTTPService, sessionAdminService auth.SessionAdminHTTPService, loginLogService auth.LoginLogHTTPService, options ...Option) {
	validate.MustRegister()
	handler := NewHandler(service, captchaService, sessionAdminService, loginLogService, options...)

	routes := adminroute.NewRegistrar(router, handler.routeRegistry)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/auth/login-config",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.LoginConfig)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/auth/captcha",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Captcha)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/auth/send-code",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("public authentication bootstrap"),
	}, handler.SendCode)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/auth/forgot-password",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("public account recovery state"),
	}, handler.ForgetPassword)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/auth/login",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("public authentication state"),
	}, handler.Login)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/auth/refresh",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("session rotation has domain audit"),
	}, handler.Refresh)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/auth/logout",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("self-service session state"),
	}, handler.Logout)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/auth/realtime-tickets",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("short-lived browser realtime credential issuance"),
	}, handler.RealtimeTicket)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/auth/queue-monitor-grants",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("short-lived queue monitor browser grant issuance"),
	}, handler.QueueMonitorGrant)

	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/user-sessions/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.SessionPageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/user-sessions/stats",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.SessionStats)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/user-sessions/revoke",
		Access: adminroute.Permission("user_userManager_kick"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "user_session",
			Action:  "revoke_batch",
			Title:   "批量踢下线用户会话",
		},
	}, handler.SessionBatchRevoke)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/user-sessions",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.SessionList)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/user-sessions/:id/revoke",
		Access: adminroute.Permission("user_userManager_kick"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "user_session",
			Action:  "revoke",
			Title:   "踢下线用户会话",
		},
	}, handler.SessionRevoke)

	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/users/login-logs/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.LoginLogPageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/users/login-logs",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.LoginLogList)
}
