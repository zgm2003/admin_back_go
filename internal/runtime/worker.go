package runtime

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"

	"admin_back_go/internal/config"
	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/infra/redislock"
	"admin_back_go/internal/infra/scheduler"
	"admin_back_go/internal/infra/secretkey"
	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/jobs"
	aichat "admin_back_go/internal/module/ai/chat"
	aiimage "admin_back_go/internal/module/ai/image"
	airun "admin_back_go/internal/module/ai/run"
	aitext "admin_back_go/internal/module/ai/text"
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/crontask"
	exporttask "admin_back_go/internal/module/export"
	notificationtask "admin_back_go/internal/module/notification/task"
	paymentmodule "admin_back_go/internal/module/payment"
	"admin_back_go/internal/module/user"
)

type workerHooks struct {
	openResources  func(context.Context) (func(context.Context) Report, CleanupFunc, error)
	buildProviders func(context.Context) (CleanupFunc, error)
	buildHandlers  func(context.Context) (CleanupFunc, error)
	startQueue     func(context.Context) (CleanupFunc, error)
	startScheduler func(context.Context) (CleanupFunc, error)
}

type WorkerRuntime struct {
	mu       sync.Mutex
	state    runtimeState
	hooks    workerHooks
	cleanup  *Cleanup
	healthFn func(context.Context) Report
	health   atomic.Pointer[Report]
}

func NewWorker(cfg config.Config, logger *slog.Logger) (*WorkerRuntime, error) {
	if logger == nil {
		logger = slog.Default()
	}
	cfg.Scheduler = config.NormalizeSchedulerConfig(cfg.Scheduler)
	cfg.AI = config.NormalizeAIConfig(cfg.AI)
	if err := config.ValidateRuntimeSecrets(cfg); err != nil {
		return nil, err
	}
	keys, err := secretkey.NewKeyRing(cfg.App.Secret)
	if err != nil {
		return nil, err
	}
	return newWorkerRuntimeWithHooks(productionWorkerHooks(cfg, logger, keys)), nil
}

