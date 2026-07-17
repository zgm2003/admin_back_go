package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/accesstoken"
	aiproviderinfra "admin_back_go/internal/infra/ai/provider"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/logstore"
	paymentcore "admin_back_go/internal/infra/payment"
	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/infra/redisclient"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/infra/secretkey"
	storagecos "admin_back_go/internal/infra/storage/cos"
	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/middleware"
	aiagent "admin_back_go/internal/module/ai/agent"
	aiasset "admin_back_go/internal/module/ai/asset"
	aiaudio "admin_back_go/internal/module/ai/audio"
	aichat "admin_back_go/internal/module/ai/chat"
	aiconversation "admin_back_go/internal/module/ai/conversation"
	aiimage "admin_back_go/internal/module/ai/image"
	aiknowledge "admin_back_go/internal/module/ai/knowledge"
	aimessage "admin_back_go/internal/module/ai/message"
	aiprompt "admin_back_go/internal/module/ai/prompt"
	aiprovider "admin_back_go/internal/module/ai/provider"
	airun "admin_back_go/internal/module/ai/run"
	aitext "admin_back_go/internal/module/ai/text"
	aitool "admin_back_go/internal/module/ai/tool"
	aivideo "admin_back_go/internal/module/ai/video"
	"admin_back_go/internal/module/auth"
	authplatform "admin_back_go/internal/module/auth_platform"
	canvasmodule "admin_back_go/internal/module/canvas"
	"admin_back_go/internal/module/clientversion"
	"admin_back_go/internal/module/crontask"
	exporttask "admin_back_go/internal/module/export"
	"admin_back_go/internal/module/mail"
	"admin_back_go/internal/module/notification"
	notificationtask "admin_back_go/internal/module/notification/task"
	"admin_back_go/internal/module/operationlog"
	paymentmodule "admin_back_go/internal/module/payment"
	walletmodule "admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/queuemonitor"
	"admin_back_go/internal/module/role"
	"admin_back_go/internal/module/sms"
	"admin_back_go/internal/module/systemlog"
	"admin_back_go/internal/module/systemsetting"
	"admin_back_go/internal/module/uploadconfig"
	"admin_back_go/internal/module/uploadtoken"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/platform/retired"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/telemetry"
)

type ReplyDispatcher interface {
	aimessage.ReplyEnqueuer
	aimessage.ReplyCanceler
	Shutdown(context.Context) error
}

type ReplyDispatcherFactory func(aichat.JobService) ReplyDispatcher

type BuildResources struct {
	DB         *database.Client
	Redis      *redisclient.Client
	TokenRedis *redisclient.Client
	QueueRedis *redisclient.Client
}

type ProviderSet struct {
	Secretbox secretbox.Box

	MailSender mail.Sender
	SMSSender  sms.Sender

	AIConnectionTester aiprovider.ProviderTester
	AIChatFactory      aichat.EngineFactory
	AIImageFactory     aiimage.ImageEngineFactory
	AIToolFactory      aitool.EngineFactory
	AIVideoFactory     aivideo.EngineFactory
	AIAudioFactory     aiaudio.EngineFactory

	ObjectReader     storagecos.ObjectReader
	ObjectWriter     storagecos.ObjectWriter
	CredentialSigner storagecos.CredentialSigner

	PaymentGateway      paymentcore.Gateway
	PaymentCertResolver paymentcore.CertPathResolver
	PaymentCertStore    paymentcore.LocalCertStore
}

type BuildInput struct {
	Config                 config.Config
	Resources              *BuildResources
	Keys                   *secretkey.KeyRing
	Providers              *ProviderSet
	Logger                 *slog.Logger
	Telemetry              telemetry.Recorder
	Queue                  taskqueue.Enqueuer
	QueueInspector         *taskqueue.Inspector
	RealtimePublisher      infrarealtime.Publisher
	ReplyDispatcherFactory ReplyDispatcherFactory
}

type BuildResult struct {
	Graph             Graph
	Retired           retired.Graph
	Authenticator     middleware.TokenAuthenticator
	PermissionChecker middleware.PermissionChecker
	OperationRecorder middleware.OperationRecorder
	ReplyDispatcher   ReplyDispatcher
}

