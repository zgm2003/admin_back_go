package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/config"
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
	"admin_back_go/internal/module/notification"
	"admin_back_go/internal/module/notificationtask"
	"admin_back_go/internal/module/operationlog"
	paymentmodule "admin_back_go/internal/module/payment"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/queuemonitor"
	"admin_back_go/internal/module/role"
	"admin_back_go/internal/module/session"
	"admin_back_go/internal/module/systemlog"
	"admin_back_go/internal/module/systemsetting"
	"admin_back_go/internal/module/uploadconfig"
	"admin_back_go/internal/module/uploadtoken"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/module/userloginlog"
	"admin_back_go/internal/module/userquickentry"
	"admin_back_go/internal/module/usersession"
	platformai "admin_back_go/internal/platform/ai"
	"admin_back_go/internal/platform/ai/openaicompat"
	"admin_back_go/internal/platform/logstore"
	"admin_back_go/internal/platform/payment"
	payalipay "admin_back_go/internal/platform/payment/alipay"
	platformrealtime "admin_back_go/internal/platform/realtime"
	"admin_back_go/internal/platform/secretbox"
	storagecos "admin_back_go/internal/platform/storage/cos"
	"admin_back_go/internal/platform/taskqueue"
	"admin_back_go/internal/server"
)

const shutdownTimeout = 5 * time.Second

func aiReplyTimeout(maxDuration time.Duration) time.Duration {
	return positiveDuration(maxDuration, 5*time.Minute) + 30*time.Second
}

