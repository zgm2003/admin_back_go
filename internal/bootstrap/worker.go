package bootstrap

import (
	"context"
	"errors"
	"log/slog"

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
	"admin_back_go/internal/module/export"
	notificationtask "admin_back_go/internal/module/notification/task"
	paymentmodule "admin_back_go/internal/module/payment"
	"admin_back_go/internal/module/user"
	runtimepkg "admin_back_go/internal/runtime"
)

type Worker struct {
	cfg         config.Config
	logger      *slog.Logger
	resources   *Resources
	queueClient *taskqueue.Client
	queueServer *taskqueue.Server
	mux         *taskqueue.Mux
	scheduler   *scheduler.Scheduler
}

// NewWorker assembles the background process without starting network loops.
func NewWorker(cfg config.Config, logger *slog.Logger) (*Worker, error) {
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

	worker := &Worker{
		cfg:    cfg,
		logger: logger,
	}
	if !cfg.Queue.Enabled {
		return worker, nil
	}

	resources, err := runtimepkg.OpenResources(context.Background(), config.ProcessWorker, cfg, runtimepkg.Openers{})
	if err != nil {
		return nil, err
	}
	worker.resources = resources
	providers, err := runtimepkg.BuildProviders(cfg, keys)
	if err != nil {
		_ = resources.Close(context.Background())
		return nil, err
	}

	queueClient, err := taskqueue.NewClient(cfg.Redis, cfg.Queue)
	if err != nil {
		_ = resources.Close(context.Background())
		return nil, err
	}
	worker.queueClient = queueClient

	queueServer, err := taskqueue.NewServer(cfg.Redis, cfg.Queue)
	if err != nil {
		_ = queueClient.Close()
		_ = resources.Close(context.Background())
		return nil, err
	}
	worker.queueServer = queueServer
	worker.mux = taskqueue.NewMux()
	realtimePublisher := realtimePublisherForWorker(cfg, resources)
	notificationTaskService := notificationtask.NewService(
		notificationtask.NewGormRepository(resources.DB),
		notificationtask.WithEnqueuer(queueClient),
		notificationtask.WithRealtimePublisher(realtimePublisher),
		notificationtask.WithLogger(logger),
	)
	secretBox := providers.Secretbox
	exportTaskRepository := exporttask.NewGormRepository(resources.DB)
	userExportProvider := user.NewExportDataProvider(user.NewGormRepository(resources.DB))
	exportRegistry, err := exporttask.NewRegistry(exporttask.Definition{
		Kind:     exporttask.KindUserList,
		Title:    "用户列表",
		Provider: userExportProvider,
	})
	if err != nil {
		queueServer.Shutdown()
		_ = queueClient.Close()
		_ = resources.Close(context.Background())
		return nil, err
	}
	exportTaskService := exporttask.NewService(
		exportTaskRepository,
		exporttask.WithDefinitionRegistry(exportRegistry),
		exporttask.WithFileWriter(exporttask.XLSXWriter{}),
		exporttask.WithFileUploader(exporttask.NewCOSUploader(
			exporttask.NewGormUploadConfigRepository(resources.DB),
			secretBox,
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
		Secretbox:       secretBox,
		EngineFactory:   providers.AIChatFactory,
		RunRecorder:     aiRunRecorder,
		TextTasks:       aiTextTasks,
		RunStaleTimeout: positiveDuration(cfg.AI.RunStaleTimeout, config.DefaultAIRunStaleTimeout),
	})
	aiImageService := aiimage.NewService(aiimage.Dependencies{
		Repository:    aiimage.NewGormRepository(resources.DB),
		Secretbox:     secretBox,
		EngineFactory: providers.AIImageFactory,
		ObjectReader:  providers.ObjectReader,
		ObjectWriter:  providers.ObjectWriter,
		RunRecorder:   aiRunRecorder,
	})
	paymentService := paymentmodule.NewService(paymentmodule.Dependencies{
		Repository:   paymentmodule.NewGormRepository(resources.DB),
		Gateway:      providers.PaymentGateway,
		Secretbox:    secretBox,
		CertResolver: providers.PaymentCertResolver,
		CertStore:    providers.PaymentCertStore,
	})
	jobs.Register(worker.mux, jobs.Dependencies{
		Logger:                  logger,
		AIChatService:           aiChatService,
		AiImageService:          aiImageService,
		AuthRepository:          auth.NewGormRepository(resources.DB),
		ExportTaskService:       exportTaskService,
		NotificationTaskService: notificationTaskService,
		PaymentService:          paymentService,
	})

	if cfg.Scheduler.Enabled {
		schedulerOptions := []scheduler.Option{scheduler.WithLogger(logger)}
		if resources.Redis != nil && resources.Redis.Redis != nil {
			schedulerOptions = append(schedulerOptions, scheduler.WithLocker(redislock.New(resources.Redis.Redis)))
		}
		s, err := scheduler.New(cfg.Scheduler, schedulerOptions...)
		if err != nil {
			worker.queueServer.Shutdown()
			_ = queueClient.Close()
			_ = resources.Close(context.Background())
			return nil, err
		}
		worker.scheduler = s
		cronScheduler := crontask.NewSchedulerService(
			crontask.NewGormRepository(resources.DB),
			crontask.NewDefaultRegistry(),
			queueClient,
			logger,
		)
		if err := cronScheduler.RegisterEnabled(context.Background(), s); err != nil {
			_ = s.Shutdown(context.Background())
			worker.queueServer.Shutdown()
			_ = queueClient.Close()
			_ = resources.Close(context.Background())
			return nil, err
		}
		if err := jobs.RegisterSchedules(s, queueClient, logger); err != nil {
			_ = s.Shutdown(context.Background())
			worker.queueServer.Shutdown()
			_ = queueClient.Close()
			_ = resources.Close(context.Background())
			return nil, err
		}
	}

	return worker, nil
}

// Start starts queue consumption and the scheduler. It does not block.
func (w *Worker) Start(ctx context.Context) error {
	if w == nil {
		return nil
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	if !w.cfg.Queue.Enabled {
		w.logger.Info("admin worker queue disabled")
		return nil
	}
	if w.scheduler != nil {
		w.scheduler.Start()
		w.logger.Info("admin worker scheduler started", "timezone", w.cfg.Scheduler.Timezone)
	}

	w.logger.Info("starting admin worker", "queue_redis_db", w.cfg.Queue.RedisDB, "concurrency", w.cfg.Queue.Concurrency)
	return w.queueServer.Start(w.mux)
}

// Shutdown stops scheduler, queue consumer, producer, and shared resources.
func (w *Worker) Shutdown(ctx context.Context) error {
	if w == nil {
		return nil
	}

	var errs []error
	if w.scheduler != nil {
		if err := w.scheduler.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if w.queueServer != nil {
		w.queueServer.Shutdown()
	}
	if w.queueClient != nil {
		if err := w.queueClient.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if w.resources != nil {
		if err := w.resources.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func realtimePublisherForWorker(cfg config.Config, resources *Resources) infrarealtime.Publisher {
	realtimeConfig := withRealtimePolicyDefaults(cfg.Realtime)
	if !realtimeConfig.Enabled {
		return infrarealtime.NoopPublisher{}
	}
	publisherName := realtimeConfig.Publisher
	if publisherName == "" {
		publisherName = config.RealtimePublisherLocal
	}
	switch publisherName {
	case config.RealtimePublisherRedis:
		if resources == nil || resources.Redis == nil || resources.Redis.Redis == nil {
			return infrarealtime.NewRedisPublisher(nil, realtimeConfig.RedisChannel)
		}
		return infrarealtime.NewRedisPublisher(resources.Redis.Redis, realtimeConfig.RedisChannel)
	case config.RealtimePublisherNoop:
		return infrarealtime.NoopPublisher{}
	case config.RealtimePublisherLocal:
		// Worker has no WebSocket sessions. Local mode would be a fake cross-process
		// fan-out, so keep it explicitly disabled in the worker.
		return infrarealtime.NoopPublisher{}
	default:
		return infrarealtime.NoopPublisher{}
	}
}
