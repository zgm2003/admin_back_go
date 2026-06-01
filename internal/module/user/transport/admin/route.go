package admin

import (
	usermodule "admin_back_go/internal/module/user"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service usermodule.HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	users := router.Group("/api/admin/v1/users")
	users.GET("/me", handler.Me)
	users.GET("/page-init", handler.PageInit)
	users.GET("/:id/profile", handler.UserProfile)
	users.GET("", handler.List)
	users.POST("/export", handler.Export)
	users.PUT("/:id", handler.Update)
	users.PATCH("/:id/status", handler.ChangeStatus)
	users.PATCH("", handler.BatchUpdateProfile)
	users.DELETE("/:id", handler.DeleteOne)
	users.DELETE("", handler.DeleteBatch)
}
