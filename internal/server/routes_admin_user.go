package server

import (
	profileadmin "admin_back_go/internal/module/profile/transport/admin"
	profileapp "admin_back_go/internal/module/profile/transport/app"
	profilecanvas "admin_back_go/internal/module/profile/transport/canvas"
	useradmin "admin_back_go/internal/module/user/transport/admin"
	userapp "admin_back_go/internal/module/user/transport/app"
	usercanvas "admin_back_go/internal/module/user/transport/canvas"

	"github.com/gin-gonic/gin"
)

func registerAdminUserRoutes(router *gin.Engine, deps Dependencies) {
	users := deps.Admin.Identity.Users
	useradmin.RegisterRoutes(router, users, deps.Core.RouteRegistry)
	userapp.RegisterRoutes(router, users, deps.Core.RouteRegistry)
	usercanvas.RegisterRoutes(router, users, deps.Core.RouteRegistry)
	profileadmin.RegisterRoutes(router, users, deps.Core.RouteRegistry)
	profileapp.RegisterRoutes(router, users, deps.Core.RouteRegistry)
	profilecanvas.RegisterRoutes(router, users, deps.Core.RouteRegistry)
}
