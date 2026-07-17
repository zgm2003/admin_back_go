package runtime

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/secretkey"
	"admin_back_go/internal/infra/taskqueue"
	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/queuemonitor"
	platformadmin "admin_back_go/internal/platform/admin"
	"admin_back_go/internal/server"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/telemetry"
)

var ErrAlreadyStarted = errors.New("runtime.already_started")

type HTTPServer interface {
	ListenAndServe() error
	Shutdown(context.Context) error
}

type runtimeState uint8

const (
	runtimeStateNew runtimeState = iota
	runtimeStateStarted
	runtimeStateStopped
)

type apiHooks struct {
	openResources  func(context.Context) (func(context.Context) Report, CleanupFunc, error)
	buildProviders func(context.Context) (CleanupFunc, error)
	buildAdmin     func(context.Context) (CleanupFunc, error)
	buildRouter    func(context.Context) (CleanupFunc, error)
	startRealtime  func(context.Context) (CleanupFunc, error)
	openHTTP       func(context.Context) (HTTPServer, error)
}

type APIRuntime struct {
	mu       sync.Mutex
	state    runtimeState
	hooks    apiHooks
	server   HTTPServer
	cleanup  *Cleanup
	healthFn func(context.Context) Report
	health   atomic.Pointer[Report]
}

func NewAPI(cfg config.Config, logger *slog.Logger, routes *adminroute.Registry, optionValues ...ProcessOption) (*APIRuntime, error) {
	cfg = config.Snapshot(cfg)
	if logger == nil {
		logger = slog.Default()
	}
	cfg.AI = config.NormalizeAIConfig(cfg.AI)
	cfg.Token = config.NormalizeTokenConfig(cfg.Token)
	if err := config.ValidateRuntimeSecrets(cfg); err != nil {
		return nil, err
	}
	keys, err := secretkey.NewKeyRing(cfg.App.Secret)
	if err != nil {
		return nil, err
	}
	if routes == nil {
		return nil, errors.New("admin route registry is required")
	}
	settings := resolveProcessOptions(optionValues)
	return newAPIRuntimeWithHooks(productionAPIHooks(cfg, logger, keys, routes, settings.recorder)), nil
}

