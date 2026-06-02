package canvas

import (
	authmodule "admin_back_go/internal/module/auth"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

const routePrefix = "/api/canvas/v1/auth"

type Dependencies struct {
	AuthService    authmodule.SessionService
	CaptchaService authmodule.CaptchaHTTPService
	UserService    UserInitService
}

func Register(router *gin.Engine, deps Dependencies) {
	validate.MustRegister()
	handler := NewHandler(deps)
	group := router.Group(routePrefix)
	group.GET("/login-config", handler.LoginConfig)
	group.GET("/captcha", handler.Captcha)
	group.POST("/send-code", handler.SendCode)
	group.POST("/login", handler.Login)
	group.POST("/refresh", handler.Refresh)
	group.POST("/logout", handler.Logout)
}
