package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"admin_back_go/internal/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var ErrEmptyDSN = errors.New("mysql dsn is empty")

type Client struct {
	Gorm *gorm.DB
	SQL  *sql.DB
}

func Open(cfg config.MySQLConfig) (*Client, error) {
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, ErrEmptyDSN
	}

	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       cfg.DSN,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
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
