package pricing

import (
	"errors"
	"testing"

	"admin_back_go/internal/module/ai/billing"
)

func tierLine(key, attempt string, category billing.UsageCategory, tier string, quantity int64) QuoteLine {
	return QuoteLine{Key: key, AttemptID: attempt, Item: billing.UsageItem{Category: category, Unit: "token", TierKey: tier, Quantity: quantity}}
}

func TestUpperBoundLinesChoosesMostExpensiveCompatibleTier(t *testing.T) {
	price := PriceBook{ModelID: "m", Rates: []Rate{
		{Category: InputTokens, Unit: "token", TierKey: "short_context", PriceUnits: 2, UnitScale: 1},
		{Category: InputTokens, Unit: "token", TierKey: "long_context", PriceUnits: 5, UnitScale: 1},
		{Category: CacheWrite, Unit: "token", TierKey: "5m", PriceUnits: 3, UnitScale: 1},
		{Category: CacheWrite, Unit: "token", TierKey: "1h", PriceUnits: 4, UnitScale: 1},
	}}
	got, err := UpperBoundLines(price, []QuoteLine{tierLine("input", "a", billing.UsageCategoryInputText, "", 1), tierLine("write", "a", billing.UsageCategoryCacheWrite, "5m", 1)})
	if err != nil || got[0].Item.TierKey != "long_context" || got[1].Item.TierKey != "5m" {
		t.Fatalf("upper bound = %#v, %v", got, err)
	}
}

func TestUpperBoundLinesPricesAggregateInputAtMostExpensiveInputFamilyRate(t *testing.T) {
	price := PriceBook{ModelID: "claude", Rates: []Rate{
		{Category: InputTokens, Unit: "token", PriceUnits: 3, UnitScale: 1},
		{Category: CacheRead, Unit: "token", PriceUnits: 1, UnitScale: 1},
		{Category: CacheWrite, Unit: "token", TierKey: "5m", PriceUnits: 4, UnitScale: 1},
		{Category: CacheWrite, Unit: "token", TierKey: "1h", PriceUnits: 6, UnitScale: 1},
	}}
	got, err := UpperBoundLines(price, []QuoteLine{tierLine("input", "a", billing.UsageCategoryInputText, "", 10)})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Item.Category != billing.UsageCategoryCacheWrite || got[0].Item.TierKey != "1h" {
		t.Fatalf("aggregate input upper bound = %#v", got[0].Item)
	}
}

func TestUpperBoundLinesUsesCatalogContextTierForProviderCacheDetail(t *testing.T) {
	price := PriceBook{ModelID: "m", Rates: []Rate{
		{Category: CacheWrite, Unit: "token", TierKey: "short_context", PriceUnits: 6, UnitScale: 1},
		{Category: CacheWrite, Unit: "token", TierKey: "long_context", PriceUnits: 10, UnitScale: 1},
	}}
	got, err := UpperBoundLines(price, []QuoteLine{tierLine("write", "a", billing.UsageCategoryCacheWrite, "5m", 1)})
	if err != nil || got[0].Item.TierKey != "long_context" {
		t.Fatalf("contextual cache upper bound = %#v, %v", got, err)
	}
}

func TestSettlementLinesAppliesLongContextPerAttemptOnlyAboveThreshold(t *testing.T) {
	price := PriceBook{ModelID: "m", ContextTierThresholdTokens: 272000, Rates: []Rate{
		{Category: InputTokens, Unit: "token", TierKey: "short_context", PriceUnits: 2, UnitScale: 1},
		{Category: InputTokens, Unit: "token", TierKey: "long_context", PriceUnits: 5, UnitScale: 1},
		{Category: CacheRead, Unit: "token", TierKey: "short_context", PriceUnits: 1, UnitScale: 1},
		{Category: CacheRead, Unit: "token", TierKey: "long_context", PriceUnits: 2, UnitScale: 1},
		{Category: OutputTokens, Unit: "token", TierKey: "short_context", PriceUnits: 6, UnitScale: 1},
		{Category: OutputTokens, Unit: "token", TierKey: "long_context", PriceUnits: 9, UnitScale: 1},
	}}
	lines := []QuoteLine{
		tierLine("a-in", "a", billing.UsageCategoryInputText, "", 272000), tierLine("a-out", "a", billing.UsageCategoryOutputText, "", 1),
		tierLine("b-in", "b", billing.UsageCategoryInputText, "", 272000), tierLine("b-cache", "b", billing.UsageCategoryCacheRead, "", 1), tierLine("b-out", "b", billing.UsageCategoryOutputText, "", 1),
	}
	got, err := SettlementLines(price, lines)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"short_context", "short_context", "long_context", "long_context", "long_context"} {
		if got[i].Item.TierKey != want {
			t.Fatalf("line %d tier = %q, want %q", i, got[i].Item.TierKey, want)
		}
	}
}

func TestSettlementLinesFailsClosedForUntieredAmbiguousCacheWrite(t *testing.T) {
	price := PriceBook{ModelID: "m", Rates: []Rate{
		{Category: CacheWrite, Unit: "token", TierKey: "5m", PriceUnits: 1, UnitScale: 1},
		{Category: CacheWrite, Unit: "token", TierKey: "1h", PriceUnits: 2, UnitScale: 1},
	}}
	_, err := SettlementLines(price, []QuoteLine{tierLine("write", "a", billing.UsageCategoryCacheWrite, "", 1)})
	if !errors.Is(err, ErrPriceUnavailable) {
		t.Fatalf("ambiguous cache write returned %v", err)
	}
}
