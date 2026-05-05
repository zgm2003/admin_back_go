package bootstrap

import (
	"log/slog"
	"testing"
	"time"

	"admin_back_go/internal/config"
	platformrealtime "admin_back_go/internal/platform/realtime"
)

func TestNewWorkerAllowsQueueDisabledWithoutRedis(t *testing.T) {
	worker, err := NewWorker(config.Config{
		Queue: config.QueueConfig{Enabled: false},
	}, slog.Default())
	if err != nil {
		t.Fatalf("expected disabled queue worker to build without redis, got %v", err)
	}
	defer worker.Shutdown(t.Context())

	if worker.queueServer != nil {
		t.Fatalf("expected nil queue server when queue is disabled")
	}
	if worker.queueClient != nil {
		t.Fatalf("expected nil queue client when queue is disabled")
	}
}

func TestNewWorkerRejectsQueueEnabledWithoutRedis(t *testing.T) {
	worker, err := NewWorker(config.Config{
		Queue: config.QueueConfig{
			Enabled:         true,
			DefaultQueue:    "default",
			DefaultMaxRetry: 3,
			DefaultTimeout:  30 * time.Second,
		},
	}, slog.Default())
	if err == nil {
		t.Fatalf("expected enabled queue without redis to fail")
	}
	if worker != nil {
		t.Fatalf("expected nil worker on error")
	}
}

func TestNewWorkerBuildsQueueAndSchedulerWithoutPingingRedis(t *testing.T) {
	worker, err := NewWorker(config.Config{
		Redis: config.RedisConfig{
			Addr:     "127.0.0.1:1",
			Password: "secret",
			DB:       0,
		},
		Queue: config.QueueConfig{
			Enabled:         true,
			RedisDB:         3,
			Concurrency:     2,
			DefaultQueue:    "default",
			CriticalWeight:  6,
			DefaultWeight:   3,
			LowWeight:       1,
			ShutdownTimeout: time.Second,
			DefaultMaxRetry: 3,
			DefaultTimeout:  30 * time.Second,
		},
		Scheduler: config.SchedulerConfig{
			Enabled:  true,
			Timezone: "UTC",
		},
	}, slog.Default())
	if err != nil {
		t.Fatalf("expected worker to build without pinging redis, got %v", err)
	}
	defer worker.Shutdown(t.Context())

	if worker.queueServer == nil {
		t.Fatalf("expected queue server")
	}
	if worker.queueClient == nil {
		t.Fatalf("expected queue client")
	}
	if worker.scheduler == nil {
		t.Fatalf("expected scheduler")
	}
}

func TestRealtimePublisherForWorkerUsesRedisOnlyForCrossProcessFanout(t *testing.T) {
	workerPublisher := realtimePublisherForWorker(config.Config{
		Realtime: config.RealtimeConfig{Enabled: true, Publisher: config.RealtimePublisherRedis, RedisChannel: "admin_go:realtime:test"},
	}, &Resources{})
	if _, ok := workerPublisher.(*platformrealtime.RedisPublisher); !ok {
		t.Fatalf("expected worker redis publisher, got %T", workerPublisher)
	}
}

func TestRealtimePublisherForWorkerDoesNotFakeLocalDelivery(t *testing.T) {
	workerPublisher := realtimePublisherForWorker(config.Config{
		Realtime: config.RealtimeConfig{Enabled: true, Publisher: config.RealtimePublisherLocal},
	}, &Resources{})
	if _, ok := workerPublisher.(platformrealtime.NoopPublisher); !ok {
		t.Fatalf("expected worker local mode to stay noop, got %T", workerPublisher)
	}
}
