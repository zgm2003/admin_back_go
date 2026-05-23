package appauth

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, authService AuthService, userService UserService) {
	validate.MustRegister()
	handler := NewHandler(authService, userService)

	authGroup := router.Group("/api/app/v1/auth")
	authGroup.POST("/login", handler.Login)
	authGroup.POST("/logout", handler.Logout)

	users := router.Group("/api/app/v1/users")
	users.GET("/me", handler.Me)
}
