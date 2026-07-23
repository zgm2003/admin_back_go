package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"admin_back_go/internal/config"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/auth"
	authadmin "admin_back_go/internal/module/auth/transport/admin"
	queuemonitoradmin "admin_back_go/internal/module/queuemonitor/transport/admin"
	realtimeadmin "admin_back_go/internal/module/realtime/transport/admin"
	"admin_back_go/internal/module/system"
	platformadmin "admin_back_go/internal/platform/admin"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	projecti18n "admin_back_go/internal/shared/i18n"
	"admin_back_go/internal/shared/validate"
	"admin_back_go/internal/telemetry"

	"github.com/gin-gonic/gin"
)

type CoreDependencies struct {
	Readiness         system.ReadinessChecker
	Logger            *slog.Logger
	Telemetry         telemetry.Recorder
	CORS              config.CORSConfig
	Authenticator     middleware.TokenAuthenticator
	PermissionChecker middleware.PermissionChecker
	OperationRecorder middleware.OperationRecorder
	RouteRegistry     *adminroute.Registry
	QueueMonitorUI    http.Handler
	RealtimeHandler   *realtimeadmin.Handler
}

type Dependencies struct {
	Core  CoreDependencies
	Admin platformadmin.Graph
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
	router.Use(middleware.AccessLog(core.Logger, core.Telemetry))
	router.Use(middleware.CORS(core.CORS))
	router.Use(projecti18n.Localize())
	router.Use(middleware.AuthToken(middleware.AuthTokenConfig{
		Authenticator: core.Authenticator,
		Platform:      enum.PlatformAdmin,
		SkipPaths:     core.RouteRegistry.PublicPaths(),
		BrowserGrants: middleware.BrowserGrantAuthConfig{
			RealtimePath:              realtimeadmin.WSPath,
			ConsumeRealtimeTicket:     realtimeGrantAuthenticator(deps.Admin.Identity.BrowserGrants),
			QueueMonitorPathPrefixes:  []string{queuemonitoradmin.UIPath},
			QueueMonitorCookieName:    authadmin.QueueMonitorGrantCookieName,
			ValidateQueueMonitorGrant: queueGrantAuthenticator(deps.Admin.Identity.BrowserGrants),
		},
	}))
	router.Use(middleware.PermissionCheck(middleware.PermissionCheckConfig{
		Checker: core.PermissionChecker,
		Rules:   core.RouteRegistry.PermissionRules(),
	}))
	router.Use(middleware.OperationLog(middleware.OperationLogConfig{
		Recorder:  core.OperationRecorder,
		Rules:     core.RouteRegistry.OperationRules(),
		Logger:    core.Logger,
		Telemetry: core.Telemetry,
	}))

	registerAuthRoutes(router, deps)
	registerAdminFoundationRoutes(router, deps)
	registerAdminAIRoutes(router, deps)
	registerAdminUserRoutes(router, deps)
	registerAdminCommsRoutes(router, deps)
	registerAdminCommerceRBACRoutes(router, deps)

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

func realtimeGrantAuthenticator(service *auth.BrowserGrantService) middleware.BrowserGrantAuthenticator {
	return func(ctx context.Context, credential string) (*middleware.AuthIdentity, *apperror.Error) {
		if service == nil {
			return nil, apperror.UnauthorizedKey("auth.browser_grant_authenticator_missing", nil, "浏览器授权服务未配置")
		}
		subject, appErr := service.ConsumeRealtimeTicket(ctx, credential)
		return grantIdentity(subject, appErr)
	}
}

func queueGrantAuthenticator(service *auth.BrowserGrantService) middleware.BrowserGrantAuthenticator {
	return func(ctx context.Context, credential string) (*middleware.AuthIdentity, *apperror.Error) {
		if service == nil {
			return nil, apperror.UnauthorizedKey("auth.browser_grant_authenticator_missing", nil, "浏览器授权服务未配置")
		}
		subject, appErr := service.ValidateQueueMonitorGrant(ctx, credential)
		return grantIdentity(subject, appErr)
	}
}

func grantIdentity(subject auth.GrantSubject, appErr *apperror.Error) (*middleware.AuthIdentity, *apperror.Error) {
	if appErr != nil {
		return nil, appErr
	}
	return &middleware.AuthIdentity{UserID: subject.UserID, SessionID: subject.SessionID, Platform: subject.Platform}, nil
}
