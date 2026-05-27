package server

import (
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/module/userquickentry"

	"github.com/gin-gonic/gin"
)

func registerAdminUserRoutes(router *gin.Engine, deps Dependencies) {
	user.RegisterRoutes(router, deps.UserService)
	userquickentry.RegisterRoutes(router, deps.UserQuickEntryService)
}
