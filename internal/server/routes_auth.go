package server

import (
	authadmin "admin_back_go/internal/module/auth/transport/admin"

	"github.com/gin-gonic/gin"
)

func registerAuthRoutes(router *gin.Engine, deps Dependencies) {
	identity := deps.Admin.Identity
	authadmin.Register(
		router,
		identity.Auth,
		identity.Captcha,
		identity.Sessions,
		identity.LoginLogs,
		authadmin.WithAllowedOrigins(deps.Core.CORS.AllowOrigins),
		authadmin.WithBrowserGrantIssuer(identity.BrowserGrants),
		authadmin.WithRouteRegistry(deps.Core.RouteRegistry),
	)
}
