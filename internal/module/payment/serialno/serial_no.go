package serialno

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var seq uint64

func New(prefix string, now time.Time) string {
	next := atomic.AddUint64(&seq, 1)
	timestamp := now.Format("20060102150405")
	nanosecond := fmt.Sprintf("%09d", now.Nanosecond())
	sequence := strings.ToUpper(strconv.FormatUint(next, 36))
	return prefix + timestamp + nanosecond + sequence
}

func NewWalletTransactionNo(now time.Time) string {
	return New("WLT", now)
}
