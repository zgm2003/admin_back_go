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
	useradmin.RegisterRoutes(router, deps.UserService)
	userapp.RegisterRoutes(router, deps.UserService)
	usercanvas.RegisterRoutes(router, deps.UserService)
	profileadmin.RegisterRoutes(router, deps.UserService)
	profileapp.RegisterRoutes(router, deps.UserService)
	profilecanvas.RegisterRoutes(router, deps.UserService)
}
