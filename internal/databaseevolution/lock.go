package databaseevolution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

var ErrAdvisoryLockUnavailable = errors.New("database advisory lock is unavailable")

func WithAdvisoryLock(ctx context.Context, database *sql.DB, name string, timeout time.Duration, callback func() error) (result error) {
	if database == nil {
		return errors.New("database is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("advisory lock name is required")
	}
	if timeout <= 0 {
		return errors.New("advisory lock timeout must be positive")
	}
	if callback == nil {
		return errors.New("advisory lock callback is required")
	}
	connection, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve advisory lock connection: %w", err)
	}
	defer connection.Close()

	timeoutSeconds := int64(math.Ceil(timeout.Seconds()))
	var locked sql.NullInt64
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", name, timeoutSeconds).Scan(&locked); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}
	if !locked.Valid || locked.Int64 != 1 {
		return ErrAdvisoryLockUnavailable
	}
	defer func() {
		releaseContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var released sql.NullInt64
		releaseErr := connection.QueryRowContext(releaseContext, "SELECT RELEASE_LOCK(?)", name).Scan(&released)
		if releaseErr == nil && (!released.Valid || released.Int64 != 1) {
			releaseErr = errors.New("advisory lock was not released")
		}
		if releaseErr != nil {
			result = errors.Join(result, fmt.Errorf("release advisory lock: %w", releaseErr))
		}
	}()

	return callback()
}
