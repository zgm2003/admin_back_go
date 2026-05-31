package server

import (
	profileadmin "admin_back_go/internal/module/profile/transport/admin"
	profileapp "admin_back_go/internal/module/profile/transport/app"
	profilecanvas "admin_back_go/internal/module/profile/transport/canvas"
	useradmin "admin_back_go/internal/module/user/transport/admin"

	"github.com/gin-gonic/gin"
)

func registerAdminUserRoutes(router *gin.Engine, deps Dependencies) {
	useradmin.RegisterRoutes(router, deps.UserService)
	profileadmin.RegisterRoutes(router, deps.UserService, deps.UserQuickEntryService)
	profileapp.RegisterRoutes(router, deps.UserService)
	profilecanvas.RegisterRoutes(router, profilecanvas.Dependencies{Service: deps.UserService})
}