func positiveDuration(value time.Duration, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

type App struct {
	cfg                config.Config
	logger             *slog.Logger
	server             *http.Server
	resources          *Resources
	queueClient        *taskqueue.Client
	queueInspector     *taskqueue.Inspector
	queueMonitorUI     *queuemonitor.MonitorUI
	realtimeManager    *platformrealtime.Manager
	realtimePublisher  platformrealtime.Publisher
	realtimeSubscriber *platformrealtime.RedisSubscriber
	aiReplyDispatcher  *aiConversationReplyDispatcher
}

func New(cfg config.Config, logger *slog.Logger) *App {
	if logger == nil {
		logger = slog.Default()
	}

	resources, err := NewResources(cfg)
	if err != nil {
		logger.Error("failed to initialize resources", "error", err)
		if resources == nil {
			resources = &Resources{}
		}
	}

	sessionAuthenticator := NewSessionAuthenticator(resources, cfg)
	authPlatformService := authplatform.NewService(authplatform.NewGormRepository(resources.DB))
	var loginLogEnqueuer taskqueue.Enqueuer
	var queueClient *taskqueue.Client
	var queueInspector *taskqueue.Inspector
	var queueMonitorUI *queuemonitor.MonitorUI
	if cfg.Queue.Enabled {
		client, err := taskqueue.NewClient(cfg.Redis, cfg.Queue)
		if err != nil {
			logger.Error("failed to initialize login log queue producer", "error", err)
		} else {
			queueClient = client
			loginLogEnqueuer = client
		}
		inspector, err := taskqueue.NewInspector(cfg.Redis, cfg.Queue)
		if err != nil {
			logger.Error("failed to initialize queue inspector", "error", err)
		} else {
			queueInspector = inspector
			monitorUI, err := queuemonitor.NewMonitorUI(cfg.Redis, cfg.Queue)
			if err != nil {
				if !queuemonitor.IsUIConfigError(err) {
					logger.Error("failed to initialize queue monitor UI", "error", err)
				}
			} else {
				queueMonitorUI = monitorUI
			}
		}
	}
	systemLogService := systemlog.NewService(logstore.New(cfg.Logging.Dir, logstore.Options{AllowedExtensions: cfg.Logging.AllowedExtensions, MaxTailLines: cfg.Logging.MaxTailLines}))
	systemSettingService := systemsetting.NewService(systemsetting.NewGormRepository(resources.DB, resources.Redis))
	secretBox := secretbox.New(cfg.Secretbox.Key)
	cosObjectWriter := storagecos.NewObjectWriter(storagecos.ObjectWriterConfig{Enabled: cfg.UploadToken.COS.Enabled})
	uploadConfigService := uploadconfig.NewService(uploadconfig.NewGormRepository(resources.DB), &secretBox)
	clientVersionService := clientversion.NewService(
		clientversion.NewGormRepository(resources.DB),
		clientversion.NewManifestPublisher(
			clientversion.NewGormUploadConfigRepository(resources.DB),
			secretBox,
			cosObjectWriter,
		),
	)
	aiProviderService := aiprovider.NewService(aiprovider.NewGormRepository(resources.DB), secretBox, aiProviderTester{})
	aiAgentService := aiagent.NewService(aiagent.NewGormRepository(resources.DB), secretBox, aiProviderTester{})
	aiToolRepo := aitool.NewGormRepository(resources.DB)
	aiToolService := aitool.NewService(
		aiToolRepo,
		aitool.DefaultExecutors(aiToolRepo),
		aitool.WithSecretbox(secretBox),
		aitool.WithEngineFactory(aiToolGenerateEngineFactory{}),
	)
	aiKnowledgeService := aiknowledge.NewService(aiknowledge.NewGormRepository(resources.DB))
	aiConversationService := aiconversation.NewService(aiconversation.NewGormRepository(resources.DB))
	aiRunService := airun.NewService(airun.NewGormRepository(resources.DB))
	var paymentNumberGenerator paymentmodule.NumberGenerator
	if resources.Redis != nil && resources.Redis.Redis != nil {
		paymentNumberGenerator = paymentmodule.NewRedisNumberGeneratorFromRedis(resources.Redis.Redis)
	}
	paymentCertResolver := payment.CertPathResolver{
		CertBaseDir: cfg.Payment.CertBaseDir,
		WorkingDir:  ".",
	}
	alipayGateway := payalipay.NewGopayGateway(cfg.Payment.AlipayTimeout)
	paymentGateway := payalipay.NewPlatformGateway(alipayGateway)
	paymentService := paymentmodule.NewService(paymentmodule.Dependencies{
		Repository:      paymentmodule.NewGormRepository(resources.DB),
		Gateway:         paymentGateway,
		Secretbox:       secretBox,
		CertResolver:    paymentCertResolver,
		NumberGenerator: paymentNumberGenerator,
	})

	cosSigner := storagecos.CredentialSigner(storagecos.DisabledSigner{})
	if cfg.UploadToken.COS.Enabled {
		cosSigner = storagecos.NewSigner(storagecos.Config{
			Enabled:  true,
			Endpoint: cfg.UploadToken.COS.Endpoint,
			Region:   cfg.UploadToken.COS.Region,
		})
	}
	uploadTokenService := uploadtoken.NewService(
		uploadtoken.NewGormRepository(resources.DB),
		secretBox,
		cosSigner,
		uploadtoken.Options{
			TTL:         cfg.UploadToken.TTL,
			RandomBytes: cfg.UploadToken.KeyRandomBytes,
		},
	)
	queueMonitorService := queuemonitor.NewService(
		queuemonitor.NewTaskqueueInspector(queueInspector),
		queuemonitor.Options{QueueNames: []string{
			taskqueue.QueueCritical,
			taskqueue.QueueDefault,
			taskqueue.QueueLow,
		}},
	)
	var captchaService *captcha.Service
	captchaEngine, captchaErr := captcha.NewSlideEngine()
	if captchaErr != nil {
		logger.Error("failed to initialize captcha engine", "error", captchaErr)
	} else {
		captchaService = captcha.NewService(
			captchaEngine,
			captcha.NewRedisStore(resources.Redis, cfg.Captcha.RedisPrefix),
			captcha.WithTTL(cfg.Captcha.TTL),
			captcha.WithPadding(cfg.Captcha.SlidePadding),
		)
	}
	authService := auth.NewService(
		auth.NewGormRepository(resources.DB),
		authPlatformService,
		sessionAuthenticator,
		captchaService,
		auth.WithCodeStore(auth.NewRedisCodeStore(resources.Redis)),
		auth.WithVerifyCodeOptions(auth.VerifyCodeOptions{
			TTL:         cfg.VerifyCode.TTL,
			RedisPrefix: cfg.VerifyCode.RedisPrefix,
			DevMode:     cfg.VerifyCode.DevMode,
			DevCode:     cfg.VerifyCode.DevCode,
		}),
		auth.WithLoginLogEnqueuer(loginLogEnqueuer),
		auth.WithLogger(logger),
	)
	buttonGrantCache := permission.NewRedisButtonGrantCache(resources.Redis)
	permissionService := permission.NewService(
		permission.NewGormRepository(resources.DB),
		nil,
		permission.WithCacheInvalidator(buttonGrantCache),
	)
	roleService := role.NewService(
		role.NewGormRepository(resources.DB),
		permissionService,
		buttonGrantCache,
		nil,
	)
	userRepository := user.NewGormRepository(resources.DB)
	addressDictCache := user.NewRedisAddressDictCache(resources.Redis)
	operationRepository := operationlog.NewGormRepository(resources.DB)
	operationService := operationlog.NewService(operationRepository)
	notificationService := notification.NewService(notification.NewGormRepository(resources.DB))
	realtimeStack := newRealtimeStackWithRedis(cfg.Realtime, cfg.CORS.AllowOrigins, resources.Redis, logger)
	aiChatService := aichat.NewService(aichat.Dependencies{
		Repository:       aichat.NewGormRepository(resources.DB),
		Publisher:        realtimeStack.publisher,
		Secretbox:        secretBox,
		EngineFactory:    aiChatEngineFactory{streamIdleTimeout: positiveDuration(cfg.AI.ChatStreamIdleTimeout, 60*time.Second)},
		ToolRuntime:      aiToolService,
		KnowledgeRuntime: aiKnowledgeRuntimeAdapter{service: aiKnowledgeService},
		RunStaleTimeout:  positiveDuration(cfg.AI.RunStaleTimeout, 15*time.Minute),
	})
	aiReplyDispatcher := newAIConversationReplyDispatcher(aiChatService, logger, aiReplyTimeout(cfg.AI.ChatStreamMaxDuration))
	aiMessageService := aimessage.NewService(aimessage.NewGormRepository(resources.DB), aimessage.WithReplyEnqueuer(aiReplyDispatcher))
	notificationTaskService := notificationtask.NewService(
		notificationtask.NewGormRepository(resources.DB),
		notificationtask.WithEnqueuer(queueClient),
		notificationtask.WithRealtimePublisher(realtimeStack.publisher),
		notificationtask.WithLogger(logger),
	)
	exportTaskService := exporttask.NewService(
		exporttask.NewGormRepository(resources.DB),
		exporttask.WithLogger(logger),
	)
	cronTaskService := crontask.NewService(crontask.NewGormRepository(resources.DB), crontask.NewDefaultRegistry())
	var operationRecorder middleware.OperationRecorder
	if operationRepository != nil {
		operationRecorder = operationlog.NewRecorder(operationRepository)
	}
	userService := user.NewService(
		userRepository,
		permissionService,
		buttonGrantCache,
		0,
		user.WithVerifyCodeStore(auth.NewRedisCodeStore(resources.Redis), cfg.VerifyCode.RedisPrefix),
		user.WithExportTaskCreator(exportTaskService),
		user.WithExportEnqueuer(queueClient),
		user.WithAddressDictCache(addressDictCache),
	)
	sessionRevoker := session.NewRevocationService(session.NewRedisCache(resourcesTokenRedis(resources)), session.RevocationConfig{RedisPrefix: cfg.Token.RedisPrefix})
	userQuickEntryService := userquickentry.NewService(userquickentry.NewGormRepository(resources.DB))
	userLoginLogService := userloginlog.NewService(userloginlog.NewGormRepository(resources.DB))
	userSessionService := usersession.NewService(usersession.NewGormRepository(resources.DB), usersession.WithCacheRevoker(sessionRevoker))
	router := server.NewRouter(server.Dependencies{
		Readiness:     resources,
		Logger:        logger,
		CORS:          cfg.CORS,
		Authenticator: TokenAuthenticatorFor(sessionAuthenticator),
		PermissionChecker: PermissionCheckerFor(
			userRepository,
			permissionService,
			buttonGrantCache,
			0,
		),
		PermissionRules:         permissionRouteRules(),
		OperationRecorder:       operationRecorder,
		OperationRules:          operationRouteRules(),
		AuthService:             authService,
		CaptchaService:          captchaService,
		ClientVersionService:    clientVersionService,
		AiChatService:           aiChatService,
		AiConversationService:   aiConversationService,
		AiAgentService:          aiAgentService,
		AiProviderService:       aiProviderService,
		AiKnowledgeService:      aiKnowledgeService,
		AiMessageService:        aiMessageService,
		AiRunService:            aiRunService,
		AiToolService:           aiToolService,
		CronTaskService:         cronTaskService,
		ExportTaskService:       exportTaskService,
		UserService:             userService,
		UserQuickEntryService:   userQuickEntryService,
		UserLoginLogService:     userLoginLogService,
		UserSessionService:      userSessionService,
		NotificationService:     notificationService,
		NotificationTaskService: notificationTaskService,
		OperationLogService:     operationService,
		PaymentService:          paymentService,
		PermissionService:       permissionService,
		QueueMonitorService:     queueMonitorService,
		QueueMonitorUI:          queueMonitorUI,
		SystemSettingService:    systemSettingService,
		SystemLogService:        systemLogService,
		UploadConfigService:     uploadConfigService,
		UploadTokenService:      uploadTokenService,
		RealtimeHandler:         realtimeStack.handler,
		RoleService:             roleService,
		AuthPlatformService:     authPlatformService,
	})
	return &App{
		cfg:                cfg,
		logger:             logger,
		resources:          resources,
		queueClient:        queueClient,
		queueInspector:     queueInspector,
		queueMonitorUI:     queueMonitorUI,
		realtimeManager:    realtimeStack.manager,
		realtimePublisher:  realtimeStack.publisher,
		realtimeSubscriber: realtimeStack.subscriber,
		aiReplyDispatcher:  aiReplyDispatcher,
		server: &http.Server{
			Addr:              cfg.HTTP.Addr,
			Handler:           router,
			ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		},
	}
}

type aiConversationReplyEnqueuer struct {
	enqueuer taskqueue.Enqueuer
}

func (e aiConversationReplyEnqueuer) EnqueueConversationReply(ctx context.Context, payload aimessage.ReplyPayload) error {
	if e.enqueuer == nil {
		return errors.New("ai conversation reply queue is not configured")
	}
	task, err := aichat.NewConversationReplyTask(aichat.ConversationReplyPayload{
		ConversationID: payload.ConversationID,
		UserID:         payload.UserID,
		AgentID:        payload.AgentID,
		UserMessageID:  payload.UserMessageID,
		RequestID:      payload.RequestID,
	})
	if err != nil {
		return err
	}
	_, err = e.enqueuer.Enqueue(ctx, task)
	return err
}

func (e aiConversationReplyEnqueuer) CancelConversationReply(ctx context.Context, payload aimessage.ReplyPayload) error {
	return errors.New("queued ai conversation reply cancellation is not supported")
}

type aiProviderTester struct{}

func (aiProviderTester) TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error) {
	switch input.EngineType {
	case platformai.EngineTypeOpenAI:
		return openaicompat.New(openaicompat.Config{
			BaseURL: input.BaseURL,
			APIKey:  input.APIKey,
			Timeout: time.Duration(input.TimeoutMs) * time.Millisecond,
		}).TestConnection(ctx, input)
	default:
		return nil, platformai.ErrInvalidConfig
	}
}

