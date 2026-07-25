package wallet

import (
	"encoding/json"
	"testing"

	"admin_back_go/internal/shared/money"
)

func TestSummaryUsesCanonicalMoneyUnitStrings(t *testing.T) {
	response, err := summaryResponse(&Wallet{BalanceUnits: money.UnitsPerRMB + 1_000_000, HeldUnits: 1_000_000, TotalRechargeUnits: 2_000_000, TotalConsumeUnits: 1_000_000})
	if err != nil {
		t.Fatal(err)
	}
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
	response, err := summaryResponse(&Wallet{BalanceUnits: 1, HeldUnits: 2})
	if response != nil || err == nil {
		t.Fatalf("expected invalid available amount to return an error, response=%#v err=%v", response, err)
	}
}
