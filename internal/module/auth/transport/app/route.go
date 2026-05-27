package app

import (
	"strings"

	authmodule "admin_back_go/internal/module/auth"
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

type RouteOptions struct {
	Prefix         string
	Platform       string
	AuthService    authmodule.SessionService
	CaptchaService authmodule.CaptchaHTTPService
	UserService    UserInitService
}

func Register(router *gin.Engine, opts RouteOptions) {
	validate.MustRegister()
	prefix := strings.TrimRight(strings.TrimSpace(opts.Prefix), "/")
	if prefix == "" {
		panic("auth app route prefix is required")
	}
	if strings.TrimSpace(opts.Platform) == "" {
		panic("auth app route platform is required")
	}
	handler := NewHandler(opts)
	group := router.Group(prefix)
	group.GET("/login-config", handler.LoginConfig)
	group.GET("/captcha", handler.Captcha)
	group.POST("/send-code", handler.SendCode)
	group.POST("/login", handler.Login)
	group.POST("/logout", handler.Logout)
}
