package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"admin_back_go/internal/config"
	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/infra/secretkey"
	"admin_back_go/internal/infra/taskqueue"
	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/queuemonitor"
	platformadmin "admin_back_go/internal/platform/admin"
	runtimepkg "admin_back_go/internal/runtime"
	"admin_back_go/internal/server"
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
	aiReplyDispatcher  platformadmin.ReplyDispatcher
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

	resources, err := runtimepkg.OpenResources(context.Background(), config.ProcessAPI, cfg, runtimepkg.Openers{})
	if err != nil {
		return nil, err
	}
	var queueClient *taskqueue.Client
	var queueInspector *taskqueue.Inspector
	var queueMonitorUI *queuemonitor.MonitorUI
	if cfg.Queue.Enabled {
		client, err := taskqueue.NewClient(cfg.Redis, cfg.Queue)
		if err != nil {
			_ = resources.Close(context.Background())
			return nil, err
		}
		queueClient = client
		inspector, err := taskqueue.NewInspector(cfg.Redis, cfg.Queue)
		if err != nil {
			_ = queueClient.Close()
			_ = resources.Close(context.Background())
			return nil, err
		}
		queueInspector = inspector
		monitorUI, monitorErr := queuemonitor.NewMonitorUI(cfg.Redis, cfg.Queue)
		if monitorErr != nil {
			if !queuemonitor.IsUIConfigError(monitorErr) {
				_ = queueInspector.Close()
				_ = queueClient.Close()
				_ = resources.Close(context.Background())
				return nil, monitorErr
			}
		} else {
			queueMonitorUI = monitorUI
		}
	}
	realtimeStack := newRealtimeStackWithRedis(cfg.Realtime, cfg.CORS.AllowOrigins, resources.Redis, logger)
	build, err := platformadmin.Build(platformadmin.BuildInput{
		Config:            cfg,
		Resources:         resources,
		Keys:              keys,
		Logger:            logger,
		Queue:             queueClient,
		QueueInspector:    queueInspector,
		RealtimePublisher: realtimeStack.publisher,
		ReplyDispatcherFactory: func(service aichat.JobService) platformadmin.ReplyDispatcher {
			return newAIConversationReplyDispatcher(service, logger, aiReplyTimeout(cfg.AI.ChatStreamMaxDuration))
		},
	})
	if err != nil {
		if realtimeStack.manager != nil {
			realtimeStack.manager.CloseAll()
		}
		_ = queueMonitorUI.Close()
		_ = queueInspector.Close()
		_ = queueClient.Close()
		_ = resources.Close(context.Background())
		return nil, err
	}
	router := server.NewRouter(server.Dependencies{
		Core: server.CoreDependencies{
			Readiness:         resources,
			Logger:            logger,
			CORS:              cfg.CORS,
			Authenticator:     build.Authenticator,
			PermissionChecker: build.PermissionChecker,
			PermissionRules:   permissionRouteRules(),
			OperationRecorder: build.OperationRecorder,
			OperationRules:    operationRouteRules(),
			QueueMonitorUI:    queueMonitorUI,
			RealtimeHandler:   realtimeStack.handler,
		},
		Admin:   build.Graph,
		Retired: build.Retired,
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
		aiReplyDispatcher:  build.ReplyDispatcher,
		server: &http.Server{
			Addr:              cfg.HTTP.Addr,
			Handler:           router,
			ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		},
	}, nil
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
	resourceErr := a.resources.Close(ctx)
	return errors.Join(shutdownErr, realtimeErr, dispatchErr, queueErr, inspectorErr, monitorErr, resourceErr)
}
