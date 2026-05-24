package appauth

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, authService AuthService, captchaService CaptchaService, userService UserService, uploadTokenService UploadTokenService) {
	validate.MustRegister()
	handler := NewHandler(authService, captchaService, userService, uploadTokenService)

	authGroup := router.Group("/api/app/v1/auth")
	authGroup.GET("/login-config", handler.LoginConfig)
	authGroup.GET("/captcha", handler.Captcha)
	authGroup.POST("/send-code", handler.SendCode)
	authGroup.POST("/login", handler.Login)
	authGroup.POST("/logout", handler.Logout)

	users := router.Group("/api/app/v1/users")
	users.GET("/me", handler.Me)

	profile := router.Group("/api/app/v1/profile")
	profile.GET("", handler.Profile)
	profile.PUT("", handler.UpdateProfile)

	uploadTokens := router.Group("/api/app/v1/upload-tokens")
	uploadTokens.POST("", handler.CreateUploadToken)
}
