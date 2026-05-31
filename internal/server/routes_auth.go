package server

import (
	authadmin "admin_back_go/internal/module/auth/transport/admin"
	authapp "admin_back_go/internal/module/auth/transport/app"
	authcanvas "admin_back_go/internal/module/auth/transport/canvas"

	"github.com/gin-gonic/gin"
)

func registerAuthRoutes(router *gin.Engine, deps Dependencies) {
	authadmin.Register(router, deps.AuthService, deps.CaptchaService, deps.SessionAdminService, deps.LoginLogService)
	authapp.Register(router, authapp.Dependencies{AuthService: deps.AuthService, CaptchaService: deps.CaptchaService, UserService: deps.UserService})
	authcanvas.Register(router, authcanvas.Dependencies{AuthService: deps.AuthService, CaptchaService: deps.CaptchaService, UserService: deps.UserService})
}
