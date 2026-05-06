package payruntime

import "errors"

var (
	ErrRepositoryNotConfigured = errors.New("pay runtime repository not configured")
	ErrOrderNotFound           = errors.New("pay runtime order not found")
	ErrOrderConflict           = errors.New("pay runtime order conflict")
	ErrTransactionNotFound     = errors.New("pay runtime transaction not found")
	ErrWalletCreditConflict    = errors.New("pay runtime wallet credit conflict")
)
