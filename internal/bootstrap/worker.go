package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/jobs"
	"admin_back_go/internal/module/aichat"
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/crontask"
	"admin_back_go/internal/module/exporttask"
	"admin_back_go/internal/module/notificationtask"
	paymentmodule "admin_back_go/internal/module/payment"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/platform/payment"
	payalipay "admin_back_go/internal/platform/payment/alipay"
	platformrealtime "admin_back_go/internal/platform/realtime"
	"admin_back_go/internal/platform/redislock"
	"admin_back_go/internal/platform/scheduler"
	"admin_back_go/internal/platform/secretbox"
	"admin_back_go/internal/platform/secretkey"
	storagecos "admin_back_go/internal/platform/storage/cos"
	"admin_back_go/internal/platform/taskqueue"
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

	resources, err := NewResources(cfg)
	if err != nil {
		if resources != nil {
			_ = resources.Close()
		}
		return nil, err
	}
	worker.resources = resources

	queueClient, err := taskqueue.NewClient(cfg.Redis, cfg.Queue)
	if err != nil {
		_ = resources.Close()
		return nil, err
	}
	worker.queueClient = queueClient

	queueServer, err := taskqueue.NewServer(cfg.Redis, cfg.Queue)
	if err != nil {
		_ = queueClient.Close()
		_ = resources.Close()
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
	secretBox := secretbox.New(keys.SecretboxKey())
	exportTaskRepository := exporttask.NewGormRepository(resources.DB)
	exportTaskService := exporttask.NewService(
		exportTaskRepository,
		exporttask.WithExportDataProvider(user.NewExportDataProvider(user.NewGormRepository(resources.DB))),
		exporttask.WithFileWriter(exporttask.XLSXWriter{}),
		exporttask.WithFileUploader(exporttask.NewCOSUploader(
			exporttask.NewGormUploadConfigRepository(resources.DB),
			secretBox,
			storagecos.NewObjectWriter(storagecos.ObjectWriterConfig{Enabled: cfg.UploadToken.COS.Enabled}),
		)),
		exporttask.WithNotifier(exporttask.NewNotificationTaskNotifier(notificationTaskService)),
		exporttask.WithLogger(logger),
	)
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
	aiChatService := aichat.NewService(aichat.Dependencies{
		Repository:      aichat.NewGormRepository(resources.DB),
		Publisher:       realtimePublisher,
		Secretbox:       secretBox,
		EngineFactory:   aiChatEngineFactory{streamIdleTimeout: positiveDuration(cfg.AI.ChatStreamIdleTimeout, 60*time.Second)},
		RunStaleTimeout: positiveDuration(cfg.AI.RunStaleTimeout, 15*time.Minute),
	})
	jobs.Register(worker.mux, jobs.Dependencies{
		Logger:                  logger,
		AIChatService:           aiChatService,
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
			_ = resources.Close()
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
			_ = resources.Close()
			return nil, err
		}
		if err := jobs.RegisterSchedules(s, queueClient, logger); err != nil {
			_ = s.Shutdown(context.Background())
			worker.queueServer.Shutdown()
			_ = queueClient.Close()
			_ = resources.Close()
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
		if err := w.resources.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func realtimePublisherForWorker(cfg config.Config, resources *Resources) platformrealtime.Publisher {
	if !cfg.Realtime.Enabled {
		return platformrealtime.NoopPublisher{}
	}
	publisherName := cfg.Realtime.Publisher
	if publisherName == "" {
		publisherName = config.RealtimePublisherLocal
	}
	switch publisherName {
	case config.RealtimePublisherRedis:
		if resources == nil || resources.Redis == nil || resources.Redis.Redis == nil {
			return platformrealtime.NewRedisPublisher(nil, cfg.Realtime.RedisChannel)
		}
		return platformrealtime.NewRedisPublisher(resources.Redis.Redis, cfg.Realtime.RedisChannel)
	case config.RealtimePublisherNoop:
		return platformrealtime.NoopPublisher{}
	case config.RealtimePublisherLocal:
		// Worker has no WebSocket sessions. Local mode would be a fake cross-process
		// fan-out, so keep it explicitly disabled in the worker.
		return platformrealtime.NoopPublisher{}
	default:
		return platformrealtime.NoopPublisher{}
	}
}
