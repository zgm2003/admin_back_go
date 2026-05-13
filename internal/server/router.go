package server

import (
	"log/slog"
	"net/http"

	"admin_back_go/internal/config"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/aiagent"
	"admin_back_go/internal/module/aichat"
	"admin_back_go/internal/module/aiconversation"
	"admin_back_go/internal/module/aiknowledge"
	"admin_back_go/internal/module/aimessage"
	"admin_back_go/internal/module/aiprovider"
	"admin_back_go/internal/module/airun"
	"admin_back_go/internal/module/aitool"
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/authplatform"
	"admin_back_go/internal/module/captcha"
	"admin_back_go/internal/module/clientversion"
	"admin_back_go/internal/module/crontask"
	"admin_back_go/internal/module/exporttask"
	"admin_back_go/internal/module/mail"
	"admin_back_go/internal/module/notification"
	"admin_back_go/internal/module/notificationtask"
	"admin_back_go/internal/module/operationlog"
	"admin_back_go/internal/module/payment"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/queuemonitor"
	"admin_back_go/internal/module/realtime"
	"admin_back_go/internal/module/role"
	"admin_back_go/internal/module/system"
	"admin_back_go/internal/module/systemlog"
	"admin_back_go/internal/module/systemsetting"
	"admin_back_go/internal/module/uploadconfig"
	"admin_back_go/internal/module/uploadtoken"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/module/userloginlog"
	"admin_back_go/internal/module/userquickentry"
	"admin_back_go/internal/module/usersession"
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	Readiness               system.ReadinessChecker
	Logger                  *slog.Logger
	CORS                    config.CORSConfig
	Authenticator           middleware.TokenAuthenticator
	PermissionChecker       middleware.PermissionChecker
	PermissionRules         map[middleware.RouteKey]string
	OperationRecorder       middleware.OperationRecorder
	OperationRules          map[middleware.RouteKey]middleware.OperationRule
	AuthService             auth.SessionService
	CaptchaService          captcha.HTTPService
	ClientVersionService    clientversion.HTTPService
	AiChatService           aichat.HTTPService
	AiConversationService   aiconversation.HTTPService
	AiAgentService          aiagent.HTTPService
	AiProviderService       aiprovider.HTTPService
	AiKnowledgeService      aiknowledge.HTTPService
	AiMessageService        aimessage.HTTPService
	AiRunService            airun.HTTPService
	AiToolService           aitool.HTTPService
	CronTaskService         crontask.HTTPService
	ExportTaskService       exporttask.HTTPService
	UserService             user.HTTPService
	UserQuickEntryService   userquickentry.HTTPService
	UserLoginLogService     userloginlog.HTTPService
	UserSessionService      usersession.HTTPService
	NotificationService     notification.HTTPService
	NotificationTaskService notificationtask.HTTPService
	OperationLogService     operationlog.HTTPService
	MailService             mail.HTTPService
	PaymentService          payment.HTTPService
	PermissionService       permission.ManagementService
	QueueMonitorService     queuemonitor.HTTPService
	QueueMonitorUI          http.Handler
	SystemSettingService    systemsetting.HTTPService
	SystemLogService        systemlog.HTTPService
	UploadConfigService     uploadconfig.HTTPService
	UploadTokenService      uploadtoken.HTTPService
	RealtimeHandler         *realtime.Handler
	RoleService             role.HTTPService
	AuthPlatformService     authplatform.HTTPService
	AuthSkipPaths           map[string]struct{}
}

func NewRouter(deps Dependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	validate.MustRegister()

	router := gin.New()
	router.UseRawPath = true
	router.UnescapePathValues = false
	router.Use(gin.Recovery())
	router.Use(middleware.RequestID())
	router.Use(middleware.AccessLog(deps.Logger))
	router.Use(middleware.CORS(deps.CORS))
	router.Use(middleware.AuthToken(middleware.AuthTokenConfig{
		Authenticator: deps.Authenticator,
		SkipPaths:     authSkipPaths(deps.AuthSkipPaths),
		CookieTokenPath: middleware.CookieTokenPathConfig{
			PathPrefixes: []string{queuemonitor.UIPath, realtime.WSPath},
			Platform:     enum.PlatformAdmin,
		},
	}))
	router.Use(middleware.PermissionCheck(middleware.PermissionCheckConfig{
		Checker: deps.PermissionChecker,
		Rules:   deps.PermissionRules,
	}))
	router.Use(middleware.OperationLog(middleware.OperationLogConfig{
		Recorder: deps.OperationRecorder,
		Rules:    deps.OperationRules,
		Logger:   deps.Logger,
	}))

	system.RegisterRoutes(router, deps.Readiness)
	captcha.RegisterRoutes(router, deps.CaptchaService)
	auth.RegisterRoutes(router, deps.AuthService)
	clientversion.RegisterRoutes(router, deps.ClientVersionService)
	aiprovider.RegisterRoutes(router, deps.AiProviderService)
	aiagent.RegisterRoutes(router, deps.AiAgentService)
	aiknowledge.RegisterRoutes(router, deps.AiKnowledgeService)
	aiconversation.RegisterRoutes(router, deps.AiConversationService)
	aimessage.RegisterRoutes(router, deps.AiMessageService)
	airun.RegisterRoutes(router, deps.AiRunService)
	aichat.RegisterRoutes(router, deps.AiChatService)
	aitool.RegisterRoutes(router, deps.AiToolService)
	user.RegisterRoutes(router, deps.UserService)
	userquickentry.RegisterRoutes(router, deps.UserQuickEntryService)
	userloginlog.RegisterRoutes(router, deps.UserLoginLogService)
	usersession.RegisterRoutes(router, deps.UserSessionService)
	exporttask.RegisterRoutes(router, deps.ExportTaskService)
	notification.RegisterRoutes(router, deps.NotificationService)
	notificationtask.RegisterRoutes(router, deps.NotificationTaskService)
	crontask.RegisterRoutes(router, deps.CronTaskService)
	operationlog.RegisterRoutes(router, deps.OperationLogService)
	mail.RegisterRoutes(router, deps.MailService)
	payment.RegisterRoutes(router, deps.PaymentService)

	permission.RegisterRoutes(router, deps.PermissionService)
	queuemonitor.RegisterRoutes(router, deps.QueueMonitorService, deps.QueueMonitorUI)
	systemsetting.RegisterRoutes(router, deps.SystemSettingService)
	systemlog.RegisterRoutes(router, deps.SystemLogService)
	uploadconfig.RegisterRoutes(router, deps.UploadConfigService)
	uploadtoken.RegisterRoutes(router, deps.UploadTokenService)
	realtime.RegisterRoutes(router, deps.RealtimeHandler)
	role.RegisterRoutes(router, deps.RoleService)
	authplatform.RegisterRoutes(router, deps.AuthPlatformService)

	return router
}

func authSkipPaths(paths map[string]struct{}) map[string]struct{} {
	if paths != nil {
		return paths
	}
	return middleware.DefaultAuthSkipPaths()
}
