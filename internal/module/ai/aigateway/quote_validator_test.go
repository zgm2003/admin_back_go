package aigateway

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"testing"

	"admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/module/ai/pricing"
)

func TestPersistedQuoteValidatorBindsQuoteToLockedPricingSnapshot(t *testing.T) {
	fingerprint := [32]byte{7}
	snapshot := validPricingSnapshot()
	run := RunSnapshot{RunID: 44, UserID: 9, ModelID: snapshot.RequestedModelID, RequestFingerprint: fingerprint, PricingSnapshotJSON: mustPricingSnapshotJSON(t, snapshot)}
	quote := QuoteEvidence{
		PricingVersion:           snapshot.Version,
		RequestFingerprint:       fingerprint,
		PreparedRequestSHA256:    sha256.Sum256([]byte(`{"model":"test"}`)),
		EffectiveMaxOutputTokens: snapshot.EffectiveMaxOutputTokens,
		UpperBoundItems: []billing.UsageItem{
			{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: 2},
			{Category: billing.UsageCategoryOutputText, Unit: "token", Quantity: 10},
		},
		CurrentCallMaxUnits: 34,
		TargetHoldUnits:     34,
	}
	validator := PersistedQuoteValidator{}
	if err := validator.ValidateQuote(context.Background(), run, quote.PreparedRequestSHA256, quote); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		mutate func(*RunSnapshot, *QuoteEvidence)
	}{
		{name: "request fingerprint", mutate: func(_ *RunSnapshot, q *QuoteEvidence) { q.RequestFingerprint[0]++ }},
		{name: "pricing version", mutate: func(_ *RunSnapshot, q *QuoteEvidence) { q.PricingVersion = "other" }},
		{name: "effective output cap", mutate: func(_ *RunSnapshot, q *QuoteEvidence) { q.EffectiveMaxOutputTokens++ }},
		{name: "current call maximum", mutate: func(_ *RunSnapshot, q *QuoteEvidence) { q.CurrentCallMaxUnits++ }},
		{name: "prior billable units", mutate: func(_ *RunSnapshot, q *QuoteEvidence) { q.PriorBillableUnits++ }},
		{name: "target hold", mutate: func(_ *RunSnapshot, q *QuoteEvidence) { q.TargetHoldUnits++ }},
		{name: "model binding", mutate: func(r *RunSnapshot, _ *QuoteEvidence) { r.ModelID = "other-model" }},
		{name: "unsupported upper-bound item", mutate: func(_ *RunSnapshot, q *QuoteEvidence) { q.UpperBoundItems[0].Unit = "character" }},
		{name: "duplicate upper-bound item", mutate: func(_ *RunSnapshot, q *QuoteEvidence) {
			q.UpperBoundItems = append(q.UpperBoundItems, q.UpperBoundItems[0])
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lockedRun := run
			candidate := quote
			candidate.UpperBoundItems = append([]billing.UsageItem(nil), quote.UpperBoundItems...)
			tc.mutate(&lockedRun, &candidate)
			if err := validator.ValidateQuote(context.Background(), lockedRun, quote.PreparedRequestSHA256, candidate); err == nil {
				t.Fatalf("quote was accepted: run=%+v quote=%+v", lockedRun, candidate)
			}
		})
	}
}

func TestPersistedQuoteValidatorRejectsVisuallyValidQuoteFromDifferentSnapshot(t *testing.T) {
	fingerprint := [32]byte{8}
	snapshot := validPricingSnapshot()
	run := RunSnapshot{RunID: 45, UserID: 9, ModelID: snapshot.RequestedModelID, RequestFingerprint: fingerprint, PricingSnapshotJSON: mustPricingSnapshotJSON(t, snapshot)}
	quote := QuoteEvidence{
		PricingVersion:           snapshot.Version,
		RequestFingerprint:       fingerprint,
		PreparedRequestSHA256:    sha256.Sum256([]byte(`{"model":"test"}`)),
		EffectiveMaxOutputTokens: snapshot.EffectiveMaxOutputTokens,
		UpperBoundItems:          []billing.UsageItem{{Category: billing.UsageCategoryOutputText, Unit: "token", Quantity: 10}},
		CurrentCallMaxUnits:      30,
		TargetHoldUnits:          30,
	}

	other := snapshot
	other.Rates = append([]pricing.Rate(nil), snapshot.Rates...)
	other.Rates[1].PriceUnits = 4
	run.PricingSnapshotJSON = mustPricingSnapshotJSON(t, other)
	if err := (PersistedQuoteValidator{}).ValidateQuote(context.Background(), run, quote.PreparedRequestSHA256, quote); err == nil {
		t.Fatal("quote calculated from a different rate snapshot was accepted")
	}
}

func TestPersistedQuoteValidatorRejectsQuoteForDifferentPreparedRequest(t *testing.T) {
	fingerprint := [32]byte{9}
	snapshot := validPricingSnapshot()
	run := RunSnapshot{RunID: 46, UserID: 9, ModelID: snapshot.RequestedModelID, RequestFingerprint: fingerprint, PricingSnapshotJSON: mustPricingSnapshotJSON(t, snapshot)}
	quote := QuoteEvidence{
		PricingVersion:           snapshot.Version,
		RequestFingerprint:       fingerprint,
		EffectiveMaxOutputTokens: snapshot.EffectiveMaxOutputTokens,
		UpperBoundItems:          []billing.UsageItem{{Category: billing.UsageCategoryOutputText, Unit: "token", Quantity: 10}},
		CurrentCallMaxUnits:      30,
		TargetHoldUnits:          30,
	}
	requestHash := sha256.Sum256([]byte(`{"model":"expected"}`))
	quote = quoteWithPreparedRequestHash(quote, sha256.Sum256([]byte(`{"model":"other"}`)))

	if err := (PersistedQuoteValidator{}).ValidateQuote(context.Background(), run, requestHash, quote); err == nil {
		t.Fatal("validator accepted quote bound to different prepared request")
	}
}

func validPricingSnapshot() PricingSnapshot {
	return PricingSnapshot{
		Version:                  "pricing-v1",
		Billable:                 true,
		CatalogVendor:            "openai",
		TransportEngine:          "openai",
		RequestedModelID:         "gpt-test-alias",
		CanonicalModelID:         "gpt-test",
		CatalogMaxOutputTokens:   100,
		EffectiveMaxOutputTokens: 10,
		MultiplierPPM:            1_000_000,
		SourceURL:                "https://example.com/pricing",
		RetrievedAt:              "2026-07-25",
		Rates: []pricing.Rate{
			{Category: pricing.InputTokens, Unit: "token", PriceUnits: 2, UnitScale: 1},
			{Category: pricing.OutputTokens, Unit: "token", PriceUnits: 3, UnitScale: 1},
			{Category: pricing.MediaUnits, Unit: "image", PriceUnits: 5, UnitScale: 1},
		},
	}
}

func mustPricingSnapshotJSON(t *testing.T, snapshot PricingSnapshot) string {
	t.Helper()
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
