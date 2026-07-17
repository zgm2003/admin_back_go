package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/redisclient"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/telemetry"
)

type OpenedResource[T any] struct {
	Client T
	Ping   func(context.Context) error
	Close  func(context.Context) error
}

type DatabaseOpener func(context.Context, config.MySQLConfig) (OpenedResource[*database.Client], error)
type RedisOpener func(context.Context, config.RedisConfig) (OpenedResource[*redisclient.Client], error)

type Openers struct {
	Database   DatabaseOpener
	Redis      RedisOpener
	TokenRedis RedisOpener
	QueueRedis RedisOpener
	Telemetry  telemetry.Recorder
}

type resourceCapabilities struct {
	database   bool
	redis      bool
	tokenRedis bool
	queueRedis bool
	realtime   bool
	scheduler  bool
}

type Resources struct {
	DB         *database.Client
	Redis      *redisclient.Client
	TokenRedis *redisclient.Client
	QueueRedis *redisclient.Client

	cleanup      *Cleanup
	capabilities resourceCapabilities
	pings        map[string]func(context.Context) error
}

func OpenResources(ctx context.Context, process config.Process, cfg config.Config, open Openers) (*Resources, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	capabilities, err := capabilitiesFor(process, cfg)
	if err != nil {
		return nil, err
	}
	open = open.withDefaults()
	cleanup := NewCleanup()
	pings := make(map[string]func(context.Context) error, 4)

	var db *database.Client
	if capabilities.database {
		opened, openErr := open.Database(ctx, cfg.MySQL)
		if openErr != nil {
			return nil, failResourceOpen(ctx, cleanup, "database", openErr)
		}
		if activateErr := activateResource(ctx, cleanup, pings, "database", opened.Client != nil, opened.Ping, opened.Close); activateErr != nil {
			return nil, activateErr
		}
		db = opened.Client
	}

	var redis *redisclient.Client
	if capabilities.redis {
		opened, openErr := open.Redis(ctx, cfg.Redis)
		if openErr != nil {
			return nil, failResourceOpen(ctx, cleanup, "redis", openErr)
		}
		if activateErr := activateResource(ctx, cleanup, pings, "redis", opened.Client != nil, opened.Ping, opened.Close); activateErr != nil {
			return nil, activateErr
		}
		redis = opened.Client
	}

	var tokenRedis *redisclient.Client
	if capabilities.tokenRedis {
		tokenCfg := cfg.Redis
		tokenCfg.DB = cfg.Token.RedisDB
		opened, openErr := open.TokenRedis(ctx, tokenCfg)
		if openErr != nil {
			return nil, failResourceOpen(ctx, cleanup, "token redis", openErr)
		}
		if activateErr := activateResource(ctx, cleanup, pings, "token_redis", opened.Client != nil, opened.Ping, opened.Close); activateErr != nil {
			return nil, activateErr
		}
		tokenRedis = opened.Client
	}

	var queueRedis *redisclient.Client
	if capabilities.queueRedis {
		queueCfg := cfg.Redis
		queueCfg.DB = cfg.Queue.RedisDB
		opened, openErr := open.QueueRedis(ctx, queueCfg)
		if openErr != nil {
			return nil, failResourceOpen(ctx, cleanup, "queue redis", openErr)
		}
		if activateErr := activateResource(ctx, cleanup, pings, "queue_redis", opened.Client != nil, opened.Ping, opened.Close); activateErr != nil {
			return nil, activateErr
		}
		queueRedis = opened.Client
	}

	return &Resources{
		DB:           db,
		Redis:        redis,
		TokenRedis:   tokenRedis,
		QueueRedis:   queueRedis,
		cleanup:      cleanup,
		capabilities: capabilities,
		pings:        pings,
	}, nil
}

func (r *Resources) Close(ctx context.Context) error {
	if r == nil || r.cleanup == nil {
		return nil
	}
	return r.cleanup.Close(ctx)
}

func (r *Resources) Health(ctx context.Context) Report {
	if ctx == nil {
		ctx = context.Background()
	}
	if r == nil {
		return NewReport(map[string]Check{
			"database":    {Status: StatusDisabled},
			"redis":       {Status: StatusDisabled},
			"token_redis": {Status: StatusDisabled},
			"queue_redis": {Status: StatusDisabled},
			"realtime":    {Status: StatusDisabled},
			"scheduler":   {Status: StatusDisabled},
		})
	}
	return NewReport(map[string]Check{
		"database":    r.resourceCheck(ctx, "database", r.capabilities.database),
		"redis":       r.resourceCheck(ctx, "redis", r.capabilities.redis),
		"token_redis": r.resourceCheck(ctx, "token_redis", r.capabilities.tokenRedis),
		"queue_redis": r.resourceCheck(ctx, "queue_redis", r.capabilities.queueRedis),
		"realtime":    capabilityCheck(r.capabilities.realtime),
		"scheduler":   capabilityCheck(r.capabilities.scheduler),
	})
}

func (r *Resources) Readiness(ctx context.Context) Report {
	return r.Health(ctx)
}

