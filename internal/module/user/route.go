package user

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	legacy := router.Group("/api/Users")
	legacy.POST("/init", handler.Init)

	users := router.Group("/api/admin/v1/users")
	users.GET("/init", handler.Init)
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

	profile := router.Group("/api/admin/v1/profile")
	profile.GET("", handler.CurrentProfile)
	profile.PUT("", handler.UpdateCurrentProfile)
	profile.PUT("/security/password", handler.UpdatePassword)
	profile.PUT("/security/email", handler.UpdateEmail)
	profile.PUT("/security/phone", handler.UpdatePhone)
}
