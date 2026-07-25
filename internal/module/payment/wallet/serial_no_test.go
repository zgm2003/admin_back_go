package wallet

import (
	"testing"
	"time"
)

func TestWalletTransactionNoUsesUniqueSequence(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 123, time.UTC)

	first := newTransactionNo(now)
	second := newTransactionNo(now)
	if first == second {
		t.Fatalf("wallet transaction numbers must be unique, got duplicate %s", first)
	}
}
