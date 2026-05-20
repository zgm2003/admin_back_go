package bootstrap

import (
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"admin_back_go/internal/config"
	platformrealtime "admin_back_go/internal/platform/realtime"
)

func TestNewWorkerAllowsQueueDisabledWithoutRedis(t *testing.T) {
	worker, err := NewWorker(config.Config{
		App:   config.AppConfig{Secret: strings.Repeat("a", 64)},
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
		App: config.AppConfig{Secret: strings.Repeat("a", 64)},
		Queue: config.QueueConfig{
			Enabled: true,
		},
	}, slog.Default())
	if err == nil {
		t.Fatalf("expected enabled queue without redis to fail")
	}
	if worker != nil {
		t.Fatalf("expected nil worker on error")
	}
}

func TestNewWorkerBuildsQueueWithoutSchedulerOrRedisPing(t *testing.T) {
	worker, err := NewWorker(config.Config{
		App: config.AppConfig{Secret: strings.Repeat("a", 64)},
		Redis: config.RedisConfig{
			Addr:     "127.0.0.1:1",
			Password: "secret",
			DB:       0,
		},
		Queue: config.QueueConfig{
			Enabled:     true,
			RedisDB:     3,
			Concurrency: 2,
		},
		Scheduler: config.SchedulerConfig{Enabled: false},
	}, slog.Default())
	if err != nil {
		t.Fatalf("expected worker queue to build without pinging redis, got %v", err)
	}
	defer worker.Shutdown(t.Context())

	if worker.queueServer == nil {
		t.Fatalf("expected queue server")
	}
	if worker.queueClient == nil {
		t.Fatalf("expected queue client")
	}
	if worker.scheduler != nil {
		t.Fatalf("expected nil scheduler when scheduler disabled")
	}
}

func TestNewWorkerRejectsSchedulerEnabledWithoutDatabase(t *testing.T) {
	worker, err := NewWorker(config.Config{
		App:   config.AppConfig{Secret: strings.Repeat("a", 64)},
		Redis: config.RedisConfig{Addr: "127.0.0.1:1"},
		Queue: config.QueueConfig{
			Enabled:     true,
			RedisDB:     3,
			Concurrency: 2,
		},
		Scheduler: config.SchedulerConfig{Enabled: true, Timezone: "UTC"},
	}, slog.Default())
	if err == nil {
		t.Fatalf("expected scheduler enabled without database to fail")
	}
	if worker != nil {
		t.Fatalf("expected nil worker on scheduler configuration error")
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

func TestRealtimePublisherForWorkerUsesCodeOwnedDefaultChannel(t *testing.T) {
	workerPublisher := realtimePublisherForWorker(config.Config{
		Realtime: config.RealtimeConfig{Enabled: true, Publisher: config.RealtimePublisherRedis},
	}, &Resources{})

	if _, ok := workerPublisher.(*platformrealtime.RedisPublisher); !ok {
		t.Fatalf("expected worker redis publisher, got %T", workerPublisher)
	}
	if got := realtimePublisherChannelFromWorkerTest(t, workerPublisher); got != config.DefaultRealtimeRedisChannel {
		t.Fatalf("expected worker redis publisher channel %q, got %q", config.DefaultRealtimeRedisChannel, got)
	}
}

func realtimePublisherChannelFromWorkerTest(t *testing.T, publisher platformrealtime.Publisher) string {
	t.Helper()
	value := reflect.ValueOf(publisher)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		t.Fatalf("expected pointer publisher, got %T", publisher)
	}
	field := value.Elem().FieldByName("channel")
	if !field.IsValid() {
		t.Fatalf("publisher %T has no channel field", publisher)
	}
	return field.String()
}

func TestRealtimePublisherForWorkerDoesNotFakeLocalDelivery(t *testing.T) {
	workerPublisher := realtimePublisherForWorker(config.Config{
		Realtime: config.RealtimeConfig{Enabled: true, Publisher: config.RealtimePublisherLocal},
	}, &Resources{})
	if _, ok := workerPublisher.(platformrealtime.NoopPublisher); !ok {
		t.Fatalf("expected worker local mode to stay noop, got %T", workerPublisher)
	}
}
