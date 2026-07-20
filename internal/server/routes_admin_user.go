package server

import (
	profileadmin "admin_back_go/internal/module/profile/transport/admin"
	useradmin "admin_back_go/internal/module/user/transport/admin"

	"github.com/gin-gonic/gin"
)

func registerAdminUserRoutes(router *gin.Engine, deps Dependencies) {
	users := deps.Admin.Identity.Users
	useradmin.RegisterRoutes(router, users, deps.Core.RouteRegistry)
	profileadmin.RegisterRoutes(router, users, deps.Core.RouteRegistry)
}
