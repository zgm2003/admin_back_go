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