func (r *Resources) resourceCheck(ctx context.Context, name string, enabled bool) Check {
	if !enabled {
		return Check{Status: StatusDisabled}
	}
	ping := r.pings[name]
	if ping == nil {
		return Check{Status: StatusDown, Message: name + " health check is unavailable"}
	}
	if err := ping(ctx); err != nil {
		return Check{Status: StatusDown, Message: name + " is unavailable"}
	}
	return Check{Status: StatusUp}
}

func capabilityCheck(enabled bool) Check {
	if !enabled {
		return Check{Status: StatusDisabled}
	}
	return Check{Status: StatusUp}
}

func capabilitiesFor(process config.Process, cfg config.Config) (resourceCapabilities, error) {
	capabilities := resourceCapabilities{
		database:   true,
		redis:      true,
		tokenRedis: process == config.ProcessAPI,
		queueRedis: cfg.Queue.Enabled,
		realtime:   cfg.Realtime.Enabled,
		scheduler:  process == config.ProcessWorker && cfg.Scheduler.Enabled,
	}
	if process != config.ProcessAPI && process != config.ProcessWorker {
		return resourceCapabilities{}, fmt.Errorf("runtime process is unsupported")
	}
	if strings.TrimSpace(cfg.MySQL.DSN) == "" {
		return resourceCapabilities{}, fmt.Errorf("%s requires database configuration", process)
	}
	if strings.TrimSpace(cfg.Redis.Addr) == "" {
		return resourceCapabilities{}, fmt.Errorf("%s requires redis configuration", process)
	}
	if capabilities.scheduler && !capabilities.queueRedis {
		return resourceCapabilities{}, fmt.Errorf("scheduler requires queue redis")
	}
	return capabilities, nil
}

func activateResource(
	ctx context.Context,
	cleanup *Cleanup,
	pings map[string]func(context.Context) error,
	name string,
	clientReady bool,
	ping func(context.Context) error,
	closeFn func(context.Context) error,
) error {
	if closeFn == nil {
		return failResourceOpen(ctx, cleanup, name, errors.New("opener returned nil close function"))
	}
	if err := cleanup.Add(name, closeFn); err != nil {
		return failResourceOpen(ctx, cleanup, name, err)
	}
	if !clientReady {
		return failResourceOpen(ctx, cleanup, name, errors.New("opener returned nil client"))
	}
	if ping == nil {
		return failResourceOpen(ctx, cleanup, name, errors.New("opener returned nil ping function"))
	}
	if err := ping(ctx); err != nil {
		return failResourceOpen(ctx, cleanup, name, err)
	}
	pings[name] = ping
	return nil
}

func failResourceOpen(ctx context.Context, cleanup *Cleanup, name string, cause error) error {
	primary := apperror.Wrap(
		"dependency."+strings.ReplaceAll(name, " ", "_"),
		apperror.CategoryDependency,
		http.StatusServiceUnavailable,
		apperror.Retryable,
		"common.dependency_unavailable",
		nil,
		name+" unavailable",
		cause,
	)
	return errors.Join(primary, cleanup.Close(ctx))
}

func (open Openers) withDefaults() Openers {
	recorder := open.Telemetry
	if recorder == nil {
		recorder = telemetry.Noop()
	}
	if open.Database == nil {
		open.Database = func(ctx context.Context, cfg config.MySQLConfig) (OpenedResource[*database.Client], error) {
			return defaultDatabaseOpener(ctx, cfg, recorder)
		}
	}
	if open.Redis == nil {
		open.Redis = func(ctx context.Context, cfg config.RedisConfig) (OpenedResource[*redisclient.Client], error) {
			return defaultRedisOpener(ctx, cfg, recorder)
		}
	}
	if open.TokenRedis == nil {
		open.TokenRedis = func(ctx context.Context, cfg config.RedisConfig) (OpenedResource[*redisclient.Client], error) {
			return defaultRedisOpener(ctx, cfg, recorder)
		}
	}
	if open.QueueRedis == nil {
		open.QueueRedis = func(ctx context.Context, cfg config.RedisConfig) (OpenedResource[*redisclient.Client], error) {
			return defaultRedisOpener(ctx, cfg, recorder)
		}
	}
	return open
}

func defaultDatabaseOpener(_ context.Context, cfg config.MySQLConfig, recorder telemetry.Recorder) (OpenedResource[*database.Client], error) {
	client, err := database.Open(cfg, database.WithTelemetry(recorder))
	if err != nil {
		return OpenedResource[*database.Client]{}, err
	}
	return OpenedResource[*database.Client]{
		Client: client,
		Ping:   client.Ping,
		Close:  func(context.Context) error { return client.Close() },
	}, nil
}

func defaultRedisOpener(_ context.Context, cfg config.RedisConfig, recorder telemetry.Recorder) (OpenedResource[*redisclient.Client], error) {
	client, err := redisclient.Open(cfg, redisclient.WithTelemetry(recorder))
	if err != nil {
		return OpenedResource[*redisclient.Client]{}, err
	}
	return OpenedResource[*redisclient.Client]{
		Client: client,
		Ping:   client.Ping,
		Close:  func(context.Context) error { return client.Close() },
	}, nil
}
