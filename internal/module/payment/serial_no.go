package payment

import (
	"time"

	"admin_back_go/internal/module/payment/serialno"
)

func newPaymentSerialNo(prefix string, now time.Time) string {
	return serialno.New(prefix, now)
}
