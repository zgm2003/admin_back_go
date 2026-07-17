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
	useradmin.RegisterRoutes(router, users)
	userapp.RegisterRoutes(router, users)
	usercanvas.RegisterRoutes(router, users)
	profileadmin.RegisterRoutes(router, users)
	profileapp.RegisterRoutes(router, users)
	profilecanvas.RegisterRoutes(router, users)
}
