package canvas

import (
	"admin_back_go/internal/module/profile"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service profile.AppService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/canvas/v1/profile")
	group.GET("", handler.Profile)
	group.PUT("", handler.UpdateProfile)
}
