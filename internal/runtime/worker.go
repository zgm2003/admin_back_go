package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"admin_back_go/internal/config"
	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/infra/redislock"
	"admin_back_go/internal/infra/scheduler"
	"admin_back_go/internal/infra/secretkey"
	storagecos "admin_back_go/internal/infra/storage/cos"
	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/jobs"
	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/ai/contextengine"
	aiimage "admin_back_go/internal/module/ai/image"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/replycommand"
	airun "admin_back_go/internal/module/ai/run"
	aitext "admin_back_go/internal/module/ai/text"
	aitool "admin_back_go/internal/module/ai/tool"
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/crontask"
	exporttask "admin_back_go/internal/module/export"
	notificationtask "admin_back_go/internal/module/notification/task"
	paymentmodule "admin_back_go/internal/module/payment"
	walletmodule "admin_back_go/internal/module/payment/wallet"
	modulerealtime "admin_back_go/internal/module/realtime"
	"admin_back_go/internal/module/uploadtoken"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/telemetry"
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

func NewWorker(cfg config.Config, logger *slog.Logger, optionValues ...ProcessOption) (*WorkerRuntime, error) {
	cfg = config.Snapshot(cfg)
	if logger == nil {
		logger = slog.Default()
	}
	cfg.Scheduler = config.NormalizeSchedulerConfig(cfg.Scheduler)
	cfg.AI = config.NormalizeAIConfig(cfg.AI)
	if err := config.ValidateRuntimeSecrets(cfg); err != nil {
		return nil, err
	}
	keys, err := secretkey.NewKeyRingWithPrevious(cfg.App.Secret, cfg.App.PreviousSecrets)
	if err != nil {
		return nil, err
	}
	settings := resolveProcessOptions(optionValues)
	return newWorkerRuntimeWithHooks(productionWorkerHooks(cfg, logger, keys, settings.recorder)), nil
}

