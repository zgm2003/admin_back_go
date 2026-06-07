package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"admin_back_go/internal/config"
	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/ai/imagecompat"
	"admin_back_go/internal/infra/ai/openaicompat"
	"admin_back_go/internal/infra/logstore"
	inframail "admin_back_go/internal/infra/mail/tencentcloudses"
	"admin_back_go/internal/infra/payment"
	payalipay "admin_back_go/internal/infra/payment/alipay"
	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/infra/secretkey"
	infrasms "admin_back_go/internal/infra/sms/tencentcloudsms"
	storagecos "admin_back_go/internal/infra/storage/cos"
	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/middleware"
	aiagent "admin_back_go/internal/module/ai/agent"
	aichat "admin_back_go/internal/module/ai/chat"
	aiconversation "admin_back_go/internal/module/ai/conversation"
	aiimage "admin_back_go/internal/module/ai/image"
	aiknowledge "admin_back_go/internal/module/ai/knowledge"
	aimessage "admin_back_go/internal/module/ai/message"
	aiprovider "admin_back_go/internal/module/ai/provider"
	airun "admin_back_go/internal/module/ai/run"
	aitext "admin_back_go/internal/module/ai/text"
	aitool "admin_back_go/internal/module/ai/tool"
	aivideo "admin_back_go/internal/module/ai/video"
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/auth_platform"
	canvasmodule "admin_back_go/internal/module/canvas"
	"admin_back_go/internal/module/clientversion"
	"admin_back_go/internal/module/crontask"
	"admin_back_go/internal/module/export"
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
	"admin_back_go/internal/server"
	"admin_back_go/internal/shared/apperror"
)

const shutdownTimeout = 5 * time.Second

