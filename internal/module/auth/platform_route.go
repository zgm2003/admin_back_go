package auth

import (
	"strings"

	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

type PlatformRouteOptions struct {
	Prefix         string
	Platform       string
	AuthService    SessionService
	CaptchaService PlatformCaptchaService
	UserService    PlatformUserInitService
}

func RegisterPlatformRoutes(router *gin.Engine, opts PlatformRouteOptions) {
	validate.MustRegister()
	prefix := strings.TrimRight(strings.TrimSpace(opts.Prefix), "/")
	if prefix == "" {
		panic("auth platform route prefix is required")
	}
	if strings.TrimSpace(opts.Platform) == "" {
		panic("auth platform route platform is required")
	}
	handler := NewPlatformHandler(opts)
	group := router.Group(prefix)
	group.GET("/login-config", handler.LoginConfig)
	group.GET("/captcha", handler.Captcha)
	group.POST("/send-code", handler.SendCode)
	group.POST("/login", handler.Login)
	group.POST("/logout", handler.Logout)
}
