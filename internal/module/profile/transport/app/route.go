package app

import (
	"admin_back_go/internal/module/profile"
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service profile.AppService) {
	validate.MustRegister()
	handler := NewHandler(service)

	users := router.Group("/api/app/v1/users")
	users.GET("/me", handler.Me)

	profileGroup := router.Group("/api/app/v1/profile")
	profileGroup.GET("", handler.Profile)
	profileGroup.PUT("", handler.UpdateProfile)
}
