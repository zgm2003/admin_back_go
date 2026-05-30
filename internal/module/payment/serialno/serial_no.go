package serialno

import (
	"fmt"
	"sync/atomic"
	"time"
)

var seq uint64

func New(prefix string, now time.Time) string {
	next := atomic.AddUint64(&seq, 1) % 1_000_000
	return prefix + now.Format("20060102150405") + fmt.Sprintf("%09d%06d", now.Nanosecond(), next)
}

func NewWalletTransactionNo(now time.Time) string {
	return New("WLT", now)
}
