package server

import (
	"context"
	"log/slog"
	"net/http"

	"admin_back_go/internal/config"
	"admin_back_go/internal/middleware"
	aiagent "admin_back_go/internal/module/ai/agent"
	aichat "admin_back_go/internal/module/ai/chat"
	aiconversation "admin_back_go/internal/module/ai/conversation"
	aiknowledge "admin_back_go/internal/module/ai/knowledge"
	aimessage "admin_back_go/internal/module/ai/message"
	aipromptadmin "admin_back_go/internal/module/ai/prompt/transport/admin"
	aiprovider "admin_back_go/internal/module/ai/provider"
	airun "admin_back_go/internal/module/ai/run"
	aitool "admin_back_go/internal/module/ai/tool"
	"admin_back_go/internal/module/auth"
	authplatformadmin "admin_back_go/internal/module/auth_platform/transport/admin"
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
	"admin_back_go/internal/module/system"
	systemlogadmin "admin_back_go/internal/module/systemlog/transport/admin"
	systemsettingadmin "admin_back_go/internal/module/systemsetting/transport/admin"
	uploadconfigadmin "admin_back_go/internal/module/uploadconfig/transport/admin"
	uploadtokenadmin "admin_back_go/internal/module/uploadtoken/transport/admin"
	"admin_back_go/internal/module/user"
	platformadmin "admin_back_go/internal/platform/admin"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/telemetry"

	"github.com/gin-gonic/gin"
)

// testDependencies preserves concise test setup while production consumes the
// grouped graph exclusively.
type testDependencies struct {
	Readiness               system.ReadinessChecker
	Logger                  *slog.Logger
	Telemetry               telemetry.Recorder
	CORS                    config.CORSConfig
	Authenticator           middleware.TokenAuthenticator
	PermissionChecker       middleware.PermissionChecker
	PermissionRules         map[middleware.RouteKey]string
	OperationRecorder       middleware.OperationRecorder
	OperationRules          map[middleware.RouteKey]middleware.OperationRule
	AuthService             auth.SessionService
	CaptchaService          auth.CaptchaHTTPService
	AiChatService           aichat.HTTPService
	AiConversationService   aiconversation.HTTPService
	AiPromptAdminService    aipromptadmin.HTTPService
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
	BrowserGrants           *auth.BrowserGrantService
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
	AuthSkipPaths           map[string]struct{}
}

func (deps testDependencies) grouped() Dependencies {
	routes := adminroute.NewRegistry()
	permissionChecker := deps.PermissionChecker
	if permissionChecker == nil {
		permissionChecker = func(context.Context, middleware.PermissionInput) *apperror.Error { return nil }
	}
	return Dependencies{
		Core: CoreDependencies{
			Readiness:         deps.Readiness,
			Logger:            deps.Logger,
			Telemetry:         deps.Telemetry,
			CORS:              deps.CORS,
			Authenticator:     deps.Authenticator,
			PermissionChecker: permissionChecker,
			OperationRecorder: deps.OperationRecorder,
			RouteRegistry:     routes,
			QueueMonitorUI:    deps.QueueMonitorUI,
			RealtimeHandler:   deps.RealtimeHandler,
		},
		Admin: platformadmin.Graph{
			Identity: platformadmin.IdentityGraph{
				Auth:          deps.AuthService,
				Captcha:       deps.CaptchaService,
				Users:         deps.UserService,
				Permissions:   deps.PermissionService,
				Roles:         deps.RoleService,
				AuthPlatforms: deps.AuthPlatformService,
				Sessions:      deps.SessionAdminService,
				LoginLogs:     deps.LoginLogService,
				BrowserGrants: deps.BrowserGrants,
			},
			System: platformadmin.SystemGraph{
				CronTasks:     deps.CronTaskService,
				Exports:       deps.ExportTaskService,
				OperationLogs: deps.OperationLogService,
				QueueMonitor:  deps.QueueMonitorService,
				Settings:      deps.SystemSettingService,
				Logs:          deps.SystemLogService,
			},
			Communications: platformadmin.CommunicationsGraph{
				Notifications:     deps.NotificationService,
				NotificationTasks: deps.NotificationTaskService,
				Mail:              deps.MailService,
				SMS:               deps.SmsService,
				UploadConfig:      deps.UploadConfigService,
				UploadTokens:      deps.UploadTokenService,
			},
			Commerce: platformadmin.CommerceGraph{
				Payment: deps.PaymentService,
				Wallet:  deps.WalletService,
			},
			AI: platformadmin.AIGraph{
				Agents:        deps.AiAgentService,
				Chat:          deps.AiChatService,
				Conversations: deps.AiConversationService,
				Knowledge:     deps.AiKnowledgeService,
				Messages:      deps.AiMessageService,
				Prompts:       deps.AiPromptAdminService,
				Providers:     deps.AiProviderService,
				Runs:          deps.AiRunService,
				Tools:         deps.AiToolService,
			},
		},
	}
}

func newRouterFromTestDependencies(deps testDependencies) *gin.Engine {
	router, err := NewRouter(deps.grouped())
	if err != nil {
		panic(err)
	}
	return router
}
