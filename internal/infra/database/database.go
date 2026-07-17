package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"admin_back_go/internal/config"
	"admin_back_go/internal/telemetry"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var ErrEmptyDSN = errors.New("mysql dsn is empty")

type Client struct {
	Gorm *gorm.DB
	SQL  *sql.DB
}

type openOptions struct {
	recorder telemetry.Recorder
}

type Option func(*openOptions)

func WithTelemetry(recorder telemetry.Recorder) Option {
	return func(options *openOptions) {
		options.recorder = recorder
	}
}

func Open(cfg config.MySQLConfig, options ...Option) (*Client, error) {
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, ErrEmptyDSN
	}
	settings := openOptions{}
	for _, option := range options {
		if option != nil {
			option(&settings)
		}
	}
	gormConfig := &gorm.Config{DisableAutomaticPing: true}
	if settings.recorder != nil {
		gormConfig.Logger = newTelemetryLogger(nil, settings.recorder, defaultSlowThreshold)
	}

	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       cfg.DSN,
		SkipInitializeWithVersion: true,
	}), gormConfig)
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	return &Client{Gorm: db, SQL: sqlDB}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.SQL == nil {
		return sql.ErrConnDone
	}
	return c.SQL.PingContext(ctx)
}

func (c *Client) Close() error {
	if c == nil || c.SQL == nil {
		return nil
	}
	return c.SQL.Close()
}
