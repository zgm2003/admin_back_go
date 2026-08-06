package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/accesstoken"
	infraai "admin_back_go/internal/infra/ai"
	aiproviderinfra "admin_back_go/internal/infra/ai/provider"
	"admin_back_go/internal/infra/contextindex"
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
	aichat "admin_back_go/internal/module/ai/chat"
	contextengine "admin_back_go/internal/module/ai/contextengine"
	aiconversation "admin_back_go/internal/module/ai/conversation"
	aiimage "admin_back_go/internal/module/ai/image"
	aimessage "admin_back_go/internal/module/ai/message"
	"admin_back_go/internal/module/ai/officialmodel"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/module/ai/replycommand"
	airun "admin_back_go/internal/module/ai/run"
	aitext "admin_back_go/internal/module/ai/text"
	aitool "admin_back_go/internal/module/ai/tool"
	"admin_back_go/internal/module/auth"
	authplatform "admin_back_go/internal/module/auth_platform"
	"admin_back_go/internal/module/crontask"
	exporttask "admin_back_go/internal/module/export"
	"admin_back_go/internal/module/mail"
	"admin_back_go/internal/module/notification"
	notificationtask "admin_back_go/internal/module/notification/task"
	"admin_back_go/internal/module/operationlog"
	paymentmodule "admin_back_go/internal/module/payment"
	redeemcode "admin_back_go/internal/module/payment/redeemcode"
	walletmodule "admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/queuemonitor"
	modulerealtime "admin_back_go/internal/module/realtime"
	"admin_back_go/internal/module/role"
	"admin_back_go/internal/module/sms"
	"admin_back_go/internal/module/systemlog"
	"admin_back_go/internal/module/systemsetting"
	"admin_back_go/internal/module/uploadconfig"
	"admin_back_go/internal/module/uploadtoken"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/clock"
	"admin_back_go/internal/telemetry"
)

type BuildResources struct {
	DB           *database.Client
	Redis        *redisclient.Client
	TokenRedis   *redisclient.Client
	QueueRedis   *redisclient.Client
	ContextIndex contextindex.Querier
}

type ProviderSet struct {
	Secretbox         secretbox.Box
	MailDiagnosticBox secretbox.VersionedBox

	MailSender mail.Sender
	SMSSender  sms.Sender

	AIConnectionTester      aiprovider.ProviderTester
	AIChatFactory           aichat.EngineFactory
	AIEmbeddingFactory      infraai.EmbeddingFactory
	AIRerankFactory         infraai.RerankFactory
	AIImageFactory          aiimage.ImageEngineFactory
	AITransportCapabilities infraai.TransportCapabilityResolver

	ObjectReader     storagecos.ObjectReader
	ObjectWriter     storagecos.ObjectWriter
	CredentialSigner storagecos.CredentialSigner

	PaymentGateway      paymentcore.Gateway
	PaymentCertResolver paymentcore.CertPathResolver
	PaymentCertStore    paymentcore.LocalCertStore
}

type BuildInput struct {
	Config            config.Config
	Resources         *BuildResources
	Keys              *secretkey.KeyRing
	Providers         *ProviderSet
	Logger            *slog.Logger
	Telemetry         telemetry.Recorder
	Queue             taskqueue.Enqueuer
	QueueInspector    *taskqueue.Inspector
	RealtimePublisher infrarealtime.Publisher
}

