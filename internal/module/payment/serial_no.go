package payment

import (
	"fmt"
	"sync/atomic"
	"time"
)

var paymentSerialNoSeq uint64

func newPaymentSerialNo(prefix string, now time.Time) string {
	seq := atomic.AddUint64(&paymentSerialNoSeq, 1) % 1_000_000
	return prefix + now.Format("20060102150405") + fmt.Sprintf("%09d%06d", now.Nanosecond(), seq)
}
