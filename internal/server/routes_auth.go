package server

import (
	authadmin "admin_back_go/internal/module/auth/transport/admin"
	authapp "admin_back_go/internal/module/auth/transport/app"
	authcanvas "admin_back_go/internal/module/auth/transport/canvas"

	"github.com/gin-gonic/gin"
)

func registerAuthRoutes(router *gin.Engine, deps Dependencies) {
	identity := deps.Admin.Identity
	authadmin.Register(router, identity.Auth, identity.Captcha, identity.Sessions, identity.LoginLogs)
	authapp.Register(router, authapp.Dependencies{AuthService: identity.Auth, CaptchaService: identity.Captcha, UserService: identity.Users})
	authcanvas.Register(router, authcanvas.Dependencies{AuthService: identity.Auth, CaptchaService: identity.Captcha, UserService: identity.Users})
}
