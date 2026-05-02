package bootstrap

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/config"
	"admin_back_go/internal/platform/database"
	"admin_back_go/internal/platform/redisclient"
	"admin_back_go/internal/readiness"
)

type Resources struct {
	DB         *database.Client
	Redis      *redisclient.Client
	TokenRedis *redisclient.Client

	databaseEnabled   bool
	redisEnabled      bool
	tokenRedisEnabled bool
	databaseErr       error
}

func NewResources(cfg config.Config) (*Resources, error) {
	resources := &Resources{}

	if strings.TrimSpace(cfg.MySQL.DSN) != "" {
		resources.databaseEnabled = true
		db, err := database.Open(cfg.MySQL)
		if err != nil {
			resources.databaseErr = err
			return resources, err
		}
		resources.DB = db
	}

	if strings.TrimSpace(cfg.Redis.Addr) != "" {
		resources.redisEnabled = true
		resources.Redis = redisclient.Open(cfg.Redis)

		tokenRedisCfg := cfg.Redis
		tokenRedisCfg.DB = cfg.Token.RedisDB
		resources.tokenRedisEnabled = true
		resources.TokenRedis = redisclient.Open(tokenRedisCfg)
	}

	return resources, nil
}

func (r *Resources) Readiness(ctx context.Context) readiness.Report {
	return readiness.NewReport(map[string]readiness.Check{
		"database":    r.databaseReadiness(ctx),
		"redis":       r.redisReadiness(ctx),
		"token_redis": r.tokenRedisReadiness(ctx),
	})
}

func (r *Resources) databaseReadiness(ctx context.Context) readiness.Check {
	if r == nil || !r.databaseEnabled {
		return readiness.Check{Status: readiness.StatusDisabled}
	}
	if r.databaseErr != nil {
		return readiness.Check{Status: readiness.StatusDown, Message: r.databaseErr.Error()}
	}
	if r.DB == nil {
		return readiness.Check{Status: readiness.StatusDown, Message: "database client is not initialized"}
	}
	if err := r.DB.Ping(ctx); err != nil {
		return readiness.Check{Status: readiness.StatusDown, Message: err.Error()}
	}
	return readiness.Check{Status: readiness.StatusUp}
}

func (r *Resources) redisReadiness(ctx context.Context) readiness.Check {
	if r == nil || !r.redisEnabled {
		return readiness.Check{Status: readiness.StatusDisabled}
	}
	if r.Redis == nil {
		return readiness.Check{Status: readiness.StatusDown, Message: "redis client is not initialized"}
	}
	if err := r.Redis.Ping(ctx); err != nil {
		return readiness.Check{Status: readiness.StatusDown, Message: err.Error()}
	}
	return readiness.Check{Status: readiness.StatusUp}
}

func (r *Resources) tokenRedisReadiness(ctx context.Context) readiness.Check {
	if r == nil || !r.tokenRedisEnabled {
		return readiness.Check{Status: readiness.StatusDisabled}
	}
	if r.TokenRedis == nil {
		return readiness.Check{Status: readiness.StatusDown, Message: "token redis client is not initialized"}
	}
	if err := r.TokenRedis.Ping(ctx); err != nil {
		return readiness.Check{Status: readiness.StatusDown, Message: err.Error()}
	}
	return readiness.Check{Status: readiness.StatusUp}
}

func (r *Resources) Close() error {
	if r == nil {
		return nil
	}

	var errs []error
	if r.Redis != nil {
		if err := r.Redis.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if r.TokenRedis != nil {
		if err := r.TokenRedis.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if r.DB != nil {
		if err := r.DB.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
