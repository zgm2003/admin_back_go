package databaseevolution

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrAdvisoryLockUnavailable = errors.New("database advisory lock is unavailable")

func WithAdvisoryLock(ctx context.Context, database *sql.DB, name string, timeout time.Duration, callback func() error) (result error) {
	if callback == nil {
		return errors.New("advisory lock callback is required")
	}
	return WithAdvisoryLockConnection(ctx, database, name, timeout, func(_ *sql.Conn) error {
		return callback()
	})
}
