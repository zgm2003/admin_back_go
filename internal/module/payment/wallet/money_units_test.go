package wallet

import (
	"encoding/json"
	"testing"

	"admin_back_go/internal/shared/money"
)

func TestSummaryUsesCanonicalMoneyUnitStrings(t *testing.T) {
	response := summaryResponse(&Wallet{BalanceUnits: money.UnitsPerRMB + 1_000_000, HeldUnits: 1_000_000, TotalRechargeUnits: 2_000_000, TotalConsumeUnits: 1_000_000})
	if response.Balance != "1.01" || response.AvailableBalance != "1" || response.HeldAmount != "0.01" || response.TotalRecharge != "0.02" || response.TotalConsume != "0.01" {
		t.Fatalf("unexpected summary: %+v", response)
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"balance":"1.01","available_balance":"1","held_amount":"0.01","total_recharge":"0.02","total_consume":"0.01"}` {
		t.Fatalf("unexpected JSON: %s", payload)
	}
}

func TestSummaryRejectsNegativeAvailableUnits(t *testing.T) {
	response := summaryResponse(&Wallet{BalanceUnits: 1, HeldUnits: 2})
	if response.AvailableBalance != "" {
		t.Fatalf("expected invalid available amount to be empty, got %q", response.AvailableBalance)
	}
}
