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
	"admin_back_go/internal/module/operationlog"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/role"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/platform/taskqueue"
	"admin_back_go/internal/server"
)

const shutdownTimeout = 5 * time.Second

type App struct {
	cfg         config.Config
	logger      *slog.Logger
	server      *http.Server
	resources   *Resources
	queueClient *taskqueue.Client
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
	if cfg.Queue.Enabled {
		client, err := taskqueue.NewClient(cfg.Redis, cfg.Queue)
		if err != nil {
			logger.Error("failed to initialize login log queue producer", "error", err)
		} else {
			queueClient = client
			loginLogEnqueuer = client
		}
	}
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
	var operationRecorder middleware.OperationRecorder
	if operationRepository != nil {
		operationRecorder = operationlog.NewRecorder(operationRepository)
	}
	userService := user.NewService(
		userRepository,
		permissionService,
		buttonGrantCache,
		0,
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
		PermissionRules:     permissionRouteRules(),
		OperationRecorder:   operationRecorder,
		OperationRules:      operationRouteRules(),
		AuthService:         authService,
		CaptchaService:      captchaService,
		UserService:         userService,
		PermissionService:   permissionService,
		RoleService:         roleService,
		AuthPlatformService: authPlatformService,
	})
	return &App{
		cfg:         cfg,
		logger:      logger,
		resources:   resources,
		queueClient: queueClient,
		server: &http.Server{
			Addr:              cfg.HTTP.Addr,
			Handler:           router,
			ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		},
	}
}

func (a *App) Run() error {
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
	queueErr := a.queueClient.Close()
	resourceErr := a.resources.Close()
	return errors.Join(shutdownErr, queueErr, resourceErr)
}
