package wallet

import (
	"fmt"
	"sync/atomic"
	"time"
)

var transactionNoSeq uint64

func newTransactionNo(now time.Time) string {
	seq := atomic.AddUint64(&transactionNoSeq, 1) % 1_000_000
	return "WLT" + now.Format("20060102150405") + fmt.Sprintf("%09d%06d", now.Nanosecond(), seq)
}
