package mail

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const diagnosticRekeyReleaseTimeout = 5 * time.Second

type GormDiagnosticRekeyRepository struct {
	db  *gorm.DB
	sql *sql.DB
}

func NewGormDiagnosticRekeyRepository(client *database.Client) *GormDiagnosticRekeyRepository {
	if client == nil || client.Gorm == nil || client.SQL == nil {
		return nil
	}
	db := client.Gorm.Session(&gorm.Session{NewDB: true, Logger: logger.Default.LogMode(logger.Silent)})
	return &GormDiagnosticRekeyRepository{db: db, sql: client.SQL}
}

func (r *GormDiagnosticRekeyRepository) WithDiagnosticRekeyLock(ctx context.Context, name string, callback func(DiagnosticRekeyRepository) error) (returnErr error) {
	if r == nil || r.db == nil || r.sql == nil {
		return ErrDiagnosticRekeyRepositoryNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if callback == nil {
		return ErrDiagnosticRekeyRepositoryFailure
	}
	connection, err := r.sql.Conn(ctx)
	if err != nil {
		return ErrDiagnosticRekeyRepositoryFailure
	}
	lockHeld := false
	defer func() {
		if lockHeld {
			releaseCtx, cancel := context.WithTimeout(context.Background(), diagnosticRekeyReleaseTimeout)
			var releaseResult sql.NullInt64
			releaseErr := connection.QueryRowContext(releaseCtx, "SELECT RELEASE_LOCK(?)", name).Scan(&releaseResult)
			cancel()
			if releaseErr != nil || !releaseResult.Valid || releaseResult.Int64 != 1 {
				returnErr = ErrDiagnosticRekeyRepositoryFailure
				_ = connection.Raw(func(any) error { return driver.ErrBadConn })
			}
		}
		if closeErr := connection.Close(); closeErr != nil {
			returnErr = ErrDiagnosticRekeyRepositoryFailure
		}
	}()

	var lockResult sql.NullInt64
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", name, 0).Scan(&lockResult); err != nil {
		return ErrDiagnosticRekeyRepositoryFailure
	}
	if !lockResult.Valid {
		return ErrDiagnosticRekeyRepositoryFailure
	}
	if lockResult.Int64 == 0 {
		return ErrDiagnosticRekeyLockUnavailable
	}
	if lockResult.Int64 != 1 {
		return ErrDiagnosticRekeyRepositoryFailure
	}
	lockHeld = true

	boundDB := r.db.Session(&gorm.Session{NewDB: true, Context: ctx})
	boundDB.Statement.ConnPool = connection
	if callbackErr := callback(&GormDiagnosticRekeyRepository{db: boundDB, sql: r.sql}); callbackErr != nil {
		returnErr = fixedDiagnosticRekeyError(callbackErr)
	}
	return returnErr
}

func (r *GormDiagnosticRekeyRepository) DistinctDiagnosticKeyIDs(ctx context.Context) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticRekeyRepositoryNotConfigured
	}
	var keyIDs []string
	err := r.db.WithContext(contextOrBackground(ctx)).Raw(
		"SELECT DISTINCT BINARY key_id AS key_id FROM mail_log_verification_codes ORDER BY BINARY key_id ASC",
	).Scan(&keyIDs).Error
	if err != nil {
		return nil, ErrDiagnosticRekeyRepositoryFailure
	}
	return keyIDs, nil
}

func (r *GormDiagnosticRekeyRepository) ListDiagnosticCipherRows(ctx context.Context, keyID string, afterID uint64, limit int) ([]DiagnosticCipherRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticRekeyRepositoryNotConfigured
	}
	if limit <= 0 || limit > DefaultDiagnosticRekeyBatchSize {
		limit = DefaultDiagnosticRekeyBatchSize
	}
	var rows []DiagnosticCipherRow
	err := r.db.WithContext(contextOrBackground(ctx)).Raw(
		"SELECT id,key_id,code_enc FROM mail_log_verification_codes WHERE key_id=BINARY ? AND id>? ORDER BY key_id ASC,id ASC LIMIT ?",
		keyID, afterID, limit,
	).Scan(&rows).Error
	if err != nil {
		return nil, ErrDiagnosticRekeyRepositoryFailure
	}
	return rows, nil
}

func (r *GormDiagnosticRekeyRepository) RewriteDiagnosticCipherBatch(ctx context.Context, rewrites []DiagnosticCipherRewrite) error {
	if len(rewrites) == 0 {
		return nil
	}
	if r == nil || r.db == nil {
		return ErrDiagnosticRekeyRepositoryNotConfigured
	}
	err := r.db.WithContext(contextOrBackground(ctx)).Transaction(func(tx *gorm.DB) error {
		for _, rewrite := range rewrites {
			result := tx.Exec(
				"UPDATE mail_log_verification_codes SET key_id=?, code_enc=? WHERE id=? AND BINARY key_id=BINARY ? AND BINARY code_enc=BINARY ?",
				rewrite.NewKeyID, rewrite.NewCodeEnc, rewrite.ID, rewrite.OldKeyID, rewrite.OldCodeEnc,
			)
			if result.Error != nil {
				return ErrDiagnosticRekeyRepositoryFailure
			}
			if result.RowsAffected != 1 {
				return ErrDiagnosticRekeyOptimisticCompareFailed
			}
		}
		return nil
	})
	if errors.Is(err, ErrDiagnosticRekeyOptimisticCompareFailed) {
		return ErrDiagnosticRekeyOptimisticCompareFailed
	}
	if err != nil {
		return ErrDiagnosticRekeyRepositoryFailure
	}
	return nil
}

func (r *GormDiagnosticRekeyRepository) CountDiagnosticKeyID(ctx context.Context, keyID string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrDiagnosticRekeyRepositoryNotConfigured
	}
	var count int64
	if err := r.db.WithContext(contextOrBackground(ctx)).Raw(
		"SELECT COUNT(*) FROM mail_log_verification_codes WHERE key_id=BINARY ?", keyID,
	).Scan(&count).Error; err != nil {
		return 0, ErrDiagnosticRekeyRepositoryFailure
	}
	return count, nil
}

func (r *GormDiagnosticRekeyRepository) CountUnknownDiagnosticKeyIDs(ctx context.Context, allowed []string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrDiagnosticRekeyRepositoryNotConfigured
	}
	query := "SELECT COUNT(*) FROM mail_log_verification_codes"
	arguments := make([]any, 0, len(allowed))
	if len(allowed) > 0 {
		query += " WHERE BINARY key_id NOT IN (" + strings.TrimRight(strings.Repeat("BINARY ?,", len(allowed)), ",") + ")"
		for _, keyID := range allowed {
			arguments = append(arguments, keyID)
		}
	}
	var count int64
	if err := r.db.WithContext(contextOrBackground(ctx)).Raw(query, arguments...).Scan(&count).Error; err != nil {
		return 0, ErrDiagnosticRekeyRepositoryFailure
	}
	return count, nil
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

var _ DiagnosticRekeyRepository = (*GormDiagnosticRekeyRepository)(nil)