type aiKnowledgeRuntimeAdapter struct {
	service *aiknowledge.Service
}

func (a aiKnowledgeRuntimeAdapter) RetrieveForRun(ctx context.Context, input aichat.KnowledgeRuntimeInput) (*aichat.KnowledgeContextResult, *apperror.Error) {
	if a.service == nil {
		return nil, apperror.Internal("AI知识库服务未配置")
	}
	result, appErr := a.service.RetrieveForRun(ctx, aiknowledge.KnowledgeRuntimeInput{
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
	return &aichat.KnowledgeContextResult{
		RetrievalID: result.RetrievalID,
		Status:      result.Status,
		Context:     result.Context,
	}, nil
}

type aiChatEngineFactory struct {
	streamIdleTimeout time.Duration
}

func (f aiChatEngineFactory) NewEngine(ctx context.Context, input aichat.EngineConfig) (platformai.Engine, error) {
	switch input.EngineType {
	case platformai.EngineTypeOpenAI:
		return openaicompat.New(openaicompat.Config{
			BaseURL:           input.BaseURL,
			APIKey:            input.APIKey,
			Timeout:           30 * time.Second,
			StreamIdleTimeout: positiveDuration(f.streamIdleTimeout, 60*time.Second),
		}), nil
	default:
		return nil, platformai.ErrInvalidConfig
	}
}

type aiToolGenerateEngineFactory struct{}

func (aiToolGenerateEngineFactory) NewEngine(ctx context.Context, input aitool.EngineConfig) (platformai.Engine, error) {
	switch input.EngineType {
	case platformai.EngineTypeOpenAI:
		return openaicompat.New(openaicompat.Config{
			BaseURL: input.BaseURL,
			APIKey:  input.APIKey,
			Timeout: 30 * time.Second,
		}), nil
	default:
		return nil, platformai.ErrInvalidConfig
	}
}

func (a *App) Run() error {
	if a.realtimeSubscriber != nil {
		if err := a.realtimeSubscriber.Start(context.Background()); err != nil {
			a.logger.Error("failed to start realtime redis subscriber", "error", err)
		}
	}
	a.logger.Info("starting admin api", "addr", a.cfg.HTTP.Addr, "env", a.cfg.App.Env)
	if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (a *App) Shutdown(ctx context.Context) error {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
	}

	shutdownErr := a.server.Shutdown(ctx)
	var realtimeErr error
	if a.realtimeSubscriber != nil {
		realtimeErr = a.realtimeSubscriber.Shutdown(ctx)
	}
	if a.realtimeManager != nil {
		a.realtimeManager.CloseAll()
	}
	dispatchErr := a.aiReplyDispatcher.Shutdown(ctx)
	queueErr := a.queueClient.Close()
	inspectorErr := a.queueInspector.Close()
	monitorErr := a.queueMonitorUI.Close()
	resourceErr := a.resources.Close()
	return errors.Join(shutdownErr, realtimeErr, dispatchErr, queueErr, inspectorErr, monitorErr, resourceErr)
}
