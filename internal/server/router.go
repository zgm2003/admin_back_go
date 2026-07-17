package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"admin_back_go/internal/config"
	"admin_back_go/internal/middleware"
	queuemonitoradmin "admin_back_go/internal/module/queuemonitor/transport/admin"
	realtimeadmin "admin_back_go/internal/module/realtime/transport/admin"
	"admin_back_go/internal/module/system"
	platformadmin "admin_back_go/internal/platform/admin"
	"admin_back_go/internal/platform/retired"
	"admin_back_go/internal/server/adminroute"
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
	OperationRecorder middleware.OperationRecorder
	RouteRegistry     *adminroute.Registry
	QueueMonitorUI    http.Handler
	RealtimeHandler   *realtimeadmin.Handler
}

type Dependencies struct {
	Core    CoreDependencies
	Admin   platformadmin.Graph
	Retired retired.Graph
}

func NewRouter(deps Dependencies) (*gin.Engine, error) {
	gin.SetMode(gin.ReleaseMode)
	validate.MustRegister()

	core := deps.Core
	if core.RouteRegistry == nil {
		return nil, errors.New("admin route registry is required")
	}
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
		SkipPaths:     core.RouteRegistry.PublicPaths(),
		CookieTokenPath: middleware.CookieTokenPathConfig{
			PathPrefixes: []string{queuemonitoradmin.UIPath, realtimeadmin.WSPath},
			Platform:     enum.PlatformAdmin,
		},
	}))
	router.Use(middleware.PermissionCheck(middleware.PermissionCheckConfig{
		Checker: core.PermissionChecker,
		Rules:   core.RouteRegistry.PermissionRules(),
	}))
	router.Use(middleware.OperationLog(middleware.OperationLogConfig{
		Recorder: core.OperationRecorder,
		Rules:    core.RouteRegistry.OperationRules(),
		Logger:   core.Logger,
	}))

	registerAuthRoutes(router, deps)
	registerAdminFoundationRoutes(router, deps)
	registerAdminAIRoutes(router, deps)
	registerAdminUserRoutes(router, deps)
	registerAdminCommsRoutes(router, deps)
	registerAdminCommerceRBACRoutes(router, deps)
	registerCanvasRoutes(router, deps)

	routes := router.Routes()
	actual := make([]adminroute.Route, 0, len(routes))
	for _, route := range routes {
		actual = append(actual, adminroute.Route{Method: route.Method, Path: route.Path})
	}
	if err := core.RouteRegistry.CompileRoutes(actual); err != nil {
		return nil, fmt.Errorf("compile admin route registry: %w", err)
	}
	return router, nil
}
