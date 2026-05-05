package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/authplatform"
	"admin_back_go/internal/module/captcha"
	"admin_back_go/internal/module/notification"
	"admin_back_go/internal/module/notificationtask"
	"admin_back_go/internal/module/operationlog"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/queuemonitor"
	"admin_back_go/internal/module/role"
	"admin_back_go/internal/module/systemlog"
	"admin_back_go/internal/module/systemsetting"
	"admin_back_go/internal/module/uploadconfig"
	"admin_back_go/internal/module/uploadtoken"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/platform/logstore"
	platformrealtime "admin_back_go/internal/platform/realtime"
	"admin_back_go/internal/platform/secretbox"
	storagecos "admin_back_go/internal/platform/storage/cos"
	"admin_back_go/internal/platform/taskqueue"
	"admin_back_go/internal/server"
)

const shutdownTimeout = 5 * time.Second

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
	uploadConfigService := uploadconfig.NewService(uploadconfig.NewGormRepository(resources.DB), &secretBox)
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
	operationRepository := operationlog.NewGormRepository(resources.DB)
	operationService := operationlog.NewService(operationRepository)
	notificationService := notification.NewService(notification.NewGormRepository(resources.DB))
	realtimeStack := newRealtimeStackWithRedis(cfg.Realtime, resources.Redis, logger)
	notificationTaskService := notificationtask.NewService(
		notificationtask.NewGormRepository(resources.DB),
		notificationtask.WithEnqueuer(queueClient),
		notificationtask.WithRealtimePublisher(realtimeStack.publisher),
		notificationtask.WithLogger(logger),
	)
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
	)
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
		UserService:             userService,
		NotificationService:     notificationService,
		NotificationTaskService: notificationTaskService,
		OperationLogService:     operationService,
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
		server: &http.Server{
			Addr:              cfg.HTTP.Addr,
			Handler:           router,
			ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		},
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
	queueErr := a.queueClient.Close()
	inspectorErr := a.queueInspector.Close()
	monitorErr := a.queueMonitorUI.Close()
	resourceErr := a.resources.Close()
	return errors.Join(shutdownErr, realtimeErr, queueErr, inspectorErr, monitorErr, resourceErr)
}
