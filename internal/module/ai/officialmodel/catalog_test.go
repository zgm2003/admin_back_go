package officialmodel

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"admin_back_go/internal/module/ai/pricing"
)

func validCatalogModel(id string) Model {
	return Model{
		CatalogVersion:      "test-v1",
		CatalogVendor:       "openai",
		ModelFamily:         "gpt",
		ModelID:             id,
		LifecycleStatus:     LifecycleActive,
		ContextWindowTokens: 128000,
		MaxOutputTokens:     4096,
		Capabilities: Capabilities{
			InputModalities:     []string{ModalityText},
			OutputModalities:    []string{ModalityText},
			SupportsStreaming:   true,
			SupportsTools:       true,
			SupportedParameters: []string{ParameterTemperature},
		},
		OfficialPrice: pricing.PriceBook{
			ModelID: id,
			Rates: []pricing.Rate{
				{Category: pricing.InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1_000_000},
				{Category: pricing.OutputTokens, Unit: "token", PriceUnits: 2, UnitScale: 1_000_000},
			},
		},
		ModelSourceURL:   "https://developers.openai.com/api/docs/models/" + id,
		PricingSourceURL: "https://developers.openai.com/api/docs/pricing",
		RetrievedAt:      "2026-07-27",
		ReviewAfter:      "2026-10-27",
	}
}

func TestOfficialCatalogRejectsDuplicateCaseSensitiveIdentity(t *testing.T) {
	first := validCatalogModel("model-a")
	second := validCatalogModel("model-a")
	if _, err := NewCatalog("test-v1", []Model{first, second}); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("duplicate canonical identity error = %v", err)
	}

	second = validCatalogModel("model-b")
	first.Aliases = []string{"model-b"}
	if _, err := NewCatalog("test-v1", []Model{first, second}); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("alias/canonical conflict error = %v", err)
	}

	first = validCatalogModel("model-a")
	second = validCatalogModel("model-b")
	first.Aliases = []string{"reviewed-alias"}
	second.Aliases = []string{"reviewed-alias"}
	if _, err := NewCatalog("test-v1", []Model{first, second}); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("duplicate reviewed alias error = %v", err)
	}
}

func TestOfficialCatalogMatchesOnlyCanonicalIDOrReviewedAlias(t *testing.T) {
	model := validCatalogModel("Exact-Model")
	model.Aliases = []string{"exact-model-reviewed"}
	catalog, err := NewCatalog("test-v1", []Model{model})
	if err != nil {
		t.Fatal(err)
	}
	for _, identity := range []string{"Exact-Model", " exact-model-reviewed "} {
		resolved, err := catalog.ResolveIdentity(identity)
		if err != nil || resolved.ModelID != "Exact-Model" {
			t.Fatalf("ResolveIdentity(%q) = %#v, %v", identity, resolved, err)
		}
	}
	for _, identity := range []string{"exact-model", "Exact", "Exact-Model-extra"} {
		if resolved, err := catalog.ResolveIdentity(identity); !errors.Is(err, ErrModelUnmapped) || resolved.ModelID != "" {
			t.Fatalf("ResolveIdentity(%q) = %#v, %v", identity, resolved, err)
		}
	}
}

func TestOfficialCatalogDefaultHasCompleteSourcesAndLimits(t *testing.T) {
	models := Default.Models()
	if Default.Version() != "official_models_v1" || len(models) != 24 {
		t.Fatalf("default catalog version=%q count=%d", Default.Version(), len(models))
	}
	for _, model := range models {
		if model.CatalogVersion != Default.Version() || model.ModelID == "" || model.CatalogVendor == "" || model.ModelFamily == "" {
			t.Fatalf("incomplete identity: %#v", model)
		}
		if model.ContextWindowTokens <= 0 || model.MaxOutputTokens <= 0 || model.MaxOutputTokens > model.ContextWindowTokens {
			t.Fatalf("invalid model limits: %#v", model)
		}
		if len(model.OfficialPrice.Rates) == 0 || model.OfficialPrice.ModelID != model.ModelID {
			t.Fatalf("missing official price: %#v", model)
		}
		for _, source := range []string{model.ModelSourceURL, model.PricingSourceURL} {
			parsed, err := url.Parse(source)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
				t.Fatalf("invalid source %q for %s", source, model.ModelID)
			}
		}
		retrieved, err := time.Parse(time.DateOnly, model.RetrievedAt)
		if err != nil {
			t.Fatalf("invalid retrieved_at for %s: %v", model.ModelID, err)
		}
		reviewAfter, err := time.Parse(time.DateOnly, model.ReviewAfter)
		if err != nil || !reviewAfter.After(retrieved) {
			t.Fatalf("invalid review_after for %s: %q", model.ModelID, model.ReviewAfter)
		}
	}
}
