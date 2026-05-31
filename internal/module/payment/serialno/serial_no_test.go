package serialno

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewUsesCompactReadableSequenceSuffix(t *testing.T) {
	atomic.StoreUint64(&seq, 0)
	now := time.Date(2026, 5, 31, 9, 22, 21, 11_472_119, time.UTC)

	var serial string
	for i := 0; i < 10; i++ {
		serial = New("RCG", now)
	}

	const want = "RCG20260531092221011472119A"
	if serial != want {
		t.Fatalf("payment serial should use compact base36 sequence suffix, got %q want %q", serial, want)
	}
	if strings.Contains(serial, "000000000000") {
		t.Fatalf("payment serial must not contain the old long zero-padded sequence, got %q", serial)
	}
}

func TestNewWalletTransactionNoUsesSharedSequence(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 123, time.UTC)

	first := NewWalletTransactionNo(now)
	second := NewWalletTransactionNo(now)

	if first == second {
		t.Fatalf("wallet transaction numbers must differ across repeated calls at the same timestamp")
	}
	if !strings.HasPrefix(first, "WLT") || !strings.HasPrefix(second, "WLT") {
		t.Fatalf("wallet transaction numbers must keep WLT prefix, got %q and %q", first, second)
	}
}

func TestNewWalletTransactionNoDoesNotWrapAtOneMillionSameTimestamp(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 123, time.UTC)

	first := NewWalletTransactionNo(now)
	for i := 0; i < 999_999; i++ {
		_ = NewWalletTransactionNo(now)
	}
	nextAfterOneMillion := NewWalletTransactionNo(now)

	if first == nextAfterOneMillion {
		t.Fatalf("wallet transaction numbers must not wrap at one million calls for the same timestamp: %s", first)
	}
}
