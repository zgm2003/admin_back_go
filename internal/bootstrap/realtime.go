package bootstrap

import (
	"log/slog"

	"admin_back_go/internal/config"
	modulerealtime "admin_back_go/internal/module/realtime"
	platformrealtime "admin_back_go/internal/platform/realtime"
)

type realtimeStack struct {
	enabled   bool
	manager   *platformrealtime.Manager
	publisher platformrealtime.Publisher
	handler   *modulerealtime.Handler
}

func newRealtimeStack(cfg config.RealtimeConfig, loggers ...*slog.Logger) realtimeStack {
	logger := slog.Default()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}

	enabled := realtimeEnabledFor(cfg, logger)
	manager := platformrealtime.NewManager()
	publisher := realtimePublisherFor(cfg, enabled, manager, logger)
	service := modulerealtime.NewService(cfg.HeartbeatInterval)
	handler := modulerealtime.NewHandler(
		service,
		platformrealtime.NewUpgrader(nil),
		manager,
		logger,
		modulerealtime.WithEnabled(enabled),
		modulerealtime.WithSendBuffer(cfg.SendBuffer),
	)

	return realtimeStack{
		enabled:   enabled,
		manager:   manager,
		publisher: publisher,
		handler:   handler,
	}
}

func realtimeEnabledFor(cfg config.RealtimeConfig, logger *slog.Logger) bool {
	if !cfg.Enabled {
		return false
	}
	switch cfg.Publisher {
	case "", config.RealtimePublisherLocal, config.RealtimePublisherNoop:
		return true
	default:
		if logger != nil {
			logger.Error("unknown realtime publisher; websocket upgrades disabled", "publisher", cfg.Publisher)
		}
		return false
	}
}

func realtimePublisherFor(cfg config.RealtimeConfig, enabled bool, manager *platformrealtime.Manager, logger *slog.Logger) platformrealtime.Publisher {
	if !enabled {
		return platformrealtime.NoopPublisher{}
	}

	switch cfg.Publisher {
	case "", config.RealtimePublisherLocal:
		return platformrealtime.NewLocalPublisher(manager)
	case config.RealtimePublisherNoop:
		return platformrealtime.NoopPublisher{}
	default:
		if logger != nil {
			logger.Error("unknown realtime publisher; realtime publication disabled", "publisher", cfg.Publisher)
		}
		return platformrealtime.NoopPublisher{}
	}
}