func productionWorkerHooks(cfg config.Config, logger *slog.Logger, keys *secretkey.KeyRing, recorders ...telemetry.Recorder) workerHooks {
	recorder := telemetry.Noop()
	if len(recorders) > 0 && recorders[0] != nil {
		recorder = recorders[0]
	}
	var resources *Resources
	var providers Providers
	var queueClient *taskqueue.Client
	var queueServer *taskqueue.Server
	var queueMux *taskqueue.Mux
	var workerScheduler *scheduler.Scheduler
	var scheduleReconciler atomic.Pointer[crontask.Reconciler]
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
			opened, err := OpenResourcesWithStartupRetry(ctx, config.ProcessWorker, cfg, Openers{Telemetry: recorder})
			if err != nil {
				return nil, nil, err
			}
			resources = opened
			return func(ctx context.Context) Report {
				base := opened.Health(ctx)
				reconciler := scheduleReconciler.Load()
				if reconciler == nil {
					return mergeSchedulerReconcilerHealth(base, cfg.Scheduler.Enabled, false, crontask.ReconcileHealth{})
				}
				return mergeSchedulerReconcilerHealth(base, cfg.Scheduler.Enabled, true, reconciler.Health())
			}, opened.Close, nil
		},
		buildProviders: func(context.Context) (CleanupFunc, error) {
			built, err := BuildProviders(cfg, keys, logger, recorder)
			if err != nil {
				return nil, err
			}
			providers = built
			if !cfg.Queue.Enabled {
				return nil, nil
			}

			registry, err := jobs.NewRegistry(jobs.Dependencies{Logger: logger})
			if err != nil {
				return nil, err
			}
			queueClient, err = taskqueue.NewClient(cfg.Redis, cfg.Queue, taskqueue.WithTelemetry(recorder), taskqueue.WithRegistry(registry))
			if err != nil {
				return nil, err
			}
			queueServer, err = taskqueue.NewServer(cfg.Redis, cfg.Queue, taskqueue.WithTelemetry(recorder))
			if err != nil {
				_ = queueClient.Close()
				queueClient = nil
				return nil, err
			}
			queueMux = taskqueue.NewMux(taskqueue.WithTelemetry(recorder))
			return func(ctx context.Context) error {
				return errors.Join(stopQueue(ctx), queueClient.Close())
			}, nil
		},
		buildHandlers: func(ctx context.Context) (CleanupFunc, error) {
			if !cfg.Queue.Enabled {
				return nil, nil
			}
			if resources == nil || queueClient == nil || queueMux == nil {
				return nil, errors.New("worker runtime queue graph is incomplete")
			}
			publisher := realtimePublisherForWorker(cfg, resources, recorder)
			replyRunner, replyReconciler, textReconciler, imageReconciler, contextReconciler, err := registerWorkerHandlers(cfg, logger, resources, providers, publisher, queueClient, queueMux, recorder)
			if err != nil {
				return nil, err
			}
			runnerCleanup, err := startReplyCommandPoller(ctx, replyRunner, time.Second, max(1, cfg.Queue.Concurrency), logger)
			if err != nil {
				return nil, err
			}
			reconcilerCleanup, err := startReplyCommandPoller(ctx, replyReconciler, time.Second, 1, logger)
			if err != nil {
				_ = runnerCleanup(context.WithoutCancel(ctx))
				return nil, err
			}
			textReconcilerCleanup, err := startReplyCommandPoller(ctx, textReconciler, time.Second, 1, logger)
			if err != nil {
				_ = reconcilerCleanup(context.WithoutCancel(ctx))
				_ = runnerCleanup(context.WithoutCancel(ctx))
				return nil, err
			}
			imageReconcilerCleanup, err := startReplyCommandPoller(ctx, imageReconciler, time.Second, 1, logger)
			if err != nil {
				_ = textReconcilerCleanup(context.WithoutCancel(ctx))
				_ = reconcilerCleanup(context.WithoutCancel(ctx))
				_ = runnerCleanup(context.WithoutCancel(ctx))
				return nil, err
			}
			contextReconcilerCleanup, err := startReplyCommandPoller(ctx, contextReconciler, time.Second, 1, logger)
			if err != nil {
				_ = imageReconcilerCleanup(context.WithoutCancel(ctx))
				_ = textReconcilerCleanup(context.WithoutCancel(ctx))
				_ = reconcilerCleanup(context.WithoutCancel(ctx))
				_ = runnerCleanup(context.WithoutCancel(ctx))
				return nil, err
			}
			return func(ctx context.Context) error {
				return errors.Join(contextReconcilerCleanup(ctx), imageReconcilerCleanup(ctx), textReconcilerCleanup(ctx), reconcilerCleanup(ctx), runnerCleanup(ctx))
			}, nil
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
			if queueClient == nil || resources == nil || resources.Redis == nil || resources.Redis.Redis == nil {
				return nil, errors.New("worker scheduler dependencies are required")
			}
			options := []scheduler.Option{scheduler.WithLogger(logger), scheduler.WithTelemetry(recorder)}
			options = append(options, scheduler.WithLeaseStore(redislock.New(resources.Redis.Redis)))
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
			if err := runSchedulerReconciliation(ctx, recorder, func(context.Context) error {
				return jobs.RegisterSchedules(built, queueClient, logger)
			}); err != nil {
				_ = built.Shutdown(ctx)
				return nil, err
			}
			built.Start()
			reconciler := crontask.NewReconciler(cronScheduler, built)
			if err := reconciler.Start(ctx); err != nil {
				_ = built.Shutdown(context.WithoutCancel(ctx))
				return nil, err
			}
			scheduleReconciler.Store(reconciler)
			logger.Info("admin worker scheduler started", "timezone", cfg.Scheduler.Timezone)
			return func(ctx context.Context) error {
				return errors.Join(reconciler.Shutdown(ctx), workerScheduler.Shutdown(ctx))
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
	recorder telemetry.Recorder,
) (*replycommand.Runner, *replycommand.Reconciler, *aitext.Reconciler, *aiimage.Reconciler, *contextengine.DocumentIndexReconciler, error) {
	realtimeEventRepository := modulerealtime.NewGormRepository(resources.DB, modulerealtime.DefaultRegistry())
	realtimeEventSink := modulerealtime.NewDurableEventSink(realtimeEventRepository, realtimePublisher, logger)
	realtimeRetentionService := modulerealtime.NewRetentionService(realtimeEventRepository)
	notificationTaskService := notificationtask.NewService(
		notificationtask.NewGormRepository(resources.DB, notificationtask.WithDurableEventSink(realtimeEventSink)),
		notificationtask.WithEnqueuer(queueClient),
		notificationtask.WithLogger(logger),
	)
	exportRegistry, err := exporttask.NewRegistry(exporttask.Definition{
		Kind:     exporttask.KindUserList,
		Title:    "用户列表",
		Provider: user.NewExportDataProvider(user.NewGormRepository(resources.DB)),
	})
	if err != nil {
		return nil, nil, nil, nil, nil, err
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
	aiOfficialModelResolver := officialmodel.NewService(officialmodel.NewGormRepository(resources.DB))
	uploadTokenRepository := uploadtoken.NewGormRepository(resources.DB)
	aiChatObjectConfig := uploadtoken.NewObjectConfigProvider(uploadTokenRepository, providers.Secretbox)
	aiChatObjectStreamer := storagecos.NewObjectStreamer(
		aiChatObjectConfig,
		storagecos.ObjectStreamerConfig{Enabled: true},
	)
	contextRepository := contextengine.NewIngestionRepository(resources.DB)
	contextEmbeddings := contextengine.NewEmbeddingResolver(resources.DB, providers.AIEmbeddingFactory, providers.Secretbox)
	contextDocumentIndex := contextengine.NewDocumentIndexService(contextengine.DocumentIndexDependencies{
		Repository:       contextRepository,
		Objects:          storagecos.NewConditionalObjectReader(aiChatObjectConfig, storagecos.ObjectStreamerConfig{Enabled: true}),
		Embeddings:       contextEmbeddings,
		Index:            resources.Qdrant,
		CollectionPrefix: cfg.Qdrant.CollectionPrefix,
	})
	contextProfileRebuild := contextengine.NewProfileRebuildService(contextengine.RebuildDependencies{
		Repository: contextRepository, Embeddings: contextEmbeddings, Index: resources.Qdrant, CollectionPrefix: cfg.Qdrant.CollectionPrefix,
	})
	contextIndexCleanup := contextengine.NewIndexCleanupService(contextRepository, resources.Qdrant, cfg.Qdrant.CollectionPrefix, nil)
	contextMemoryRepository := contextengine.NewMemoryRepository(resources.DB)
	contextMemory := contextengine.NewMemoryService(contextengine.MemoryServiceDependencies{
		Repository: contextMemoryRepository,
		Summarizer: newMemoryProviderSummarizer(resources.DB, providers.AIChatFactory, providers.Secretbox),
	})
	contextMemoryEnqueuer := contextengine.NewMemoryBuildEnqueuer(queueClient)
	contextEnqueuer := contextengine.NewDocumentVersionEnqueuer(queueClient, contextRepository)
	conversationDocuments := contextengine.NewConversationDocumentService(resources.DB, contextEnqueuer)
	if conversationDocuments == nil {
		return nil, nil, nil, nil, nil, errors.New("worker conversation document dependencies are incomplete")
	}
	conversationIndexRepository := contextengine.NewConversationIndexRepository(resources.DB.Gorm)
	conversationIndexEnqueuer := contextengine.NewConversationTurnEnqueuer(queueClient)
	conversationIndex := contextengine.NewConversationIndexService(contextengine.ConversationIndexDependencies{
		Repository: conversationIndexRepository, Embeddings: contextEmbeddings, Index: resources.Qdrant, CollectionPrefix: cfg.Qdrant.CollectionPrefix,
	})
	contextRuntime := contextengine.BuildRuntime(contextengine.RuntimeDependencies{
		Database: resources.DB, OfficialModels: aiOfficialModelResolver,
		EmbeddingFactory: providers.AIEmbeddingFactory, RerankFactory: providers.AIRerankFactory,
		Secretbox: providers.Secretbox, Index: resources.Qdrant, CollectionPrefix: cfg.Qdrant.CollectionPrefix, Platform: "admin", Telemetry: recorder,
	})
	if contextRuntime == nil {
		return nil, nil, nil, nil, nil, errors.New("worker context runtime dependencies are incomplete")
	}
	aiTextTasks := aitext.NewGormStore(resources.DB)
	aiToolRepository := aitool.NewGormRepository(resources.DB)
	aiToolRuntime := aitool.NewService(aiToolRepository, aitool.DefaultExecutors(aiToolRepository))
	replyRepository := replycommand.NewGormRepository(
		resources.DB,
		replycommand.WithDurableEventSink(realtimeEventSink),
	)
	walletRepository := walletmodule.NewGormRepository(resources.DB)
	paidChatExecutor := newPaidChatAttemptExecutor(resources.DB, walletRepository, replyRepository, realtimeEventSink, contextRuntime,
		withConversationIndexPostCommit(conversationIndexRepository, conversationIndexEnqueuer),
		withMemoryPostCommit(contextMemoryRepository, contextMemoryEnqueuer))
	if paidChatExecutor == nil {
		return nil, nil, nil, nil, nil, errors.New("worker paid AI Gateway dependencies are incomplete")
	}
	paidTextExecutor := newPaidTextTaskExecutor(resources.DB, walletRepository, aiTextTasks, providers.AIChatFactory, providers.AIToolFactory, providers.Secretbox)
	if paidTextExecutor == nil {
		return nil, nil, nil, nil, nil, errors.New("worker paid AI text Gateway dependencies are incomplete")
	}
	textWaker := aitext.NewWakeupEnqueuer(queueClient)
	aiTextService := aitext.NewService(aitext.ServiceDependencies{Store: aiTextTasks, Waker: textWaker, Executor: paidTextExecutor})
	aiChatService, err := aichat.NewRuntimeService(aichat.Dependencies{
		Repository:          aichat.NewGormRepository(resources.DB),
		AssistantPublisher:  replyAssistantPublisher{repository: replyRepository},
		DeliveryCommitter:   replyDeliveryCommitter{repository: replyRepository},
		PaidAttemptExecutor: paidChatExecutor,
		Publisher:           realtimePublisher,
		Secretbox:           providers.Secretbox,
		EngineFactory:       providers.AIChatFactory,
		FileOpener:          aiChatObjectStreamer,
		ToolRuntime:         aiToolRuntime,
		ContextRuntime:      contextRuntime,
		RunRecorder:         aiRunRecorder,
		TextGeneration:      aiTextService,
		PricingResolver:     aiOfficialModelResolver,
		RunStaleTimeout:     positiveProviderDuration(cfg.AI.RunStaleTimeout, config.DefaultAIRunStaleTimeout),
		Logger:              logger,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("build worker AI chat service: %w", err)
	}
	replyRunner := replycommand.NewRunner(replycommand.RunnerOptions{
		Repository:       replyRepository,
		Executor:         aiChatService,
		Finalizer:        aiChatService,
		CancelSubscriber: replycommand.NewRedisCancelSubscriber(resources.RealtimeRedis),
		Logger:           logger,
	})
	imageRepository := aiimage.NewGormRepository(resources.DB)
	paidImageExecutor := newPaidImageTaskExecutor(resources.DB, walletRepository, imageRepository, providers.AIImageFactory, providers.Secretbox, providers.ObjectReader, providers.ObjectWriter)
	if paidImageExecutor == nil {
		return nil, nil, nil, nil, nil, errors.New("worker paid AI image Gateway dependencies are incomplete")
	}
	imageWaker := aiimage.NewWakeupEnqueuer(queueClient)
	aiImageService := aiimage.NewService(aiimage.Dependencies{
		Repository:      imageRepository,
		Enqueuer:        queueClient,
		Secretbox:       providers.Secretbox,
		EngineFactory:   providers.AIImageFactory,
		ObjectReader:    providers.ObjectReader,
		ObjectWriter:    providers.ObjectWriter,
		RunRecorder:     aiRunRecorder,
		Executor:        paidImageExecutor,
		PricingResolver: aiOfficialModelResolver,
	})
	paymentService := paymentmodule.NewService(paymentmodule.Dependencies{
		Repository:   paymentmodule.NewGormRepository(resources.DB, walletRepository),
		Gateway:      providers.PaymentGateway,
		Secretbox:    providers.Secretbox,
		CertResolver: providers.PaymentCertResolver,
		CertStore:    providers.PaymentCertStore,
	})
	registry, err := jobs.NewRegistry(jobs.Dependencies{
		Logger:                   logger,
		AIChatService:            aiChatService,
		AITextService:            aiTextService,
		AIReplyRunner:            replyRunner,
		AiImageService:           aiImageService,
		AuthRepository:           auth.NewGormRepository(resources.DB),
		ExportTaskService:        exportTaskService,
		NotificationTaskService:  notificationTaskService,
		PaymentService:           paymentService,
		RealtimeRetentionService: realtimeRetentionService,
		ContextDocumentIndex:     contextDocumentIndex,
		ContextMemoryBuild:       contextMemory,
		ContextConversationIndex: conversationIndex,
		ContextProfileRebuild:    contextProfileRebuild,
		ContextIndexCleanup:      contextIndexCleanup,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	if err := requireContextTaskRegistrations(registry); err != nil {
		return nil, nil, nil, nil, nil, err
	}
	if err := queueMux.RegisterRegistry(registry); err != nil {
		return nil, nil, nil, nil, nil, err
	}
	return replyRunner,
		replycommand.NewReconciler(replycommand.ReconcilerOptions{Repository: replyRepository, Finalizer: paidChatExecutor}),
		aitext.NewReconciler(aiTextTasks, textWaker, max(25, cfg.Queue.Concurrency)),
		aiimage.NewReconciler(imageRepository, imageWaker, max(25, cfg.Queue.Concurrency)),
		contextengine.NewDocumentIndexReconciler(contextRepository, contextEnqueuer, max(25, cfg.Queue.Concurrency), uint32(jobs.ContextDocumentIndexMaxRetry+1),
			contextengine.WithProfileIndexConsistency(contextRepository, resources.Qdrant, cfg.Qdrant.CollectionPrefix),
			contextengine.WithConversationIndexRepair(conversationIndexRepository, conversationIndexEnqueuer),
			contextengine.WithConversationDocumentRepair(conversationDocuments, conversationDocuments),
			contextengine.WithMemoryRepair(contextMemoryRepository, contextMemoryEnqueuer)),
		nil
}

func requireContextTaskRegistrations(registry *taskqueue.Registry) error {
	if registry == nil {
		return errors.New("worker task registry is required")
	}
	required := map[string]bool{
		contextengine.TaskContextConversationIndexV1: false,
		contextengine.TaskContextDocumentIndexV1:     false,
		contextengine.TaskContextIndexCleanupV1:      false,
		contextengine.TaskContextMemoryBuildV1:       false,
		contextengine.TaskContextProfileRebuildV1:    false,
	}
	for _, taskType := range registry.Types() {
		if !strings.HasPrefix(taskType, "ai:context-") {
			continue
		}
		if _, known := required[taskType]; !known {
			return fmt.Errorf("unexpected Context task registration: %s", taskType)
		}
		required[taskType] = true
	}
	for taskType, found := range required {
		if !found {
			return fmt.Errorf("required Context task is not registered: %s", taskType)
		}
	}
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

func mergeSchedulerReconcilerHealth(base Report, enabled bool, started bool, health crontask.ReconcileHealth) Report {
	if !enabled {
		return cloneReport(base)
	}
	checks := make(map[string]Check, len(base.Checks)+1)
	for name, check := range base.Checks {
		checks[name] = check
	}
	if !started {
		checks["scheduler"] = Check{Status: StatusDown, Message: "cron schedule reconciler has not started"}
		return NewReport(checks)
	}
	if !health.Healthy {
		message := health.Err
		if message == "" {
			message = "cron schedules have not reconciled successfully"
		}
		checks["scheduler"] = Check{Status: StatusDown, Message: message}
	}
	return NewReport(checks)
}

var _ Runtime = (*WorkerRuntime)(nil)
