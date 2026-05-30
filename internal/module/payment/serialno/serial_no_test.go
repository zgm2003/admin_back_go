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
