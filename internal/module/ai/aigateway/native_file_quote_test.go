package aigateway

import (
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/billing"
)

func TestNativeFileQuoteUsesOfficialContextAndMaxOutputBounds(t *testing.T) {
	snapshot := validPricingSnapshot()
	snapshot.ContextWindowTokens = 400000
	snapshot.CatalogMaxOutputTokens = 32768
	snapshot.EffectiveMaxOutputTokens = 32768
	quote := QuoteEvidence{
		PreparedRequestSchema:    infraai.PreparedChatSchemaFileManifestV1,
		InputUpperBoundStrategy:  infraai.SafeInputUpperBoundStrategyNativeFileContextWindowV1,
		EffectiveMaxOutputTokens: 32768,
		UpperBoundItems: []billing.UsageItem{
			{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: 400000},
			{Category: billing.UsageCategoryOutputText, Unit: "token", Quantity: 32768},
		},
	}
	if err := validateNativeFileTokenBounds(snapshot, quote, 400000, 32768); err != nil {
		t.Fatal(err)
	}
}

func TestNativeFileQuoteRejectsNonOfficialBounds(t *testing.T) {
	snapshot := validPricingSnapshot()
	snapshot.ContextWindowTokens = 400000
	snapshot.CatalogMaxOutputTokens = 32768
	snapshot.EffectiveMaxOutputTokens = 32768
	for _, input := range []int64{399999, 400001} {
		quote := QuoteEvidence{
			PreparedRequestSchema:    infraai.PreparedChatSchemaFileManifestV1,
			InputUpperBoundStrategy:  infraai.SafeInputUpperBoundStrategyNativeFileContextWindowV1,
			EffectiveMaxOutputTokens: 32768,
		}
		if err := validateNativeFileTokenBounds(snapshot, quote, input, 32768); err == nil {
			t.Fatalf("input bound %d must fail", input)
		}
	}
}
