package pricing

import (
	"errors"
	"math"
	"testing"

	"admin_back_go/internal/module/ai/billing"
)

func TestQuoteFiveRMBPerMillionTokens(t *testing.T) {
	price := ModelPrice{ModelID: "m", Rates: []Rate{{Category: InputTokens, Unit: "token", PriceUnits: 500000000, UnitScale: 1000000}}}
	got, err := Quote(price, []QuoteLine{{Key: "a", Item: billing.UsageItem{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: 1000000}}}, 1000000)
	if err != nil || got.AmountUnits != 500000000 {
		t.Fatalf("quote = %#v, %v", got, err)
	}
}

func TestQuoteRoundsOnceAndAllocatesDeterministically(t *testing.T) {
	price := ModelPrice{ModelID: "m", Rates: []Rate{{Category: InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 3}}}
	lines := []QuoteLine{
		{Key: "b", Item: billing.UsageItem{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: 1}},
		{Key: "a", Item: billing.UsageItem{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: 1}},
	}
	got, err := Quote(price, lines, 1000000)
	if err != nil || got.AmountUnits != 1 || len(got.Lines) != 2 {
		t.Fatalf("quote = %#v, %v", got, err)
	}
	if got.Lines[0].AmountUnits != 0 || got.Lines[1].AmountUnits != 1 {
		t.Fatalf("largest remainder tie must be stable by key: %#v", got.Lines)
	}
}

func TestQuoteRejectsUnsupportedAndDuplicateLines(t *testing.T) {
	price := ModelPrice{ModelID: "m", Rates: []Rate{{Category: InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1}}}
	line := QuoteLine{Key: "same", Item: billing.UsageItem{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: 1}}
	if _, err := Quote(price, []QuoteLine{line, line}, 1000000); !errors.Is(err, ErrDuplicateLine) {
		t.Fatalf("expected duplicate line error, got %v", err)
	}
	if _, err := Quote(price, []QuoteLine{{Key: "media", Item: billing.UsageItem{Category: billing.UsageCategoryMedia, Unit: "image", Quantity: 1}}}, 1000000); !errors.Is(err, ErrUnsupportedUsage) {
		t.Fatalf("expected unsupported usage, got %v", err)
	}
}

func TestQuoteRejectsMultiplierAndFinalOverflow(t *testing.T) {
	price := ModelPrice{ModelID: "m", Rates: []Rate{{Category: InputTokens, Unit: "token", PriceUnits: math.MaxInt64, UnitScale: 1}}}
	line := []QuoteLine{{Key: "a", Item: billing.UsageItem{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: 2}}}
	if _, err := Quote(price, line, 0); !errors.Is(err, ErrInvalidMultiplier) {
		t.Fatalf("expected invalid multiplier, got %v", err)
	}
	if _, err := Quote(price, line, 1000000); !errors.Is(err, ErrQuoteOverflow) {
		t.Fatalf("expected overflow, got %v", err)
	}
}