func productionWorkerHooks(cfg config.Config, logger *slog.Logger, keys *secretkey.KeyRing) workerHooks {
	var resources *Resources
	var providers Providers
	var queueClient *taskqueue.Client
	var queueServer *taskqueue.Server
	var queueMux *taskqueue.Mux
	var workerScheduler *scheduler.Scheduler
	var queueStopOnce sync.Once
	queueStopped := make(chan struct{})
	stopQueue := func(ctx context.Context) error {
		queueStopOnce.Do(func() {
			go func() {
				if queueServer != nil {
					queueServer.Shutdown()
				}
				close(queueStopped)
			}()
		})
		if ctx == nil {
			<-queueStopped
			return nil
		}
		select {
		case <-queueStopped:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return workerHooks{
		openResources: func(ctx context.Context) (func(context.Context) Report, CleanupFunc, error) {
			opened, err := OpenResources(ctx, config.ProcessWorker, cfg, Openers{})
			if err != nil {
				return nil, nil, err
			}
			resources = opened
			return opened.Health, opened.Close, nil
		},
		buildProviders: func(context.Context) (CleanupFunc, error) {
			built, err := BuildProviders(cfg, keys)
			if err != nil {
				return nil, err
			}
			providers = built
			if !cfg.Queue.Enabled {
				return nil, nil
			}

			queueClient, err = taskqueue.NewClient(cfg.Redis, cfg.Queue)
			if err != nil {
				return nil, err
			}
			queueServer, err = taskqueue.NewServer(cfg.Redis, cfg.Queue)
			if err != nil {
				_ = queueClient.Close()
				queueClient = nil
				return nil, err
			}
			queueMux = taskqueue.NewMux()
			return func(ctx context.Context) error {
				return errors.Join(stopQueue(ctx), queueClient.Close())
			}, nil
		},
		buildHandlers: func(context.Context) (CleanupFunc, error) {
			if !cfg.Queue.Enabled {
				return nil, nil
			}
			if resources == nil || queueClient == nil || queueMux == nil {
				return nil, errors.New("worker runtime queue graph is incomplete")
			}
			publisher := realtimePublisherForWorker(cfg, resources)
			if err := registerWorkerHandlers(cfg, logger, resources, providers, publisher, queueClient, queueMux); err != nil {
				return nil, err
			}
			// Current publishers share process-owned Redis or are no-op values. Keep
			// the lifecycle seam explicit so a future owned publisher can close here.
			return func(context.Context) error { return nil }, nil
		},
		startQueue: func(ctx context.Context) (CleanupFunc, error) {
			if !cfg.Queue.Enabled {
				return nil, nil
			}
			if queueServer == nil || queueMux == nil {
				return nil, errors.New("worker queue server is required")
			}
			if err := queueServer.Start(queueMux); err != nil {
				_ = stopQueue(context.WithoutCancel(ctx))
				return nil, err
			}
			logger.Info("admin worker queue started", "queue_redis_db", cfg.Queue.RedisDB, "concurrency", cfg.Queue.Concurrency)
			return func(ctx context.Context) error {
				return stopQueue(ctx)
			}, nil
		},
		startScheduler: func(ctx context.Context) (CleanupFunc, error) {
			if !cfg.Scheduler.Enabled {
				return nil, nil
			}
			if queueClient == nil || resources == nil {
				return nil, errors.New("worker scheduler dependencies are required")
			}
			options := []scheduler.Option{scheduler.WithLogger(logger)}
			if resources.Redis != nil && resources.Redis.Redis != nil {
				options = append(options, scheduler.WithLocker(redislock.New(resources.Redis.Redis)))
			}
			built, err := scheduler.New(cfg.Scheduler, options...)
			if err != nil {
				return nil, err
			}
			workerScheduler = built
			cronScheduler := crontask.NewSchedulerService(
				crontask.NewGormRepository(resources.DB),
				crontask.NewDefaultRegistry(),
				queueClient,
				logger,
			)
			if err := cronScheduler.RegisterEnabled(ctx, built); err != nil {
				_ = built.Shutdown(ctx)
				return nil, err
			}
			if err := jobs.RegisterSchedules(built, queueClient, logger); err != nil {
				_ = built.Shutdown(ctx)
				return nil, err
			}
			built.Start()
			logger.Info("admin worker scheduler started", "timezone", cfg.Scheduler.Timezone)
			return func(ctx context.Context) error {
				return workerScheduler.Shutdown(ctx)
			}, nil
		},
	}
}

func registerWorkerHandlers(
	cfg config.Config,
	logger *slog.Logger,
	resources *Resources,
	providers Providers,
	realtimePublisher infrarealtime.Publisher,
	queueClient *taskqueue.Client,
	queueMux *taskqueue.Mux,
) error {
	notificationTaskService := notificationtask.NewService(
		notificationtask.NewGormRepository(resources.DB),
		notificationtask.WithEnqueuer(queueClient),
		notificationtask.WithRealtimePublisher(realtimePublisher),
		notificationtask.WithLogger(logger),
	)
	exportRegistry, err := exporttask.NewRegistry(exporttask.Definition{
		Kind:     exporttask.KindUserList,
		Title:    "用户列表",
		Provider: user.NewExportDataProvider(user.NewGormRepository(resources.DB)),
	})
	if err != nil {
		return err
	}
	exportTaskService := exporttask.NewService(
		exporttask.NewGormRepository(resources.DB),
		exporttask.WithDefinitionRegistry(exportRegistry),
		exporttask.WithFileWriter(exporttask.XLSXWriter{}),
		exporttask.WithFileUploader(exporttask.NewCOSUploader(
			exporttask.NewGormUploadConfigRepository(resources.DB),
			providers.Secretbox,
			providers.ObjectWriter,
		)),
		exporttask.WithNotifier(exporttask.NewNotificationTaskNotifier(notificationTaskService)),
		exporttask.WithLogger(logger),
	)
	aiRunRepository := airun.NewGormRepository(resources.DB)
	aiRunRecorder := airun.NewRecorder(aiRunRepository, nil)
	aiTextTasks := aitext.NewGormStore(resources.DB)
	aiChatService := aichat.NewService(aichat.Dependencies{
		Repository:      aichat.NewGormRepository(resources.DB),
		Publisher:       realtimePublisher,
		Secretbox:       providers.Secretbox,
		EngineFactory:   providers.AIChatFactory,
		RunRecorder:     aiRunRecorder,
		TextTasks:       aiTextTasks,
		RunStaleTimeout: positiveProviderDuration(cfg.AI.RunStaleTimeout, config.DefaultAIRunStaleTimeout),
	})
	aiImageService := aiimage.NewService(aiimage.Dependencies{
		Repository:    aiimage.NewGormRepository(resources.DB),
		Secretbox:     providers.Secretbox,
		EngineFactory: providers.AIImageFactory,
		ObjectReader:  providers.ObjectReader,
		ObjectWriter:  providers.ObjectWriter,
		RunRecorder:   aiRunRecorder,
	})
	paymentService := paymentmodule.NewService(paymentmodule.Dependencies{
		Repository:   paymentmodule.NewGormRepository(resources.DB),
		Gateway:      providers.PaymentGateway,
		Secretbox:    providers.Secretbox,
		CertResolver: providers.PaymentCertResolver,
		CertStore:    providers.PaymentCertStore,
	})
	jobs.Register(queueMux, jobs.Dependencies{
		Logger:                  logger,
		AIChatService:           aiChatService,
		AiImageService:          aiImageService,
		AuthRepository:          auth.NewGormRepository(resources.DB),
		ExportTaskService:       exportTaskService,
		NotificationTaskService: notificationTaskService,
		PaymentService:          paymentService,
	})
	return nil
}

func newWorkerRuntimeWithHooks(hooks workerHooks) *WorkerRuntime {
	runtime := &WorkerRuntime{
		state:   runtimeStateNew,
		hooks:   hooks,
		cleanup: NewCleanup(),
	}
	runtime.storeHealth(NewReport(map[string]Check{
		"runtime": {Status: StatusDown, Message: "runtime has not started"},
	}))
	return runtime
}

func (runtime *WorkerRuntime) Start(ctx context.Context) error {
	if runtime == nil {
		return errors.New("worker runtime is nil")
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
		{name: "task handlers", hook: runtime.hooks.buildHandlers},
		{name: "queue server", hook: runtime.hooks.startQueue},
		{name: "scheduler", hook: runtime.hooks.startScheduler},
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
	runtime.refreshHealth(ctx)
	<-ctx.Done()
	return nil
}

func (runtime *WorkerRuntime) Shutdown(ctx context.Context) error {
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

func (runtime *WorkerRuntime) Health(ctx context.Context) Report {
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

func (runtime *WorkerRuntime) beginStart() error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.state != runtimeStateNew {
		return ErrAlreadyStarted
	}
	runtime.state = runtimeStateStarted
	return nil
}

func (runtime *WorkerRuntime) failStart(ctx context.Context, cause error) error {
	runtime.mu.Lock()
	runtime.state = runtimeStateStopped
	runtime.mu.Unlock()
	cleanupErr := runtime.cleanup.Close(context.WithoutCancel(ctx))
	runtime.storeHealth(NewReport(map[string]Check{
		"runtime": {Status: StatusDown, Message: "runtime failed"},
	}))
	return errors.Join(cause, cleanupErr)
}

func (runtime *WorkerRuntime) addCleanup(ctx context.Context, name string, closeFn CleanupFunc) error {
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

func (runtime *WorkerRuntime) refreshHealth(ctx context.Context) {
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

func (runtime *WorkerRuntime) storeHealth(report Report) {
	copy := cloneReport(report)
	runtime.health.Store(&copy)
}

var _ Runtime = (*WorkerRuntime)(nil)
