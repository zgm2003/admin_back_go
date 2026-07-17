package server

import (
	"log/slog"
	"net/http"

	"admin_back_go/internal/config"
	"admin_back_go/internal/middleware"
	aiagent "admin_back_go/internal/module/ai/agent"
	aiassetcanvas "admin_back_go/internal/module/ai/asset/transport/canvas"
	aiaudio "admin_back_go/internal/module/ai/audio"
	aichat "admin_back_go/internal/module/ai/chat"
	aiconversation "admin_back_go/internal/module/ai/conversation"
	aiimage "admin_back_go/internal/module/ai/image"
	aiknowledge "admin_back_go/internal/module/ai/knowledge"
	aimessage "admin_back_go/internal/module/ai/message"
	aipromptadmin "admin_back_go/internal/module/ai/prompt/transport/admin"
	aipromptcanvas "admin_back_go/internal/module/ai/prompt/transport/canvas"
	aiprovider "admin_back_go/internal/module/ai/provider"
	airun "admin_back_go/internal/module/ai/run"
	aitool "admin_back_go/internal/module/ai/tool"
	aivideo "admin_back_go/internal/module/ai/video"
	"admin_back_go/internal/module/auth"
	authplatformadmin "admin_back_go/internal/module/auth_platform/transport/admin"
	canvastransport "admin_back_go/internal/module/canvas/transport/canvas"
	clientversion "admin_back_go/internal/module/clientversion"
	crontask "admin_back_go/internal/module/crontask"
	exporttask "admin_back_go/internal/module/export"
	mailadmin "admin_back_go/internal/module/mail/transport/admin"
	notification "admin_back_go/internal/module/notification"
	notificationtask "admin_back_go/internal/module/notification/task"
	operationlogadmin "admin_back_go/internal/module/operationlog/transport/admin"
	"admin_back_go/internal/module/payment"
	walletadmin "admin_back_go/internal/module/payment/wallet/transport/admin"
	permissionadmin "admin_back_go/internal/module/permission/transport/admin"
	queuemonitoradmin "admin_back_go/internal/module/queuemonitor/transport/admin"
	realtimeadmin "admin_back_go/internal/module/realtime/transport/admin"
	roleadmin "admin_back_go/internal/module/role/transport/admin"
	smsadmin "admin_back_go/internal/module/sms/transport/admin"
	system "admin_back_go/internal/module/system"
	systemlogadmin "admin_back_go/internal/module/systemlog/transport/admin"
	systemsettingadmin "admin_back_go/internal/module/systemsetting/transport/admin"
	uploadconfigadmin "admin_back_go/internal/module/uploadconfig/transport/admin"
	uploadtokenadmin "admin_back_go/internal/module/uploadtoken/transport/admin"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/shared/enum"
	projecti18n "admin_back_go/internal/shared/i18n"
	"admin_back_go/internal/shared/validate"

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
	CaptchaService          auth.CaptchaHTTPService
	ClientVersionService    clientversion.HTTPService
	AiChatService           aichat.HTTPService
	AiConversationService   aiconversation.HTTPService
	AiImageService          aiimage.HTTPService
	AiAudioService          aiaudio.HTTPService
	AiAssetService          aiassetcanvas.HTTPService
	AiPromptAdminService    aipromptadmin.HTTPService
	AiPromptService         aipromptcanvas.HTTPService
	AiVideoService          aivideo.HTTPService
	AiAgentService          aiagent.HTTPService
	AiProviderService       aiprovider.HTTPService
	AiKnowledgeService      aiknowledge.HTTPService
	AiMessageService        aimessage.HTTPService
	AiRunService            airun.HTTPService
	AiToolService           aitool.HTTPService
	CronTaskService         crontask.HTTPService
	ExportTaskService       exporttask.HTTPService
	UserService             user.HTTPService
	LoginLogService         auth.LoginLogHTTPService
	SessionAdminService     auth.SessionAdminHTTPService
	NotificationService     notification.HTTPService
	NotificationTaskService notificationtask.HTTPService
	OperationLogService     operationlogadmin.HTTPService
	MailService             mailadmin.HTTPService
	SmsService              smsadmin.HTTPService
	PaymentService          payment.HTTPService
	WalletService           walletadmin.HTTPService
	PermissionService       permissionadmin.ManagementService
	QueueMonitorService     queuemonitoradmin.HTTPService
	QueueMonitorUI          http.Handler
	SystemSettingService    systemsettingadmin.HTTPService
	SystemLogService        systemlogadmin.HTTPService
	UploadConfigService     uploadconfigadmin.HTTPService
	UploadTokenService      uploadtokenadmin.HTTPService
	RealtimeHandler         *realtimeadmin.Handler
	RoleService             roleadmin.HTTPService
	AuthPlatformService     authplatformadmin.HTTPService
	CanvasService           canvastransport.HTTPService
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
	router.Use(middleware.ErrorReporter(deps.Logger))
	router.Use(middleware.AccessLog(deps.Logger))
	router.Use(middleware.CORS(deps.CORS))
	router.Use(projecti18n.Localize())
	router.Use(middleware.AuthToken(middleware.AuthTokenConfig{
		Authenticator: deps.Authenticator,
		SkipPaths:     authSkipPaths(deps.AuthSkipPaths),
		CookieTokenPath: middleware.CookieTokenPathConfig{
			PathPrefixes: []string{queuemonitoradmin.UIPath, realtimeadmin.WSPath},
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