func Build(input BuildInput) (*BuildResult, error) {
	if err := validateBuildInput(input); err != nil {
		return nil, err
	}
	cfg := input.Config
	cfg.AI = config.NormalizeAIConfig(cfg.AI)
	cfg.Token = config.NormalizeTokenConfig(cfg.Token)
	logger := input.Logger
	if logger == nil {
		logger = slog.Default()
	}
	recorder := input.Telemetry
	if recorder == nil {
		recorder = telemetry.Noop()
	}
	publisher := input.RealtimePublisher
	if publisher == nil {
		publisher = infrarealtime.NoopPublisher{}
	}
	providers := *input.Providers
	resources := input.Resources
	accessCodec, err := accessTokenCodecForKeys(input.Keys)
	if err != nil {
		return nil, fmt.Errorf("build access token codec: %w", err)
	}

	authPlatformService := authplatform.NewService(authplatform.NewGormRepository(resources.DB))
	sessionAuthenticator := auth.NewSessionLifecycle(auth.LifecycleDeps{
		Config:         cfg.Token,
		Cache:          auth.NewSessionRedisCache(resources.TokenRedis),
		Repository:     auth.NewSessionGormRepository(resources.DB),
		PolicyProvider: authPlatformService,
		AccessCodec:    accessCodec,
		TokenPepper:    input.Keys.TokenPepper(),
	})
	browserGrantService := auth.NewBrowserGrantService(
		auth.NewRedisBrowserGrantStore(resources.TokenRedis),
		auth.BrowserGrantConfig{RedisPrefix: cfg.Token.RedisPrefix},
	)

	systemLogService := systemlog.NewService(logstore.New(cfg.Logging.Dir, logstore.Options{
		AllowedExtensions: cfg.Logging.AllowedExtensions,
		MaxTailLines:      cfg.Logging.MaxTailLines,
	}))
	systemSettingRepository := systemsetting.NewGormRepository(resources.DB, resources.Redis)
	systemSettingService := systemsetting.NewService(systemSettingRepository)
	walletService := walletmodule.NewService(walletmodule.NewGormRepository(resources.DB))
	uploadConfigService := uploadconfig.NewService(uploadconfig.NewGormRepository(resources.DB), &providers.Secretbox)
	mailService := mail.NewService(mail.NewGormRepository(resources.DB), providers.Secretbox, providers.MailSender)
	smsService := sms.NewService(sms.NewGormRepository(resources.DB), providers.Secretbox, providers.SMSSender)
	clientVersionService := clientversion.NewService(
		clientversion.NewGormRepository(resources.DB),
		clientversion.NewManifestPublisher(
			clientversion.NewGormUploadConfigRepository(resources.DB),
			providers.Secretbox,
			providers.ObjectWriter,
		),
	)

	aiProviderService := aiprovider.NewServiceWithDriver(
		aiprovider.NewGormRepository(resources.DB),
		providers.Secretbox,
		providers.AIConnectionTester,
		aiproviderinfra.NewOpenAIDriver(nil, aiproviderinfra.WithTelemetry(recorder)),
	)
	aiAgentService := aiagent.NewService(aiagent.NewGormRepository(resources.DB), providers.Secretbox, providers.AIConnectionTester)
	aiRunRepository := airun.NewGormRepository(resources.DB)
	aiRunRecorder := airun.NewRecorder(aiRunRepository, nil)
	aiTextTasks := aitext.NewGormStore(resources.DB)
	aiImageService := aiimage.NewService(aiimage.Dependencies{
		Repository:    aiimage.NewGormRepository(resources.DB),
		Enqueuer:      input.Queue,
		Secretbox:     providers.Secretbox,
		EngineFactory: providers.AIImageFactory,
		ObjectReader:  providers.ObjectReader,
		ObjectWriter:  providers.ObjectWriter,
		RunRecorder:   aiRunRecorder,
	})
	aiToolRepository := aitool.NewGormRepository(resources.DB)
	aiToolService := aitool.NewService(
		aiToolRepository,
		aitool.DefaultExecutors(aiToolRepository),
		aitool.WithSecretbox(providers.Secretbox),
		aitool.WithEngineFactory(providers.AIToolFactory),
	)
	aiKnowledgeService := aiknowledge.NewService(aiknowledge.NewGormRepository(resources.DB))
	aiConversationService := aiconversation.NewService(aiconversation.NewGormRepository(resources.DB))
	aiRunService := airun.NewService(aiRunRepository)
	aiAssetService := aiasset.NewService(aiasset.NewGormRepository(resources.DB))
	aiPromptService := aiprompt.NewService(aiprompt.NewGormRepository(resources.DB))
	aiVideoService := aivideo.NewService(aivideo.Dependencies{
		Repository:    aivideo.NewGormRepository(resources.DB),
		Secretbox:     providers.Secretbox,
		EngineFactory: providers.AIVideoFactory,
		RunRecorder:   aiRunRecorder,
		ObjectWriter:  providers.ObjectWriter,
	})
	aiAudioService := aiaudio.NewService(aiaudio.Dependencies{
		Repository:    aiaudio.NewGormRepository(resources.DB),
		Secretbox:     providers.Secretbox,
		EngineFactory: providers.AIAudioFactory,
		RunRecorder:   aiRunRecorder,
	})

	canvasService := canvasmodule.NewServiceWithSettings(canvasmodule.NewGormRepository(resources.DB), canvasmodule.SettingsDependencies{
		AuthPolicy: authPlatformService,
	})
	paymentService := paymentmodule.NewService(paymentmodule.Dependencies{
		Repository:   paymentmodule.NewGormRepository(resources.DB),
		Gateway:      providers.PaymentGateway,
		Secretbox:    providers.Secretbox,
		CertResolver: providers.PaymentCertResolver,
		CertStore:    providers.PaymentCertStore,
	})
	uploadTokenService := uploadtoken.NewService(
		uploadtoken.NewGormRepository(resources.DB),
		providers.Secretbox,
		providers.CredentialSigner,
		uploadtoken.Options{TTLPolicy: uploadtoken.NewSystemSettingTTLPolicyProvider(systemSettingRepository)},
	)
	queueMonitorService := queuemonitor.NewService(
		queuemonitor.NewTaskqueueInspector(input.QueueInspector),
		queuemonitor.Options{QueueNames: []string{taskqueue.QueueCritical, taskqueue.QueueDefault, taskqueue.QueueLow}},
	)

	captchaEngine, err := auth.NewSlideEngine()
	if err != nil {
		return nil, fmt.Errorf("build admin captcha: %w", err)
	}
	captchaService := auth.NewCaptchaService(
		captchaEngine,
		auth.NewCaptchaRedisStore(resources.Redis, ""),
		auth.NewSystemSettingCaptchaPolicyProvider(systemSettingRepository),
	)
	authService := auth.NewService(
		auth.NewGormRepository(resources.DB),
		authPlatformService,
		sessionAuthenticator,
		captchaService,
		auth.WithCodeStore(auth.NewRedisCodeStore(resources.Redis)),
		auth.WithVerifyCodeMailSender(mailService),
		auth.WithVerifyCodePolicyProvider(auth.NewChannelVerifyCodePolicyProvider(mailService, smsService)),
		auth.WithLoginLogEnqueuer(input.Queue),
		auth.WithLogger(logger),
	)

	principalRepository := permission.NewGormPrincipalRepository(resources.DB)
	principalCache := permission.NewRedisPrincipalCache(resources.Redis, permission.PrincipalCacheConfig{RedisPrefix: cfg.Token.RedisPrefix})
	principalService := permission.NewPrincipalService(principalRepository, principalCache, permission.PrincipalServiceOptions{})
	if principalRepository != nil && principalCache != nil {
		reconcileCtx, cancelReconcile := context.WithTimeout(context.Background(), 10*time.Second)
		if err := principalService.Reconcile(reconcileCtx); err != nil {
			cancelReconcile()
			return nil, fmt.Errorf("reconcile admin principal cache: %w", err)
		}
		cancelReconcile()
	}
	permissionService := permission.NewService(
		permission.NewGormRepository(resources.DB),
		nil,
		permission.WithPrincipalMutations(principalService),
	)
	roleService := role.NewService(role.NewGormRepository(resources.DB), permissionService, nil, nil, role.WithPrincipalMutations(principalService))
	userRepository := user.NewGormRepository(resources.DB)
	addressCache := user.NewRedisAddressDictCache(resources.Redis)
	operationRepository := operationlog.NewGormRepository(resources.DB)
	operationService := operationlog.NewService(operationRepository)
	notificationService := notification.NewService(notification.NewGormRepository(resources.DB))

	aiChatService := aichat.NewService(aichat.Dependencies{
		Repository:       aichat.NewGormRepository(resources.DB),
		Publisher:        publisher,
		Secretbox:        providers.Secretbox,
		EngineFactory:    providers.AIChatFactory,
		ToolRuntime:      aiToolService,
		KnowledgeRuntime: knowledgeRuntimeAdapter{service: aiKnowledgeService},
		RunRecorder:      aiRunRecorder,
		TextTasks:        aiTextTasks,
		RunStaleTimeout:  cfg.AI.RunStaleTimeout,
	})
	replyDispatcher := input.ReplyDispatcherFactory(aiChatService)
	if isNilCapability(replyDispatcher) {
		return nil, errors.New("admin reply dispatcher is required")
	}
	aiMessageService := aimessage.NewService(
		aimessage.NewGormRepository(resources.DB),
		aimessage.WithReplyEnqueuer(replyDispatcher),
	)
	notificationTaskService := notificationtask.NewService(
		notificationtask.NewGormRepository(resources.DB),
		notificationtask.WithEnqueuer(input.Queue),
		notificationtask.WithRealtimePublisher(publisher),
		notificationtask.WithLogger(logger),
	)
	exportTaskService := exporttask.NewService(exporttask.NewGormRepository(resources.DB), exporttask.WithLogger(logger))
	cronTaskService := crontask.NewService(crontask.NewGormRepository(resources.DB), crontask.NewDefaultRegistry())
	userService := user.NewService(
		userRepository,
		permissionService,
		nil,
		0,
		user.WithPrincipalMutations(principalService),
		user.WithVerifyCodeStore(auth.NewRedisCodeStore(resources.Redis)),
		user.WithExportTaskCreator(exportTaskService),
		user.WithExportEnqueuer(input.Queue),
		user.WithAddressDictCache(addressCache),
	)
	sessionRevoker := auth.NewSessionRevocationService(
		auth.NewSessionRedisCache(resources.TokenRedis),
		auth.SessionRevocationConfig{RedisPrefix: cfg.Token.RedisPrefix},
	)
	loginLogService := auth.NewLoginLogService(auth.NewLoginLogGormRepository(resources.DB))
	sessionAdminService := auth.NewSessionAdminService(
		auth.NewSessionAdminGormRepository(resources.DB),
		auth.WithSessionAdminCacheRevoker(sessionRevoker),
	)

	graph := Graph{
		Identity: IdentityGraph{
			Auth:          authService,
			Captcha:       captchaService,
			Users:         userService,
			Permissions:   permissionService,
			Roles:         roleService,
			AuthPlatforms: authPlatformService,
			Sessions:      sessionAdminService,
			LoginLogs:     loginLogService,
			BrowserGrants: browserGrantService,
		},
		System: SystemGraph{
			ClientVersions: clientVersionService,
			CronTasks:      cronTaskService,
			Exports:        exportTaskService,
			OperationLogs:  operationService,
			QueueMonitor:   queueMonitorService,
			Settings:       systemSettingService,
			Logs:           systemLogService,
		},
		Communications: CommunicationsGraph{
			Notifications:     notificationService,
			NotificationTasks: notificationTaskService,
			Mail:              mailService,
			SMS:               smsService,
			UploadConfig:      uploadConfigService,
			UploadTokens:      uploadTokenService,
		},
		Commerce: CommerceGraph{Payment: paymentService, Wallet: walletService},
		AI: AIGraph{
			Agents:        aiAgentService,
			Chat:          aiChatService,
			Conversations: aiConversationService,
			Knowledge:     aiKnowledgeService,
			Messages:      aiMessageService,
			Prompts:       aiPromptService,
			Providers:     aiProviderService,
			Runs:          aiRunService,
			Tools:         aiToolService,
		},
	}
	if err := graph.Validate(); err != nil {
		return nil, err
	}

	var operationRecorder middleware.OperationRecorder
	if operationRepository != nil {
		operationRecorder = operationlog.NewRecorder(operationRepository)
	}
	return &BuildResult{
		Graph: graph,
		Retired: retired.Graph{
			Canvas:   canvasService,
			AIAssets: aiAssetService,
			AIAudio:  aiAudioService,
			AIChat:   aiChatService,
			AIImages: aiImageService,
			AIPrompt: aiPromptService,
			AIVideo:  aiVideoService,
		},
		Authenticator:     tokenAuthenticatorFor(sessionAuthenticator),
		PermissionChecker: permissionCheckerFor(principalService),
		OperationRecorder: operationRecorder,
		ReplyDispatcher:   replyDispatcher,
	}, nil
}