func aiReplyTimeout(maxDuration time.Duration) time.Duration {
	return positiveDuration(maxDuration, config.DefaultAIChatStreamMaxDuration) + 30*time.Second
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
	realtimeManager    *infrarealtime.Manager
	realtimePublisher  infrarealtime.Publisher
	realtimeSubscriber *infrarealtime.RedisSubscriber
	aiReplyDispatcher  *aiConversationReplyDispatcher
}

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.Default()
	}
	cfg.AI = config.NormalizeAIConfig(cfg.AI)
	if err := config.ValidateRuntimeSecrets(cfg); err != nil {
		return nil, err
	}
	keys, err := secretkey.NewKeyRing(cfg.App.Secret)
	if err != nil {
		return nil, err
	}

	resources, err := NewResources(cfg)
	if err != nil {
		logger.Error("failed to initialize resources", "error", err)
		if resources == nil {
			resources = &Resources{}
		}
	}

	sessionAuthenticator := NewSessionAuthenticator(resources, cfg, keys)
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
	systemSettingRepository := systemsetting.NewGormRepository(resources.DB, resources.Redis)
	systemSettingService := systemsetting.NewService(systemSettingRepository)
	secretBox := secretbox.New(keys.SecretboxKey())
	cosObjectReader := storagecos.NewObjectReader(storagecos.ObjectReaderConfig{Enabled: true})
	cosObjectWriter := storagecos.NewObjectWriter(storagecos.ObjectWriterConfig{Enabled: true})
	walletService := walletmodule.NewService(walletmodule.NewGormRepository(resources.DB))
	uploadConfigService := uploadconfig.NewService(uploadconfig.NewGormRepository(resources.DB), &secretBox)
	sesClient := inframail.New(10 * time.Second)
	mailSender := mail.SenderFunc(func(ctx context.Context, input mail.SendInput) (mail.SendResult, error) {
		result, err := sesClient.Send(ctx, inframail.SendInput{
			SecretID:     input.SecretID,
			SecretKey:    input.SecretKey,
			Region:       input.Region,
			Endpoint:     input.Endpoint,
			FromEmail:    input.FromEmail,
			FromName:     input.FromName,
			ReplyTo:      input.ReplyTo,
			ToEmail:      input.ToEmail,
			Subject:      input.Subject,
			TemplateID:   input.TemplateID,
			TemplateData: input.TemplateData,
		})
		if err != nil {
			return mail.SendResult{}, err
		}
		return mail.SendResult{RequestID: result.RequestID, MessageID: result.MessageID}, nil
	})
	mailService := mail.NewService(mail.NewGormRepository(resources.DB), secretBox, mailSender)
	smsClient := infrasms.New(10 * time.Second)
	smsSender := sms.SenderFunc(func(ctx context.Context, input sms.SendInput) (sms.SendResult, error) {
		result, err := smsClient.Send(ctx, infrasms.SendInput{
			SecretID:         input.SecretID,
			SecretKey:        input.SecretKey,
			Region:           input.Region,
			Endpoint:         input.Endpoint,
			SmsSdkAppID:      input.SmsSdkAppID,
			SignName:         input.SignName,
			TemplateID:       input.TemplateID,
			PhoneNumber:      input.PhoneNumber,
			TemplateParamSet: input.TemplateParamSet,
		})
		if err != nil {
			return sms.SendResult{RequestID: result.RequestID, SerialNo: result.SerialNo, Fee: result.Fee}, err
		}
		return sms.SendResult{RequestID: result.RequestID, SerialNo: result.SerialNo, Fee: result.Fee}, nil
	})
	smsService := sms.NewService(sms.NewGormRepository(resources.DB), secretBox, smsSender)
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
	aiRunRepository := airun.NewGormRepository(resources.DB)
	aiRunRecorder := airun.NewRecorder(aiRunRepository, nil)
	aiTextTasks := aitext.NewGormStore(resources.DB)
	aiImageService := aiimage.NewService(aiimage.Dependencies{
		Repository:    aiimage.NewGormRepository(resources.DB),
		Enqueuer:      queueClient,
		Secretbox:     secretBox,
		EngineFactory: aiImageEngineFactory{},
		ObjectReader:  cosObjectReader,
		ObjectWriter:  cosObjectWriter,
		RunRecorder:   aiRunRecorder,
	})
	aiToolRepo := aitool.NewGormRepository(resources.DB)
	aiToolService := aitool.NewService(
		aiToolRepo,
		aitool.DefaultExecutors(aiToolRepo),
		aitool.WithSecretbox(secretBox),
		aitool.WithEngineFactory(aiToolGenerateEngineFactory{}),
	)
	aiKnowledgeService := aiknowledge.NewService(aiknowledge.NewGormRepository(resources.DB))
	aiConversationService := aiconversation.NewService(aiconversation.NewGormRepository(resources.DB))
	aiRunService := airun.NewService(aiRunRepository)
	paymentCertResolver := payment.CertPathResolver{
		CertBaseDir: cfg.Payment.CertBaseDir,
		WorkingDir:  ".",
	}
	paymentCertStore := payment.LocalCertStore{BaseDir: cfg.Payment.CertBaseDir}
	alipayGateway := payalipay.NewGopayGateway()
	paymentGateway := payalipay.NewPlatformGateway(alipayGateway)
	aiVideoService := aivideo.NewService(aivideo.Dependencies{
		Repository:    aivideo.NewGormRepository(resources.DB),
		Secretbox:     secretBox,
		EngineFactory: aiVideoEngineFactory{},
		RunRecorder:   aiRunRecorder,
	})
	canvasService := canvasmodule.NewServiceWithSettings(canvasmodule.NewGormRepository(resources.DB), canvasmodule.SettingsDependencies{
		AuthPolicy: authPlatformService,
	})
	paymentService := paymentmodule.NewService(paymentmodule.Dependencies{
		Repository:   paymentmodule.NewGormRepository(resources.DB),
		Gateway:      paymentGateway,
		Secretbox:    secretBox,
		CertResolver: paymentCertResolver,
		CertStore:    paymentCertStore,
	})

	cosSigner := storagecos.NewSigner(storagecos.Config{Enabled: true})
	uploadTokenService := uploadtoken.NewService(
		uploadtoken.NewGormRepository(resources.DB),
		secretBox,
		cosSigner,
		uploadtoken.Options{
			TTLPolicy: uploadtoken.NewSystemSettingTTLPolicyProvider(systemSettingRepository),
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
	var captchaService *auth.CaptchaService
	captchaEngine, captchaErr := auth.NewSlideEngine()
	if captchaErr != nil {
		logger.Error("failed to initialize captcha engine", "error", captchaErr)
	} else {
		captchaService = auth.NewCaptchaService(
			captchaEngine,
			auth.NewCaptchaRedisStore(resources.Redis, ""),
			auth.NewSystemSettingCaptchaPolicyProvider(systemSettingRepository),
		)
	}
	authService := auth.NewService(
		auth.NewGormRepository(resources.DB),
		authPlatformService,
		sessionAuthenticator,
		captchaService,
		auth.WithCodeStore(auth.NewRedisCodeStore(resources.Redis)),
		auth.WithVerifyCodeMailSender(mailService),
		auth.WithVerifyCodePolicyProvider(auth.NewChannelVerifyCodePolicyProvider(mailService, smsService)),
		auth.WithLoginLogEnqueuer(loginLogEnqueuer),
		auth.WithLogger(logger),
	)
	routeAccessGrantCache := permission.NewRedisRouteAccessGrantCache(resources.Redis)
	permissionService := permission.NewService(
		permission.NewGormRepository(resources.DB),
		nil,
		permission.WithCacheInvalidator(routeAccessGrantCache),
	)
	roleService := role.NewService(
		role.NewGormRepository(resources.DB),
		permissionService,
		routeAccessGrantCache,
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
		EngineFactory:    aiChatEngineFactory{streamIdleTimeout: positiveDuration(cfg.AI.ChatStreamIdleTimeout, config.DefaultAIChatStreamIdleTimeout)},
		ToolRuntime:      aiToolService,
		KnowledgeRuntime: aiKnowledgeRuntimeAdapter{service: aiKnowledgeService},
		RunRecorder:      aiRunRecorder,
		TextTasks:        aiTextTasks,
		RunStaleTimeout:  positiveDuration(cfg.AI.RunStaleTimeout, config.DefaultAIRunStaleTimeout),
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
		routeAccessGrantCache,
		0,
		user.WithVerifyCodeStore(auth.NewRedisCodeStore(resources.Redis)),
		user.WithExportTaskCreator(exportTaskService),
		user.WithExportEnqueuer(queueClient),
		user.WithAddressDictCache(addressDictCache),
	)
	sessionRevoker := auth.NewSessionRevocationService(auth.NewSessionRedisCache(resourcesTokenRedis(resources)), auth.SessionRevocationConfig{RedisPrefix: cfg.Token.RedisPrefix})
	loginLogService := auth.NewLoginLogService(auth.NewLoginLogGormRepository(resources.DB))
	sessionAdminService := auth.NewSessionAdminService(auth.NewSessionAdminGormRepository(resources.DB), auth.WithSessionAdminCacheRevoker(sessionRevoker))
	router := server.NewRouter(server.Dependencies{
		Readiness:     resources,
		Logger:        logger,
		CORS:          cfg.CORS,
		Authenticator: TokenAuthenticatorFor(sessionAuthenticator),
		PermissionChecker: PermissionCheckerFor(
			userRepository,
			permissionService,
			routeAccessGrantCache,
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
		AiImageService:          aiImageService,
		AiVideoService:          aiVideoService,
		AiAgentService:          aiAgentService,
		AiProviderService:       aiProviderService,
		AiKnowledgeService:      aiKnowledgeService,
		AiMessageService:        aiMessageService,
		AiRunService:            aiRunService,
		AiToolService:           aiToolService,
		CronTaskService:         cronTaskService,
		ExportTaskService:       exportTaskService,
		UserService:             userService,
		LoginLogService:         loginLogService,
		SessionAdminService:     sessionAdminService,
		NotificationService:     notificationService,
		NotificationTaskService: notificationTaskService,
		OperationLogService:     operationService,
		MailService:             mailService,
		SmsService:              smsService,
		PaymentService:          paymentService,
		WalletService:           walletService,
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
		CanvasService:           canvasService,
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
	}, nil
}

type aiProviderTester struct{}

func (aiProviderTester) TestConnection(ctx context.Context, input infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	switch input.EngineType {
	case infraai.EngineTypeOpenAI:
		return openaicompat.New(openaicompat.Config{
			BaseURL: input.BaseURL,
			APIKey:  input.APIKey,
			Timeout: time.Duration(input.TimeoutMs) * time.Millisecond,
		}).TestConnection(ctx, input)
	default:
		return nil, infraai.ErrInvalidConfig
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

func (f aiChatEngineFactory) NewEngine(ctx context.Context, input aichat.EngineConfig) (infraai.Engine, error) {
	switch input.EngineType {
	case infraai.EngineTypeOpenAI:
		return openaicompat.New(openaicompat.Config{
			BaseURL:           input.BaseURL,
			APIKey:            input.APIKey,
			Timeout:           30 * time.Second,
			StreamIdleTimeout: positiveDuration(f.streamIdleTimeout, config.DefaultAIChatStreamIdleTimeout),
		}), nil
	default:
		return nil, infraai.ErrInvalidConfig
	}
}

type aiImageEngineFactory struct{}

func (aiImageEngineFactory) NewImageEngine(config aiimage.ImageEngineConfig) infraai.ImageEngine {
	switch infraai.EngineType(config.EngineType) {
	case infraai.EngineTypeOpenAI:
		return imagecompat.New(imagecompat.Config{
			BaseURL: config.BaseURL,
			APIKey:  config.APIKey,
			Timeout: config.Timeout,
		})
	default:
		return nil
	}
}

type aiVideoEngineFactory struct{}

func (aiVideoEngineFactory) NewVideoEngine(ctx context.Context, input aivideo.EngineConfig) (infraai.VideoEngine, error) {
	switch input.EngineType {
	case infraai.EngineTypeOpenAI:
		return openaicompat.New(openaicompat.Config{
			BaseURL: input.BaseURL,
			APIKey:  input.APIKey,
			Timeout: 10 * time.Minute,
		}), nil
	default:
		return nil, infraai.ErrInvalidConfig
	}
}

type aiToolGenerateEngineFactory struct{}

func (aiToolGenerateEngineFactory) NewEngine(ctx context.Context, input aitool.EngineConfig) (infraai.Engine, error) {
	switch input.EngineType {
	case infraai.EngineTypeOpenAI:
		return openaicompat.New(openaicompat.Config{
			BaseURL: input.BaseURL,
			APIKey:  input.APIKey,
			Timeout: 30 * time.Second,
		}), nil
	default:
		return nil, infraai.ErrInvalidConfig
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