func productionAPIHooks(cfg config.Config, logger *slog.Logger, keys *secretkey.KeyRing, routes *adminroute.Registry, recorders ...telemetry.Recorder) apiHooks {
	recorder := telemetry.Noop()
	if len(recorders) > 0 && recorders[0] != nil {
		recorder = recorders[0]
	}
	var resources *Resources
	var providers Providers
	var queueClient *taskqueue.Client
	var queueInspector *taskqueue.Inspector
	var queueMonitorUI *queuemonitor.MonitorUI
	var realtime realtimeStack
	var adminBuild *platformadmin.BuildResult
	var handler http.Handler

	return apiHooks{
		openResources: func(ctx context.Context) (func(context.Context) Report, CleanupFunc, error) {
			opened, err := OpenResources(ctx, config.ProcessAPI, cfg, Openers{Telemetry: recorder})
			if err != nil {
				return nil, nil, err
			}
			resources = opened
			return opened.Health, opened.Close, nil
		},
		buildProviders: func(context.Context) (CleanupFunc, error) {
			built, err := BuildProviders(cfg, keys, recorder)
			if err != nil {
				return nil, err
			}
			providers = built
			if !cfg.Queue.Enabled {
				return nil, nil
			}

			queueClient, err = taskqueue.NewClient(cfg.Redis, cfg.Queue, taskqueue.WithTelemetry(recorder))
			if err != nil {
				return nil, err
			}
			queueInspector, err = taskqueue.NewInspector(cfg.Redis, cfg.Queue)
			if err != nil {
				_ = queueClient.Close()
				queueClient = nil
				return nil, err
			}
			queueMonitorUI, err = queuemonitor.NewMonitorUI(cfg.Redis, cfg.Queue)
			if err != nil && !queuemonitor.IsUIConfigError(err) {
				_ = queueInspector.Close()
				_ = queueClient.Close()
				queueInspector = nil
				queueClient = nil
				return nil, err
			}
			if err != nil {
				queueMonitorUI = nil
			}
			return func(context.Context) error {
				return errors.Join(queueMonitorUI.Close(), queueInspector.Close(), queueClient.Close())
			}, nil
		},
		buildAdmin: func(context.Context) (CleanupFunc, error) {
			realtime = newRealtimeStackWithRedis(cfg.Realtime, cfg.CORS.AllowOrigins, resources.Redis, logger, recorder)
			adminResources := &platformadmin.BuildResources{
				DB:         resources.DB,
				Redis:      resources.Redis,
				TokenRedis: resources.TokenRedis,
				QueueRedis: resources.QueueRedis,
			}
			adminProviders := platformadmin.ProviderSet{
				Secretbox:           providers.Secretbox,
				MailSender:          providers.MailSender,
				SMSSender:           providers.SMSSender,
				AIConnectionTester:  providers.AIConnectionTester,
				AIChatFactory:       providers.AIChatFactory,
				AIImageFactory:      providers.AIImageFactory,
				AIToolFactory:       providers.AIToolFactory,
				AIVideoFactory:      providers.AIVideoFactory,
				AIAudioFactory:      providers.AIAudioFactory,
				ObjectReader:        providers.ObjectReader,
				ObjectWriter:        providers.ObjectWriter,
				CredentialSigner:    providers.CredentialSigner,
				PaymentGateway:      providers.PaymentGateway,
				PaymentCertResolver: providers.PaymentCertResolver,
				PaymentCertStore:    providers.PaymentCertStore,
			}
			built, err := platformadmin.Build(platformadmin.BuildInput{
				Config:            cfg,
				Resources:         adminResources,
				Keys:              keys,
				Providers:         &adminProviders,
				Logger:            logger,
				Telemetry:         recorder,
				Queue:             queueClient,
				QueueInspector:    queueInspector,
				RealtimePublisher: realtime.publisher,
				ReplyDispatcherFactory: func(service aichat.JobService) platformadmin.ReplyDispatcher {
					return newAIConversationReplyDispatcher(service, logger, aiReplyTimeout(cfg.AI.ChatStreamMaxDuration))
				},
			})
			if err != nil {
				if realtime.manager != nil {
					realtime.manager.CloseAll()
				}
				return nil, err
			}
			adminBuild = built
			return built.ReplyDispatcher.Shutdown, nil
		},
		buildRouter: func(context.Context) (CleanupFunc, error) {
			if adminBuild == nil || resources == nil {
				return nil, errors.New("api runtime graph is incomplete")
			}
			builtRouter, err := server.NewRouter(server.Dependencies{
				Core: server.CoreDependencies{
					Readiness:         resources,
					Logger:            logger,
					Telemetry:         recorder,
					CORS:              cfg.CORS,
					Authenticator:     adminBuild.Authenticator,
					PermissionChecker: adminBuild.PermissionChecker,
					OperationRecorder: adminBuild.OperationRecorder,
					RouteRegistry:     routes,
					QueueMonitorUI:    queueMonitorUI,
					RealtimeHandler:   realtime.handler,
				},
				Admin:   adminBuild.Graph,
				Retired: adminBuild.Retired,
			})
			if err != nil {
				return nil, err
			}
			handler = builtRouter
			return nil, nil
		},
		startRealtime: func(ctx context.Context) (CleanupFunc, error) {
			if realtime.subscriber != nil {
				if err := realtime.subscriber.Start(ctx); err != nil {
					if realtime.manager != nil {
						realtime.manager.CloseAll()
					}
					return nil, err
				}
			}
			return func(ctx context.Context) error {
				var err error
				if realtime.subscriber != nil {
					err = realtime.subscriber.Shutdown(ctx)
				}
				if realtime.manager != nil {
					realtime.manager.CloseAll()
				}
				return err
			}, nil
		},
		openHTTP: func(context.Context) (HTTPServer, error) {
			if handler == nil {
				return nil, errors.New("api router is required")
			}
			logger.Info("starting admin api", "addr", cfg.HTTP.Addr, "env", cfg.App.Env)
			return &http.Server{
				Addr:              cfg.HTTP.Addr,
				Handler:           handler,
				ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
			}, nil
		},
	}
}

func aiReplyTimeout(maxDuration time.Duration) time.Duration {
	return positiveProviderDuration(maxDuration, config.DefaultAIChatStreamMaxDuration) + 30*time.Second
}

func newAPIRuntimeWithHooks(hooks apiHooks) *APIRuntime {
	runtime := &APIRuntime{
		state:   runtimeStateNew,
		hooks:   hooks,
		cleanup: NewCleanup(),
	}
	runtime.storeHealth(NewReport(map[string]Check{
		"runtime": {Status: StatusDown, Message: "runtime has not started"},
	}))
	return runtime
}

