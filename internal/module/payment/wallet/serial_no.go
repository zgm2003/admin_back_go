package wallet

import (
	"time"

	"admin_back_go/internal/module/payment/serialno"
)

func newTransactionNo(now time.Time) string {
	return serialno.NewWalletTransactionNo(now)
}

// NewTransactionNo exposes the wallet sequence to process composition tests and adapters.
func NewTransactionNo(now time.Time) string { return newTransactionNo(now) }
