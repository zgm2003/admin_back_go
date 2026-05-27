package appauth

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, userService UserService, uploadTokenService UploadTokenService) {
	validate.MustRegister()
	handler := NewHandler(userService, uploadTokenService)

	users := router.Group("/api/app/v1/users")
	users.GET("/me", handler.Me)

	profile := router.Group("/api/app/v1/profile")
	profile.GET("", handler.Profile)
	profile.PUT("", handler.UpdateProfile)

	uploadTokens := router.Group("/api/app/v1/upload-tokens")
	uploadTokens.POST("", handler.CreateUploadToken)
}
