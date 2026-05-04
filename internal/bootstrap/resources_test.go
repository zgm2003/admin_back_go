package bootstrap

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/config"
)

func TestNewResourcesAllowsEmptyMySQLDSN(t *testing.T) {
	resources, err := NewResources(config.Config{
		Redis: config.RedisConfig{Addr: ""},
	})
	if err != nil {
		t.Fatalf("expected empty mysql dsn to be allowed, got %v", err)
	}
	defer resources.Close()

	if resources.DB != nil {
		t.Fatalf("expected nil db resource when mysql dsn is empty")
	}
	if resources.Redis != nil {
		t.Fatalf("expected nil redis resource when redis addr is empty")
	}
}

func TestNewResourcesCreatesConfiguredClientsWithoutLiveServices(t *testing.T) {
	resources, err := NewResources(config.Config{
		MySQL: config.MySQLConfig{
			DSN:             "user:pass@tcp(127.0.0.1:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local",
			MaxOpenConns:    9,
			MaxIdleConns:    4,
			ConnMaxLifetime: 10 * time.Minute,
		},
		Redis: config.RedisConfig{
			Addr:     "127.0.0.1:6380",
			Password: "secret",
			DB:       3,
		},
		Token: config.TokenConfig{
			RedisDB: 2,
		},
	})
	if err != nil {
		t.Fatalf("expected resources to build without pinging live services, got %v", err)
	}
	defer resources.Close()

	if resources.DB == nil || resources.DB.SQL == nil {
		t.Fatalf("expected db resource")
	}
	if got := resources.DB.SQL.Stats().MaxOpenConnections; got != 9 {
		t.Fatalf("expected db max open connections 9, got %d", got)
	}
	if resources.Redis == nil || resources.Redis.Redis == nil {
		t.Fatalf("expected redis resource")
	}
	if got := resources.Redis.Redis.Options().DB; got != 3 {
		t.Fatalf("expected redis db 3, got %d", got)
	}
	if resources.TokenRedis == nil || resources.TokenRedis.Redis == nil {
		t.Fatalf("expected token redis resource")
	}
	if got := resources.TokenRedis.Redis.Options().DB; got != 2 {
		t.Fatalf("expected token redis db 2, got %d", got)
	}
	if resources.QueueRedis != nil {
		t.Fatalf("expected queue redis to stay disabled when queue config is empty")
	}
}

func TestResourcesCloseIsSafeOnNil(t *testing.T) {
	var resources *Resources
	if err := resources.Close(); err != nil {
		t.Fatalf("expected nil resources close to be safe, got %v", err)
	}
}

func TestResourcesReadinessReportsDisabledWhenResourcesAreNotConfigured(t *testing.T) {
	resources, err := NewResources(config.Config{})
	if err != nil {
		t.Fatalf("expected empty resources to build, got %v", err)
	}
	defer resources.Close()

	report := resources.Readiness(t.Context())

	if report.Status != "ready" {
		t.Fatalf("expected ready when optional resources are disabled, got %q", report.Status)
	}
	if report.Checks["database"].Status != "disabled" {
		t.Fatalf("expected database disabled, got %#v", report.Checks["database"])
	}
	if report.Checks["redis"].Status != "disabled" {
		t.Fatalf("expected redis disabled, got %#v", report.Checks["redis"])
	}
	if report.Checks["token_redis"].Status != "disabled" {
		t.Fatalf("expected token_redis disabled, got %#v", report.Checks["token_redis"])
	}
	if report.Checks["queue_redis"].Status != "disabled" {
		t.Fatalf("expected queue_redis disabled, got %#v", report.Checks["queue_redis"])
	}
	if report.Checks["realtime"].Status != "disabled" {
		t.Fatalf("expected realtime disabled, got %#v", report.Checks["realtime"])
	}
}

func TestResourcesReadinessReportsDownWhenConfiguredResourcesCannotPing(t *testing.T) {
	resources, err := NewResources(config.Config{
		MySQL: config.MySQLConfig{
			DSN:             "user:pass@tcp(127.0.0.1:1)/admin?charset=utf8mb4&parseTime=True&loc=Local",
			MaxOpenConns:    1,
			MaxIdleConns:    1,
			ConnMaxLifetime: time.Minute,
		},
		Redis: config.RedisConfig{Addr: "127.0.0.1:1"},
	})
	if err != nil {
		t.Fatalf("expected resources to build without pinging, got %v", err)
	}
	defer resources.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	report := resources.Readiness(ctx)

	if report.Status != "not_ready" {
		t.Fatalf("expected not_ready when configured resources are down, got %q", report.Status)
	}
	if report.Checks["database"].Status != "down" {
		t.Fatalf("expected database down, got %#v", report.Checks["database"])
	}
	if report.Checks["redis"].Status != "down" {
		t.Fatalf("expected redis down, got %#v", report.Checks["redis"])
	}
}

