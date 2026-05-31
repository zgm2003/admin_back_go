package server

import (
	profileadmin "admin_back_go/internal/module/profile/transport/admin"
	profileapp "admin_back_go/internal/module/profile/transport/app"
	useradmin "admin_back_go/internal/module/user/transport/admin"
	"admin_back_go/internal/shared/enum"

	"github.com/gin-gonic/gin"
)

func registerAdminUserRoutes(router *gin.Engine, deps Dependencies) {
	useradmin.RegisterRoutes(router, deps.UserService)
	profileadmin.RegisterRoutes(router, deps.UserService, deps.UserQuickEntryService)
	profileapp.RegisterRoutes(router, deps.UserService)
	profileapp.RegisterRoutesWithOptions(router, profileapp.RouteOptions{UsersPrefix: "/api/canvas/v1/users", Platform: enum.PlatformCanvas, Service: deps.UserService})
}