func (runtime *APIRuntime) Start(ctx context.Context) error {
	if runtime == nil {
		return errors.New("api runtime is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := runtime.beginStart(); err != nil {
		return err
	}

	if runtime.hooks.openResources != nil {
		healthFn, closeFn, err := runtime.hooks.openResources(ctx)
		if err != nil {
			return runtime.failStart(ctx, err)
		}
		runtime.mu.Lock()
		runtime.healthFn = healthFn
		runtime.mu.Unlock()
		if err := runtime.addCleanup(ctx, "resources", closeFn); err != nil {
			return runtime.failStart(ctx, err)
		}
	}
	for _, step := range []struct {
		name string
		hook func(context.Context) (CleanupFunc, error)
	}{
		{name: "providers", hook: runtime.hooks.buildProviders},
		{name: "admin graph", hook: runtime.hooks.buildAdmin},
		{name: "router", hook: runtime.hooks.buildRouter},
		{name: "realtime", hook: runtime.hooks.startRealtime},
	} {
		if step.hook == nil {
			continue
		}
		closeFn, err := step.hook(ctx)
		if err != nil {
			return runtime.failStart(ctx, err)
		}
		if err := runtime.addCleanup(ctx, step.name, closeFn); err != nil {
			return runtime.failStart(ctx, err)
		}
	}

	if runtime.hooks.openHTTP == nil {
		return runtime.failStart(ctx, errors.New("api HTTP server factory is required"))
	}
	server, err := runtime.hooks.openHTTP(ctx)
	if err != nil {
		return runtime.failStart(ctx, err)
	}
	if server == nil {
		return runtime.failStart(ctx, errors.New("api HTTP server is required"))
	}
	runtime.mu.Lock()
	runtime.server = server
	runtime.mu.Unlock()
	if err := runtime.addCleanup(ctx, "http server", server.Shutdown); err != nil {
		return runtime.failStart(ctx, err)
	}
	runtime.refreshHealth(ctx)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-serveErr:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return runtime.failStart(context.Background(), err)
	}
}

func (runtime *APIRuntime) Shutdown(ctx context.Context) error {
	if runtime == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runtime.mu.Lock()
	runtime.state = runtimeStateStopped
	runtime.mu.Unlock()
	err := runtime.cleanup.Close(ctx)
	runtime.storeHealth(NewReport(map[string]Check{
		"runtime": {Status: StatusDown, Message: "runtime stopped"},
	}))
	return err
}

func (runtime *APIRuntime) Health(ctx context.Context) Report {
	if runtime == nil {
		return NewReport(map[string]Check{"runtime": {Status: StatusDown, Message: "runtime is nil"}})
	}
	runtime.refreshHealth(ctx)
	stored := runtime.health.Load()
	if stored == nil {
		return NewReport(map[string]Check{"runtime": {Status: StatusDown}})
	}
	return cloneReport(*stored)
}

func (runtime *APIRuntime) beginStart() error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.state != runtimeStateNew {
		return ErrAlreadyStarted
	}
	runtime.state = runtimeStateStarted
	return nil
}

func (runtime *APIRuntime) failStart(ctx context.Context, cause error) error {
	runtime.mu.Lock()
	runtime.state = runtimeStateStopped
	runtime.mu.Unlock()
	cleanupErr := runtime.cleanup.Close(context.WithoutCancel(ctx))
	runtime.storeHealth(NewReport(map[string]Check{
		"runtime": {Status: StatusDown, Message: "runtime failed"},
	}))
	return errors.Join(cause, cleanupErr)
}

func (runtime *APIRuntime) addCleanup(ctx context.Context, name string, closeFn CleanupFunc) error {
	if closeFn == nil {
		return nil
	}
	if err := runtime.cleanup.Add(name, closeFn); err != nil {
		if ctx == nil {
			ctx = context.Background()
		}
		return errors.Join(err, closeFn(context.WithoutCancel(ctx)))
	}
	return nil
}

func (runtime *APIRuntime) refreshHealth(ctx context.Context) {
	runtime.mu.Lock()
	if runtime.state != runtimeStateStarted {
		runtime.mu.Unlock()
		return
	}
	healthFn := runtime.healthFn
	runtime.mu.Unlock()
	if healthFn == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runtime.storeHealth(healthFn(ctx))
}

func (runtime *APIRuntime) storeHealth(report Report) {
	copy := cloneReport(report)
	runtime.health.Store(&copy)
}

func cloneReport(report Report) Report {
	checks := make(map[string]Check, len(report.Checks))
	for name, check := range report.Checks {
		checks[name] = check
	}
	return Report{Status: report.Status, Checks: checks}
}

var _ Runtime = (*APIRuntime)(nil)
