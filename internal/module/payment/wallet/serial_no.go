package wallet

import (
	"time"

	"admin_back_go/internal/module/payment/serialno"
)

func newTransactionNo(now time.Time) string {
	return serialno.NewWalletTransactionNo(now)
}
