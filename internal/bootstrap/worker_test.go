package bootstrap

import (
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"admin_back_go/internal/config"
	infrarealtime "admin_back_go/internal/infra/realtime"
)

func TestNewWorkerAllowsQueueDisabledWithoutRedis(t *testing.T) {
	worker, err := NewWorker(config.Config{
		App:   config.AppConfig{Secret: strings.Repeat("a", 64)},
		Queue: config.QueueConfig{Enabled: false},
	}, slog.Default())
	if err != nil {
		t.Fatalf("expected disabled queue worker to build without redis, got %v", err)
	}
	defer shutdownWorkerForTest(t, worker)

	if worker.queueServer != nil {
		t.Fatalf("expected nil queue server when queue is disabled")
	}
	if worker.queueClient != nil {
		t.Fatalf("expected nil queue client when queue is disabled")
	}
}

func TestNewWorkerNormalizesSchedulerPolicyDefaults(t *testing.T) {
	worker, err := NewWorker(config.Config{
		App:       config.AppConfig{Secret: strings.Repeat("a", 64)},
		Queue:     config.QueueConfig{Enabled: false},
		Scheduler: config.SchedulerConfig{Enabled: true},
	}, slog.Default())
	if err != nil {
		t.Fatalf("expected worker to build, got %v", err)
	}
	defer shutdownWorkerForTest(t, worker)

	if worker.cfg.Scheduler.Timezone != config.DefaultSchedulerTimezone {
		t.Fatalf("expected worker scheduler timezone %q, got %q", config.DefaultSchedulerTimezone, worker.cfg.Scheduler.Timezone)
	}
	if worker.cfg.Scheduler.LockPrefix != config.DefaultSchedulerLockPrefix {
		t.Fatalf("expected worker scheduler lock prefix %q, got %q", config.DefaultSchedulerLockPrefix, worker.cfg.Scheduler.LockPrefix)
	}
	if worker.cfg.Scheduler.LockTTL != config.DefaultSchedulerLockTTL {
		t.Fatalf("expected worker scheduler lock ttl %s, got %s", config.DefaultSchedulerLockTTL, worker.cfg.Scheduler.LockTTL)
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

func TestNewWorkerRejectsQueueWhenRequiredDatabaseIsMissing(t *testing.T) {
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
	if err == nil {
		t.Fatal("expected worker resource validation to fail")
	}
	if worker != nil {
		t.Fatalf("partial worker published: %+v", worker)
	}
}

func shutdownWorkerForTest(t *testing.T, worker *Worker) {
	t.Helper()
	if worker == nil {
		return
	}
	if err := worker.Shutdown(t.Context()); err != nil {
		t.Fatalf("worker shutdown: %v", err)
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
	if _, ok := workerPublisher.(*infrarealtime.RedisPublisher); !ok {
		t.Fatalf("expected worker redis publisher, got %T", workerPublisher)
	}
}

func TestRealtimePublisherForWorkerUsesCodeOwnedDefaultChannel(t *testing.T) {
	workerPublisher := realtimePublisherForWorker(config.Config{
		Realtime: config.RealtimeConfig{Enabled: true, Publisher: config.RealtimePublisherRedis},
	}, &Resources{})

	if _, ok := workerPublisher.(*infrarealtime.RedisPublisher); !ok {
		t.Fatalf("expected worker redis publisher, got %T", workerPublisher)
	}
	if got := realtimePublisherChannelFromWorkerTest(t, workerPublisher); got != config.DefaultRealtimeRedisChannel {
		t.Fatalf("expected worker redis publisher channel %q, got %q", config.DefaultRealtimeRedisChannel, got)
	}
}

func realtimePublisherChannelFromWorkerTest(t *testing.T, publisher infrarealtime.Publisher) string {
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
	if _, ok := workerPublisher.(infrarealtime.NoopPublisher); !ok {
		t.Fatalf("expected worker local mode to stay noop, got %T", workerPublisher)
	}
}