func accessTokenCodecForKeys(keys *secretkey.KeyRing) (accesstoken.Codec, error) {
	if keys == nil {
		return nil, errors.New("access token key ring is required")
	}
	return accesstoken.NewRotatingJWTCodec(
		keys.JWTSigningKeyID(),
		keys.JWTVerificationKeys(),
		accesstoken.Options{Issuer: "admin_go"},
	)
}

func validateBuildInput(input BuildInput) error {
	if input.Resources == nil {
		return errors.New("admin build resources are required")
	}
	if input.Resources.DB == nil || input.Resources.Redis == nil || input.Resources.TokenRedis == nil {
		return errors.New("admin build resources are incomplete")
	}
	if input.Config.Queue.Enabled && input.Resources.QueueRedis == nil {
		return errors.New("admin build queue resource is required")
	}
	if input.Config.Queue.Enabled && isNilCapability(input.Queue) {
		return errors.New("admin build queue adapter is required")
	}
	if input.Config.Realtime.Enabled && isNilCapability(input.RealtimePublisher) {
		return errors.New("admin build realtime publisher is required")
	}
	if input.Keys == nil {
		return errors.New("admin build key ring is required")
	}
	if input.Providers == nil {
		return errors.New("admin build provider set is required")
	}
	if input.ReplyDispatcherFactory == nil {
		return errors.New("admin build reply dispatcher factory is required")
	}
	return nil
}

