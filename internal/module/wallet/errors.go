package wallet

import "errors"

var (
	ErrRepositoryNotConfigured = errors.New("wallet repository not configured")
	ErrUserNotFound            = errors.New("wallet user not found")
	ErrInsufficientBalance     = errors.New("wallet insufficient balance")
	ErrAdjustmentConflict      = errors.New("wallet adjustment idempotency conflict")
)
