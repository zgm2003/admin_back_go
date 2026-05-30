package wallet

import (
	"testing"
	"time"

	_ "admin_back_go/internal/module/payment"
	_ "unsafe"
)

//go:linkname paymentWalletTransactionNo admin_back_go/internal/module/payment.newWalletTransactionNo
func paymentWalletTransactionNo(now time.Time) string

func TestWalletTransactionNoSharesSequenceWithRechargeTransactions(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 123, time.UTC)

	rechargeNo := paymentWalletTransactionNo(now)
	consumeNo := newTransactionNo(now)

	if rechargeNo == consumeNo {
		t.Fatalf("recharge and consume wallet transaction numbers must share one WLT sequence, got duplicate %s", rechargeNo)
	}
}
