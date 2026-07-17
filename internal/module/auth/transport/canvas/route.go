package canvas

import (
	"net/http"

	authmodule "admin_back_go/internal/module/auth"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	AuthService    authmodule.SessionService
	CaptchaService authmodule.CaptchaHTTPService
	UserService    UserInitService
}

func Register(router *gin.Engine, deps Dependencies, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(deps)
	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/canvas/v1/auth/login-config",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.LoginConfig)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/canvas/v1/auth/captcha",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Captcha)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/canvas/v1/auth/send-code",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("temporary retired Canvas route"),
	}, handler.SendCode)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/canvas/v1/auth/login",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("temporary retired Canvas route"),
	}, handler.Login)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/canvas/v1/auth/refresh",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("temporary retired Canvas route"),
	}, handler.Refresh)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/canvas/v1/auth/logout",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("temporary retired Canvas route"),
	}, handler.Logout)
}