func TestResourcesReadinessReportsQueueRedisAndRealtimeConfig(t *testing.T) {
	resources, err := NewResources(config.Config{
		Redis: config.RedisConfig{Addr: "127.0.0.1:6380", DB: 0},
		Token: config.TokenConfig{RedisDB: 2},
		Queue: config.QueueConfig{Enabled: true, RedisDB: 3},
		Realtime: config.RealtimeConfig{
			Enabled:           true,
			Publisher:         config.RealtimePublisherLocal,
			HeartbeatInterval: 25 * time.Second,
			SendBuffer:        16,
		},
	})
	if err != nil {
		t.Fatalf("expected resources to build without pinging live services, got %v", err)
	}
	defer resources.Close()

	report := resources.Readiness(t.Context())

	if _, ok := report.Checks["queue_redis"]; !ok {
		t.Fatalf("expected queue_redis readiness check, got %#v", report.Checks)
	}
	if resources.QueueRedis == nil || resources.QueueRedis.Redis == nil {
		t.Fatalf("expected queue redis resource")
	}
	if got := resources.QueueRedis.Redis.Options().DB; got != 3 {
		t.Fatalf("expected queue redis db 3, got %d", got)
	}
	if got := report.Checks["realtime"].Status; got != "up" {
		t.Fatalf("expected realtime up, got %#v", report.Checks["realtime"])
	}
}

func TestResourcesReadinessReportsQueueDisabledSeparatelyFromRedis(t *testing.T) {
	resources, err := NewResources(config.Config{
		Redis: config.RedisConfig{Addr: "127.0.0.1:6380", DB: 0},
		Token: config.TokenConfig{RedisDB: 2},
		Queue: config.QueueConfig{Enabled: false, RedisDB: 3},
		Realtime: config.RealtimeConfig{
			Enabled:   false,
			Publisher: config.RealtimePublisherLocal,
		},
	})
	if err != nil {
		t.Fatalf("expected resources to build without pinging live services, got %v", err)
	}
	defer resources.Close()

	report := resources.Readiness(t.Context())

	if got := report.Checks["queue_redis"].Status; got != "disabled" {
		t.Fatalf("expected queue_redis disabled, got %#v", report.Checks["queue_redis"])
	}
	if got := report.Checks["realtime"].Status; got != "disabled" {
		t.Fatalf("expected realtime disabled, got %#v", report.Checks["realtime"])
	}
}

func TestResourcesReadinessReportsDownWhenQueueEnabledWithoutRedisAddr(t *testing.T) {
	resources, err := NewResources(config.Config{
		Redis: config.RedisConfig{Addr: ""},
		Queue: config.QueueConfig{Enabled: true, RedisDB: 3},
	})
	if err != nil {
		t.Fatalf("expected resources to build, got %v", err)
	}
	defer resources.Close()

	report := resources.Readiness(t.Context())

	if report.Status != "not_ready" {
		t.Fatalf("expected not_ready when queue is enabled without redis addr, got %q", report.Status)
	}
	if got := report.Checks["queue_redis"]; got.Status != "down" {
		t.Fatalf("expected queue_redis down, got %#v", got)
	}
}

func TestResourcesReadinessFailsOnUnknownRealtimePublisher(t *testing.T) {
	resources, err := NewResources(config.Config{
		Realtime: config.RealtimeConfig{
			Enabled:   true,
			Publisher: "redis",
		},
	})
	if err != nil {
		t.Fatalf("expected resources to build, got %v", err)
	}
	defer resources.Close()

	report := resources.Readiness(t.Context())

	if report.Status != "not_ready" {
		t.Fatalf("expected not_ready for unsupported realtime publisher, got %q", report.Status)
	}
	if got := report.Checks["realtime"]; got.Status != "down" {
		t.Fatalf("expected realtime down, got %#v", got)
	}
}
