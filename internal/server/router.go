package server

import (
	"log/slog"
	"net/http"

	"admin_back_go/internal/config"
	"admin_back_go/internal/middleware"
	queuemonitoradmin "admin_back_go/internal/module/queuemonitor/transport/admin"
	realtimeadmin "admin_back_go/internal/module/realtime/transport/admin"
	"admin_back_go/internal/module/system"
	platformadmin "admin_back_go/internal/platform/admin"
	"admin_back_go/internal/platform/retired"
	"admin_back_go/internal/shared/enum"
	projecti18n "admin_back_go/internal/shared/i18n"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

type CoreDependencies struct {
	Readiness         system.ReadinessChecker
	Logger            *slog.Logger
	CORS              config.CORSConfig
	Authenticator     middleware.TokenAuthenticator
	PermissionChecker middleware.PermissionChecker
	PermissionRules   map[middleware.RouteKey]string
	OperationRecorder middleware.OperationRecorder
	OperationRules    map[middleware.RouteKey]middleware.OperationRule
	QueueMonitorUI    http.Handler
	RealtimeHandler   *realtimeadmin.Handler
	AuthSkipPaths     map[string]struct{}
}

type Dependencies struct {
	Core    CoreDependencies
	Admin   platformadmin.Graph
	Retired retired.Graph
}

func NewRouter(deps Dependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	validate.MustRegister()

	core := deps.Core
	router := gin.New()
	router.UseRawPath = true
	router.UnescapePathValues = false
	router.Use(gin.Recovery())
	router.Use(middleware.RequestID())
	router.Use(middleware.ErrorReporter(core.Logger))
	router.Use(middleware.AccessLog(core.Logger))
	router.Use(middleware.CORS(core.CORS))
	router.Use(projecti18n.Localize())
	router.Use(middleware.AuthToken(middleware.AuthTokenConfig{
		Authenticator: core.Authenticator,
		SkipPaths:     authSkipPaths(core.AuthSkipPaths),
		CookieTokenPath: middleware.CookieTokenPathConfig{
			PathPrefixes: []string{queuemonitoradmin.UIPath, realtimeadmin.WSPath},
			Platform:     enum.PlatformAdmin,
		},
	}))
	router.Use(middleware.PermissionCheck(middleware.PermissionCheckConfig{
		Checker: core.PermissionChecker,
		Rules:   core.PermissionRules,
	}))
	router.Use(middleware.OperationLog(middleware.OperationLogConfig{
		Recorder: core.OperationRecorder,
		Rules:    core.OperationRules,
		Logger:   core.Logger,
	}))

	registerAuthRoutes(router, deps)
	registerAdminFoundationRoutes(router, deps)
	registerAdminAIRoutes(router, deps)
	registerAdminUserRoutes(router, deps)
	registerAdminCommsRoutes(router, deps)
	registerAdminCommerceRBACRoutes(router, deps)
	registerCanvasRoutes(router, deps)

	return router
}

func authSkipPaths(paths map[string]struct{}) map[string]struct{} {
	if paths != nil {
		return paths
	}
	return middleware.DefaultAuthSkipPaths()
}
