package app

import (
	"fmt"
	"strings"

	"admin_back_go/internal/module/profile"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

type RouteOptions struct {
	UsersPrefix   string
	ProfilePrefix string
	Platform      string
	Service       profile.AppService
}

func RegisterRoutes(router *gin.Engine, service profile.AppService) {
	RegisterRoutesWithOptions(router, RouteOptions{
		UsersPrefix:   "/api/app/v1/users",
		ProfilePrefix: "/api/app/v1/profile",
		Platform:      enum.PlatformApp,
		Service:       service,
	})
}

func RegisterRoutesWithOptions(router *gin.Engine, options RouteOptions) {
	validate.MustRegister()
	platform := strings.TrimSpace(options.Platform)
	if platform == "" || !enum.IsPlatform(platform) {
		panic(fmt.Sprintf("invalid app profile route platform: %q", options.Platform))
	}
	handler := NewHandler(options.Service, platform)

	if usersPrefix := strings.TrimRight(strings.TrimSpace(options.UsersPrefix), "/"); usersPrefix != "" {
		users := router.Group(usersPrefix)
		users.GET("/me", handler.Me)
	}

	if profilePrefix := strings.TrimRight(strings.TrimSpace(options.ProfilePrefix), "/"); profilePrefix != "" {
		profileGroup := router.Group(profilePrefix)
		profileGroup.GET("", handler.Profile)
		profileGroup.PUT("", handler.UpdateProfile)
	}
}
