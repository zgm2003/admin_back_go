package serialno

import (
	"strings"
	"testing"
	"time"
)

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
