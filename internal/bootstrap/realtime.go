package bootstrap

import (
	"log/slog"

	"admin_back_go/internal/config"
	modulerealtime "admin_back_go/internal/module/realtime"
	platformrealtime "admin_back_go/internal/platform/realtime"
	"admin_back_go/internal/platform/redisclient"
)

type realtimeStack struct {
	enabled    bool
	manager    *platformrealtime.Manager
	publisher  platformrealtime.Publisher
	subscriber *platformrealtime.RedisSubscriber
	handler    *modulerealtime.Handler
}

func newRealtimeStack(cfg config.RealtimeConfig, loggers ...*slog.Logger) realtimeStack {
	return newRealtimeStackWithRedis(cfg, nil, nil, loggers...)
}

func newRealtimeStackWithRedis(cfg config.RealtimeConfig, allowedOrigins []string, redis *redisclient.Client, loggers ...*slog.Logger) realtimeStack {
	logger := slog.Default()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}

	enabled := realtimeEnabledFor(cfg, logger)
	manager := platformrealtime.NewManager()
	localPublisher := platformrealtime.NewLocalPublisher(manager)
	publisher, subscriber := realtimePublisherFor(cfg, enabled, redis, localPublisher, logger)
	service := modulerealtime.NewService(cfg.HeartbeatInterval)
	handler := modulerealtime.NewHandler(
		service,
		platformrealtime.NewUpgrader(platformrealtime.NewAllowedOriginChecker(allowedOrigins)),
		manager,
		logger,
		modulerealtime.WithEnabled(enabled),
		modulerealtime.WithSendBuffer(cfg.SendBuffer),
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
	case "", config.RealtimePublisherLocal, config.RealtimePublisherNoop, config.RealtimePublisherRedis:
		return true
	default:
		if logger != nil {
			logger.Error("unknown realtime publisher; websocket upgrades disabled", "publisher", cfg.Publisher)
		}
		return false
	}
}

func realtimePublisherFor(cfg config.RealtimeConfig, enabled bool, redis *redisclient.Client, localPublisher *platformrealtime.LocalPublisher, logger *slog.Logger) (platformrealtime.Publisher, *platformrealtime.RedisSubscriber) {
	if !enabled {
		return platformrealtime.NoopPublisher{}, nil
	}

	publisherName := cfg.Publisher
	if publisherName == "" {
		publisherName = config.RealtimePublisherLocal
	}
	switch publisherName {
	case "", config.RealtimePublisherLocal:
		return localPublisher, nil
	case config.RealtimePublisherNoop:
		return platformrealtime.NoopPublisher{}, nil
	case config.RealtimePublisherRedis:
		if redis == nil || redis.Redis == nil {
			if logger != nil {
				logger.Error("realtime redis publisher selected but redis client is not ready")
			}
			return platformrealtime.NewRedisPublisher(nil, cfg.RedisChannel), platformrealtime.NewRedisSubscriber(nil, cfg.RedisChannel, localPublisher, logger)
		}
		return platformrealtime.NewRedisPublisher(redis.Redis, cfg.RedisChannel), platformrealtime.NewRedisSubscriber(redis.Redis, cfg.RedisChannel, localPublisher, logger)
	default:
		if logger != nil {
			logger.Error("unknown realtime publisher; realtime publication disabled", "publisher", cfg.Publisher)
		}
		return platformrealtime.NoopPublisher{}, nil
	}
}