type BuildResult struct {
	Graph             Graph
	Authenticator     middleware.TokenAuthenticator
	PermissionChecker middleware.PermissionChecker
	OperationRecorder middleware.OperationRecorder
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
	sharedClock := clock.SystemClock{}
	realtimeEventRepository := modulerealtime.NewGormRepository(resources.DB, modulerealtime.DefaultRegistry())
	realtimeEventSink := modulerealtime.NewDurableEventSink(realtimeEventRepository, publisher, logger)
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
	walletRepository := walletmodule.NewGormRepository(resources.DB)
	walletService := walletmodule.NewService(walletRepository)
	redeemCodeService := redeemcode.NewService(
		redeemcode.NewGormRepository(resources.DB, walletRepository, sharedClock),
		redeemcode.WithAttemptLimiter(redeemcode.NewRedisAttemptLimiter(resources.Redis.Redis)),
		redeemcode.WithClock(sharedClock),
		redeemcode.WithTelemetry(recorder),
		redeemcode.WithLogger(logger),
	)
	uploadConfigService := uploadconfig.NewService(uploadconfig.NewGormRepository(resources.DB), &providers.Secretbox)
	mailService := mail.NewServiceWithDependencies(mail.ServiceDependencies{
		Repository:    mail.NewGormRepository(resources.DB),
		CredentialBox: providers.Secretbox,
		DiagnosticBox: providers.MailDiagnosticBox,
		Sender:        providers.MailSender,
		Clock:         sharedClock,
	})
	smsService := sms.NewService(sms.NewGormRepository(resources.DB), providers.Secretbox, providers.SMSSender)
	aiOfficialModelResolver := officialmodel.NewService(officialmodel.NewGormRepository(resources.DB))
	aiProviderRepository := aiprovider.NewGormRepository(resources.DB)
	aiProviderService := aiprovider.NewServiceWithDriver(
		aiProviderRepository,
		providers.Secretbox,
		providers.AIConnectionTester,
		aiproviderinfra.NewOpenAIDriver(nil, aiproviderinfra.WithTelemetry(recorder)),
		aiprovider.WithOfficialModelMatcher(aiOfficialModelResolver),
	)
	if aiProviderRepository != nil {
		reconcileCtx, cancelReconcile := context.WithTimeout(context.Background(), 10*time.Second)
		if err := aiProviderService.ReconcileOfficialModelMappings(reconcileCtx); err != nil {
			cancelReconcile()
			return nil, fmt.Errorf("reconcile AI provider model mappings: %w", err)
		}
		cancelReconcile()
	}
	uploadTokenRepository := uploadtoken.NewGormRepository(resources.DB)
	uploadRuleResolver := uploadtoken.NewActiveRuleResolver(uploadTokenRepository)
	aiChatObjectConfig := uploadtoken.NewObjectConfigProvider(uploadTokenRepository, providers.Secretbox)
	contextDependencies := contextengine.RuntimeDependencies{
		Database: resources.DB, OfficialModels: aiOfficialModelResolver,
		EmbeddingFactory: providers.AIEmbeddingFactory, RerankFactory: providers.AIRerankFactory,
		Secretbox: providers.Secretbox, Index: resources.ContextIndex, CollectionPrefix: cfg.Qdrant.CollectionPrefix, Platform: "admin", Telemetry: recorder,
	}
	contextEvaluation := contextengine.NewEvaluationService(contextengine.BuildEvaluationPipeline(contextDependencies))
	if contextEvaluation == nil {
		return nil, errors.New("build admin context evaluation")
	}
	contextService := contextengine.NewAdminService(
		contextengine.NewAdminRepository(resources.DB),
		storagecos.NewConditionalObjectReader(aiChatObjectConfig, storagecos.ObjectStreamerConfig{Enabled: true}),
		contextengine.NewDocumentVersionEnqueuer(input.Queue, contextengine.NewIngestionRepository(resources.DB)),
		contextengine.WithOfficialModelResolver(aiOfficialModelResolver),
		contextengine.WithProfileRebuildEnqueuer(contextengine.NewProfileRebuildEnqueuer(input.Queue)),
		contextengine.WithEvaluationRunner(contextEvaluation),
	)
	aiAgentService := aiagent.NewService(
		aiagent.NewGormRepository(resources.DB),
		providers.Secretbox,
		providers.AIConnectionTester,
		aiagent.WithPricingResolver(aiOfficialModelResolver),
		aiagent.WithTransportCapabilityResolver(providers.AITransportCapabilities),
		aiagent.WithUploadRuleResolver(uploadRuleResolver),
		aiagent.WithContextProfileResolver(contextService),
	)
	aiRunRepository := airun.NewGormRepository(resources.DB)
	aiRunRecorder := airun.NewRecorder(aiRunRepository, nil)
	aiTextTasks := aitext.NewGormStore(resources.DB)
	aiTextService := aitext.NewService(aitext.ServiceDependencies{
		Store: aiTextTasks,
		Waker: aitext.NewWakeupEnqueuer(input.Queue),
	})
	aiToolRepository := aitool.NewGormRepository(resources.DB)
	aiToolService := aitool.NewService(
		aiToolRepository,
		aitool.DefaultExecutors(aiToolRepository),
		aitool.WithDraftTaskService(aiTextService),
		aitool.WithPricingResolver(aiOfficialModelResolver),
	)
	aiConversationService := aiconversation.NewService(
		aiconversation.NewGormRepository(resources.DB),
		aiconversation.WithCancelPublisher(replycommand.NewRedisCancelPublisher(resources.Redis)),
	)
	aiRunService := airun.NewService(
		aiRunRepository,
		airun.WithLogger(logger),
		airun.WithInputAttachmentPreviewer(storagecos.NewImagePreviewer(
			aiChatObjectConfig,
			storagecos.ImagePreviewerConfig{Enabled: true},
		)),
	)
	paymentService := paymentmodule.NewService(paymentmodule.Dependencies{
		Repository:   paymentmodule.NewGormRepository(resources.DB, walletRepository),
		Gateway:      providers.PaymentGateway,
		Secretbox:    providers.Secretbox,
		CertResolver: providers.PaymentCertResolver,
		CertStore:    providers.PaymentCertStore,
	})
	uploadTokenService := uploadtoken.NewService(
		uploadTokenRepository,
		providers.Secretbox,
		providers.CredentialSigner,
		uploadtoken.Options{TTLPolicy: uploadtoken.NewSystemSettingTTLPolicyProvider(systemSettingRepository)},
	)
	aiChatObjectInspector := storagecos.NewObjectInspector(
		aiChatObjectConfig,
		storagecos.ObjectInspectorConfig{Enabled: true},
	)
	aiChatObjectStreamer := storagecos.NewObjectStreamer(
		aiChatObjectConfig,
		storagecos.ObjectStreamerConfig{Enabled: true},
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
		auth.WithVerifyCodePhoneSender(smsService),
		auth.WithVerifyCodeReadinessProvider(auth.NewChannelVerifyCodeReadinessProvider(mailService, smsService)),
		auth.WithVerifyCodePolicyProvider(auth.NewChannelVerifyCodePolicyProvider(mailService, smsService)),
		auth.WithLoginLogEnqueuer(input.Queue),
		auth.WithLogger(logger),
		auth.WithClock(sharedClock),
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
	aiReplyRepository := replycommand.NewGormRepository(
		resources.DB,
		replycommand.WithDurableEventSink(realtimeEventSink),
	)
	contextRuntime := contextengine.BuildRuntime(contextDependencies)
	if contextRuntime == nil {
		return nil, errors.New("build admin context runtime")
	}
	conversationDocuments := contextengine.NewConversationDocumentService(
		resources.DB,
		contextengine.NewDocumentVersionEnqueuer(input.Queue, contextengine.NewIngestionRepository(resources.DB)),
	)
	if conversationDocuments == nil {
		return nil, errors.New("build admin conversation document service")
	}
	historyInvalidator := contextengine.NewHistoryInvalidationService(
		resources.DB,
		contextengine.NewIndexCleanupEnqueuer(input.Queue),
	)
	if historyInvalidator == nil {
		return nil, errors.New("build admin history invalidation service")
	}

	aiChatService, err := aichat.NewRuntimeService(aichat.Dependencies{
		Repository:        aichat.NewGormRepository(resources.DB),
		DeliveryCommitter: replyDeliveryCommitter{repository: aiReplyRepository},
		Publisher:         publisher,
		Secretbox:         providers.Secretbox,
		EngineFactory:     providers.AIChatFactory,
		FileOpener:        aiChatObjectStreamer,
		ToolRuntime:       aiToolService,
		ContextRuntime:    contextRuntime,
		RunRecorder:       aiRunRecorder,
		TextGeneration:    aiTextService,
		PricingResolver:   aiOfficialModelResolver,
		RunStaleTimeout:   cfg.AI.RunStaleTimeout,
		Logger:            logger,
	})
	if err != nil {
		return nil, fmt.Errorf("build admin AI chat service: %w", err)
	}
	aiMessageService := aimessage.NewService(
		aimessage.NewGormRepository(
			resources.DB,
			aiReplyRepository,
			replycommand.NewHistoryParticipant(aiReplyRepository),
			aimessage.WithRepositoryPricingResolver(aiOfficialModelResolver),
			aimessage.WithRepositoryUploadRuleGuard(uploadRuleResolver),
			aimessage.WithRepositoryHistoryDerivedInvalidator(historyInvalidator),
		),
		aimessage.WithReplyWaker(replycommand.NewWakeupEnqueuer(input.Queue)),
		aimessage.WithCancelPublisher(replycommand.NewRedisCancelPublisher(resources.Redis)),
		aimessage.WithPricingResolver(aiOfficialModelResolver),
		aimessage.WithTransportCapabilityResolver(providers.AITransportCapabilities),
		aimessage.WithObjectInspector(aiChatObjectInspector),
		aimessage.WithUploadRuleResolver(uploadRuleResolver),
		aimessage.WithConversationDocumentEnsurer(conversationDocuments),
	)
	notificationTaskService := notificationtask.NewService(
		notificationtask.NewGormRepository(resources.DB, notificationtask.WithDurableEventSink(realtimeEventSink)),
		notificationtask.WithEnqueuer(input.Queue),
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
			CronTasks:     cronTaskService,
			Exports:       exportTaskService,
			OperationLogs: operationService,
			QueueMonitor:  queueMonitorService,
			Settings:      systemSettingService,
			Logs:          systemLogService,
		},
		Communications: CommunicationsGraph{
			Notifications:     notificationService,
			NotificationTasks: notificationTaskService,
			Mail:              mailService,
			SMS:               smsService,
			UploadConfig:      uploadConfigService,
			UploadTokens:      uploadTokenService,
		},
		Commerce: CommerceGraph{Payment: paymentService, Wallet: walletService, RedeemCodes: redeemCodeService},
		AI: AIGraph{
			Context:        contextService,
			Agents:         aiAgentService,
			Chat:           aiChatService,
			Conversations:  aiConversationService,
			Messages:       aiMessageService,
			OfficialModels: aiOfficialModelResolver,
			Providers:      aiProviderService,
			Runs:           aiRunService,
			Tools:          aiToolService,
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
		Graph:             graph,
		Authenticator:     tokenAuthenticatorFor(sessionAuthenticator),
		PermissionChecker: permissionCheckerFor(principalService),
		OperationRecorder: operationRecorder,
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
	return nil
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