type knowledgeRuntimeAdapter struct {
	service *aiknowledge.Service
}

func (adapter knowledgeRuntimeAdapter) RetrieveForRun(ctx context.Context, input aichat.KnowledgeRuntimeInput) (*aichat.KnowledgeContextResult, *apperror.Error) {
	if adapter.service == nil {
		return nil, apperror.Internal("AI知识库服务未配置")
	}
	result, appErr := adapter.service.RetrieveForRun(ctx, aiknowledge.KnowledgeRuntimeInput{
		RunID:          input.RunID,
		AgentID:        input.AgentID,
		ConversationID: input.ConversationID,
		UserMessageID:  input.UserMessageID,
		Query:          input.Query,
		StartedAt:      input.StartedAt,
	})
	if appErr != nil || result == nil {
		return nil, appErr
	}
	return &aichat.KnowledgeContextResult{RetrievalID: result.RetrievalID, Status: result.Status, Context: result.Context}, nil
}

func tokenAuthenticatorFor(authenticator *auth.SessionLifecycle) middleware.TokenAuthenticator {
	return func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
		identity, appErr := authenticator.Authenticate(ctx, auth.AccessCredential{
			AccessToken: input.AccessToken,
			Platform:    input.Platform,
			DeviceID:    input.DeviceID,
			ClientIP:    input.ClientIP,
		})
		if appErr != nil || identity == nil {
			return nil, appErr
		}
		return &middleware.AuthIdentity{UserID: identity.UserID, SessionID: identity.SessionID, Platform: identity.Platform}, nil
	}
}

func permissionCheckerFor(service *permission.PrincipalService) middleware.PermissionChecker {
	return func(ctx context.Context, input middleware.PermissionInput) *apperror.Error {
		if service == nil {
			return apperror.InternalKey("permission.principal_service_missing", nil, "权限主体服务未配置")
		}
		return service.Authorize(ctx, input.UserID, input.Platform, input.Code)
	}
}
