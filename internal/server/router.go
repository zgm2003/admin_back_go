package server

import (
	"log/slog"

	"admin_back_go/internal/config"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/system"
	"admin_back_go/internal/module/user"

	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	Readiness     system.ReadinessChecker
	Logger        *slog.Logger
	CORS          config.CORSConfig
	Authenticator middleware.TokenAuthenticator
	AuthService   auth.SessionService
	UserService   user.InitService
	AuthSkipPaths map[string]struct{}
}

func NewRouter(deps Dependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.RequestID())
	router.Use(middleware.AccessLog(deps.Logger))
	router.Use(middleware.CORS(deps.CORS))
	router.Use(middleware.AuthToken(middleware.AuthTokenConfig{
		Authenticator: deps.Authenticator,
		SkipPaths:     authSkipPaths(deps.AuthSkipPaths),
	}))

	system.RegisterRoutes(router, deps.Readiness)
	auth.RegisterRoutes(router, deps.AuthService)
	user.RegisterRoutes(router, deps.UserService)

	return router
}

func authSkipPaths(paths map[string]struct{}) map[string]struct{} {
	if paths != nil {
		return paths
	}
	return middleware.DefaultAuthSkipPaths()
}
