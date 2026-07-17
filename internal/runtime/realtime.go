package runtime

import (
	"log/slog"

	"admin_back_go/internal/config"
	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/infra/redisclient"
	modulerealtime "admin_back_go/internal/module/realtime"
	realtimeadmin "admin_back_go/internal/module/realtime/transport/admin"
	"admin_back_go/internal/telemetry"
)

type realtimeStack struct {
	enabled    bool
	manager    *infrarealtime.Manager
	publisher  infrarealtime.Publisher
	subscriber *infrarealtime.RedisSubscriber
	handler    *realtimeadmin.Handler
}

func newRealtimeStack(cfg config.RealtimeConfig, loggers ...*slog.Logger) realtimeStack {
	var logger *slog.Logger
	if len(loggers) > 0 {
		logger = loggers[0]
	}
	return newRealtimeStackWithRedis(cfg, nil, nil, logger)
}

func withRealtimePolicyDefaults(cfg config.RealtimeConfig) config.RealtimeConfig {
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = config.DefaultRealtimeHeartbeatInterval
	}
	if cfg.SendBuffer <= 0 {
		cfg.SendBuffer = config.DefaultRealtimeSendBuffer
	}
	if cfg.RedisChannel == "" {
		cfg.RedisChannel = config.DefaultRealtimeRedisChannel
	}
	return cfg
}

func newRealtimeStackWithRedis(cfg config.RealtimeConfig, allowedOrigins []string, redis *redisclient.Client, logger *slog.Logger, recorders ...telemetry.Recorder) realtimeStack {
	if logger == nil {
		logger = slog.Default()
	}
	recorder := telemetry.Noop()
	if len(recorders) > 0 && recorders[0] != nil {
		recorder = recorders[0]
	}
	cfg = withRealtimePolicyDefaults(cfg)

	enabled := realtimeEnabledFor(cfg, logger)
	manager := infrarealtime.NewManager(infrarealtime.WithTelemetry(recorder))
	localPublisher := infrarealtime.NewLocalPublisher(manager)
	publisher, subscriber := realtimePublisherFor(cfg, enabled, redis, localPublisher, logger, recorders...)
	service := modulerealtime.NewService(cfg.HeartbeatInterval)
	handler := realtimeadmin.NewHandler(
		service,
		infrarealtime.NewUpgrader(infrarealtime.NewAllowedOriginChecker(allowedOrigins)),
		manager,
		logger,
		realtimeadmin.WithEnabled(enabled),
		realtimeadmin.WithSendBuffer(cfg.SendBuffer),
	)

	return realtimeStack{
		enabled:    enabled,
		manager:    manager,
		publisher:  publisher,
		subscriber: subscriber,
		handler:    handler,
	}
}

func realtimeEnabledFor(cfg config.RealtimeConfig, logger *slog.Logger) bool {
	if !cfg.Enabled {
		return false
	}
	publisherName := cfg.Publisher
	if publisherName == "" {
		publisherName = config.RealtimePublisherLocal
	}
	switch publisherName {
	case config.RealtimePublisherLocal, config.RealtimePublisherNoop, config.RealtimePublisherRedis:
		return true
	default:
		if logger != nil {
			logger.Error("unknown realtime publisher; websocket upgrades disabled", "publisher", cfg.Publisher)
		}
		return false
	}
}

func realtimePublisherFor(
	cfg config.RealtimeConfig,
	enabled bool,
	redis *redisclient.Client,
	localPublisher *infrarealtime.LocalPublisher,
	logger *slog.Logger,
	recorders ...telemetry.Recorder,
) (infrarealtime.Publisher, *infrarealtime.RedisSubscriber) {
	recorder := telemetry.Noop()
	instrument := len(recorders) > 0 && recorders[0] != nil
	if instrument {
		recorder = recorders[0]
	}
	wrap := func(publisher infrarealtime.Publisher, transport string) infrarealtime.Publisher {
		if !instrument {
			return publisher
		}
		return infrarealtime.InstrumentPublisher(publisher, transport, recorder)
	}
	if !enabled {
		return infrarealtime.NoopPublisher{}, nil
	}

	publisherName := cfg.Publisher
	if publisherName == "" {
		publisherName = config.RealtimePublisherLocal
	}
	switch publisherName {
	case config.RealtimePublisherLocal:
		return wrap(localPublisher, "local"), nil
	case config.RealtimePublisherNoop:
		return wrap(infrarealtime.NoopPublisher{}, "noop"), nil
	case config.RealtimePublisherRedis:
		local := wrap(localPublisher, "local")
		if redis == nil || redis.Redis == nil {
			if logger != nil {
				logger.Error("realtime redis publisher selected but redis client is not ready")
			}
			publisher := wrap(infrarealtime.NewRedisPublisher(nil, cfg.RedisChannel), "redis")
			return publisher, infrarealtime.NewRedisSubscriber(nil, cfg.RedisChannel, local, logger)
		}
		publisher := wrap(infrarealtime.NewRedisPublisher(redis.Redis, cfg.RedisChannel), "redis")
		return publisher, infrarealtime.NewRedisSubscriber(redis.Redis, cfg.RedisChannel, local, logger)
	default:
		if logger != nil {
			logger.Error("unknown realtime publisher; realtime publication disabled", "publisher", cfg.Publisher)
		}
		return infrarealtime.NoopPublisher{}, nil
	}
}

func realtimePublisherForWorker(cfg config.Config, resources *Resources, recorders ...telemetry.Recorder) infrarealtime.Publisher {
	recorder := telemetry.Noop()
	instrument := len(recorders) > 0 && recorders[0] != nil
	if instrument {
		recorder = recorders[0]
	}
	wrap := func(publisher infrarealtime.Publisher, transport string) infrarealtime.Publisher {
		if !instrument {
			return publisher
		}
		return infrarealtime.InstrumentPublisher(publisher, transport, recorder)
	}
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
			return wrap(infrarealtime.NewRedisPublisher(nil, realtimeConfig.RedisChannel), "redis")
		}
		return wrap(infrarealtime.NewRedisPublisher(resources.Redis.Redis, realtimeConfig.RedisChannel), "redis")
	case config.RealtimePublisherNoop, config.RealtimePublisherLocal:
		return wrap(infrarealtime.NoopPublisher{}, "noop")
	default:
		return infrarealtime.NoopPublisher{}
	}
}
