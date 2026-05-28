package server

import (
	authadmin "admin_back_go/internal/module/auth/transport/admin"
	authapp "admin_back_go/internal/module/auth/transport/app"
	"admin_back_go/internal/shared/enum"

	"github.com/gin-gonic/gin"
)

func registerAuthRoutes(router *gin.Engine, deps Dependencies) {
	authadmin.Register(router, deps.AuthService, deps.CaptchaService, deps.SessionAdminService, deps.LoginLogService)
	authapp.Register(router, authapp.RouteOptions{
		Prefix:         "/api/app/v1/auth",
		Platform:       enum.PlatformApp,
		AuthService:    deps.AuthService,
		CaptchaService: deps.CaptchaService,
		UserService:    deps.UserService,
	})
}
