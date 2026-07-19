package admin

import (
	"net/http"

	"admin_back_go/internal/module/profile"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service profile.HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/profile",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: profile.ProfileResponse{},
		},
	}, handler.CurrentProfile)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/profile",
		Access: adminroute.Authenticated(),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "profile",
			Action:  "update_profile",
			Title:   "编辑个人资料",
		},
		Contract: &adminroute.HTTPContract{
			Request:  updateProfileRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.UpdateCurrentProfile)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/profile/security/password",
		Access: adminroute.Authenticated(),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "profile_security",
			Action:  "update_password",
			Title:   "修改登录密码",
		},
		Contract: &adminroute.HTTPContract{
			Request:  updatePasswordRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.UpdatePassword)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/profile/security/email",
		Access: adminroute.Authenticated(),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "profile_security",
			Action:  "update_email",
			Title:   "绑定或换绑邮箱",
		},
		Contract: &adminroute.HTTPContract{
			Request:  updateEmailRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.UpdateEmail)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/profile/security/phone",
		Access: adminroute.Authenticated(),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "profile_security",
			Action:  "update_phone",
			Title:   "绑定或换绑手机号",
		},
		Contract: &adminroute.HTTPContract{
			Request:  updatePhoneRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.UpdatePhone)
}
