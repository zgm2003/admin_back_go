package admin

import (
	"admin_back_go/internal/module/profile"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service profile.HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	group := router.Group("/api/admin/v1/profile")
	group.GET("", handler.CurrentProfile)
	group.PUT("", handler.UpdateCurrentProfile)
	group.PUT("/security/password", handler.UpdatePassword)
	group.PUT("/security/email", handler.UpdateEmail)
	group.PUT("/security/phone", handler.UpdatePhone)
}
