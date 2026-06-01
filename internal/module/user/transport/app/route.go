package app

import (
	usermodule "admin_back_go/internal/module/user"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service usermodule.InitService) {
	validate.MustRegister()
	handler := NewHandler(service)

	users := router.Group("/api/app/v1/users")
	users.GET("/me", handler.Me)
}
