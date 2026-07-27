package pricing

import (
	"errors"
	"strings"
	"testing"
)

func TestCatalogResolvesCanonicalAndAliasWithoutTransportInference(t *testing.T) {
	catalog := NewCatalog([]ModelPrice{{Version: "v1", CatalogVendor: "anthropic", ModelID: "claude-3", Aliases: []string{"claude"}, MaxOutputTokens: 100, SourceURL: "https://example.test", RetrievedAt: "2026-07-25", Rates: []Rate{{Category: InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1}}}})
	got, err := catalog.Resolve(" claude ")
	if err != nil || got.CatalogVendor != "anthropic" || got.ModelID != "claude-3" {
		t.Fatalf("alias resolution = %#v, %v", got, err)
	}
	got, err = catalog.Resolve("claude-3")
	if err != nil || got.ModelID != "claude-3" {
		t.Fatalf("canonical resolution = %#v, %v", got, err)
	}
}

func TestV3CatalogDocumentRequiresCurrencyPolicyAndOfficialSources(t *testing.T) {
	valid := `{
		"version":"official_numeric_parity_v3",
		"official_currency":"USD",
		"billing_currency":"CNY",
		"conversion_policy":"numeric_parity",
		"models":[{
			"catalog_vendor":"openai","model_family":"gpt","model_id":"m",
			"pricing_profile":"standard_global","max_output_tokens":1,
			"source_url":"https://developers.openai.com/api/docs/pricing",
			"retrieved_at":"2026-07-27",
			"rates":[{"category":"input","unit":"token","tier_key":"","price":"0.1","unit_scale":1000000}]
		}]}`
	catalog, err := loadOfficialCatalog([]byte(valid))
	if err != nil {
		t.Fatalf("load valid v3 catalog: %v", err)
	}
	price, err := catalog.Resolve("m")
	if err != nil || price.Rates[0].PriceUnits != 10000000 || price.ModelFamily != "gpt" || price.PricingProfile != "standard_global" {
		t.Fatalf("loaded price = %#v, %v", price, err)
	}

	for name, replacement := range map[string]string{
		"version":  `"version":"v3"`,
		"official": `"official_currency":"EUR"`,
		"billing":  `"billing_currency":"USD"`,
		"policy":   `"conversion_policy":"fx"`,
		"host":     `"source_url":"https://example.test/pricing"`,
		"http":     `"source_url":"http://developers.openai.com/api/docs/pricing"`,
		"zero":     `"price":"0"`,
		"unit":     `"unit":"request"`,
	} {
		input := valid
		switch name {
		case "version":
			input = strings.Replace(input, `"version":"official_numeric_parity_v3"`, replacement, 1)
		case "official":
			input = strings.Replace(input, `"official_currency":"USD"`, replacement, 1)
		case "billing":
			input = strings.Replace(input, `"billing_currency":"CNY"`, replacement, 1)
		case "policy":
			input = strings.Replace(input, `"conversion_policy":"numeric_parity"`, replacement, 1)
		case "host", "http":
			input = strings.Replace(input, `"source_url":"https://developers.openai.com/api/docs/pricing"`, replacement, 1)
		case "zero":
			input = strings.Replace(input, `"price":"0.1"`, replacement, 1)
		case "unit":
			input = strings.Replace(input, `"unit":"token"`, replacement, 1)
		}
		if _, err := loadOfficialCatalog([]byte(input)); !errors.Is(err, ErrInvalidCatalog) {
			t.Fatalf("%s violation returned %v", name, err)
		}
	}
}

func TestV3CatalogRejectsNormalizedIdentityCollisions(t *testing.T) {
	input := `{
		"version":"official_numeric_parity_v3","official_currency":"USD","billing_currency":"CNY","conversion_policy":"numeric_parity",
		"models":[
			{"catalog_vendor":"openai","model_family":"gpt","model_id":"a","aliases":["shared"],"pricing_profile":"standard_global","max_output_tokens":1,"source_url":"https://openai.com/a","retrieved_at":"2026-07-27","rates":[{"category":"input","unit":"token","tier_key":"","price":"1","unit_scale":1}]},
			{"catalog_vendor":"openai","model_family":"gpt","model_id":"b","aliases":[" shared "],"pricing_profile":"standard_global","max_output_tokens":1,"source_url":"https://openai.com/b","retrieved_at":"2026-07-27","rates":[{"category":"input","unit":"token","tier_key":"","price":"1","unit_scale":1}]}
		]}`
	if _, err := loadOfficialCatalog([]byte(input)); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("normalized alias collision returned %v", err)
	}
}

func TestEmbeddedV3CatalogContainsOnlyReviewedManagedModels(t *testing.T) {
	wantAliases := map[string]string{"gpt-5.6": "gpt-5.6-sol", "gpt-4.1-latest": "gpt-4.1", "claude-haiku-4-5": "claude-haiku-4-5-20251001", "claude-sonnet-4-5": "claude-sonnet-4-5-20250929", "claude-opus-4-5": "claude-opus-4-5-20251101"}
	wantManaged := map[string]bool{
		"gpt-5.6-sol": true, "gpt-5.6-terra": true, "gpt-5.6-luna": true, "gpt-5.5": true, "gpt-5.5-pro": true,
		"gpt-5.4": true, "gpt-5.4-mini": true, "gpt-5.4-nano": true, "gpt-5.4-pro": true, "gpt-4.1": true,
		"gpt-4.1-mini": true, "gpt-4o": true, "gpt-4o-mini": true, "claude-fable-5": true, "claude-opus-5": true,
		"claude-sonnet-5": true, "claude-haiku-4-5-20251001": true, "claude-opus-4-8": true, "claude-opus-4-7": true,
		"claude-opus-4-6": true, "claude-sonnet-4-6": true, "claude-sonnet-4-5-20250929": true, "claude-opus-4-5-20251101": true,
	}
	if Default.Version() != "official_numeric_parity_v3" {
		t.Fatalf("catalog version = %q", Default.Version())
	}
	for _, price := range Default.Models() {
		if price.ModelFamily == "gpt" || price.ModelFamily == "claude" {
			if !wantManaged[price.ModelID] {
				t.Fatalf("unexpected managed model %q", price.ModelID)
			}
			delete(wantManaged, price.ModelID)
			if price.PricingProfile != "standard_global" || price.RetrievedAt != "2026-07-27" {
				t.Fatalf("invalid managed metadata: %#v", price)
			}
		}
	}
	if len(wantManaged) != 0 {
		t.Fatalf("missing managed models: %v", wantManaged)
	}
	for alias, want := range wantAliases {
		got, err := Default.Resolve(alias)
		if err != nil || got.ModelID != want {
			t.Fatalf("alias %q = %#v, %v", alias, got, err)
		}
	}
	sonnet5, err := Default.Resolve("claude-sonnet-5")
	if err != nil || sonnet5.ReviewAfter != "2026-09-01" {
		t.Fatalf("claude-sonnet-5 review metadata = %#v, %v", sonnet5, err)
	}
}

func TestCatalogRejectsAmbiguousAliasAndInvalidRates(t *testing.T) {
	catalog := NewCatalog([]ModelPrice{
		{Version: "v1", CatalogVendor: "openai", ModelID: "a", Aliases: []string{"same"}, MaxOutputTokens: 1, SourceURL: "https://example.test/a", RetrievedAt: "2026-07-25", Rates: []Rate{{Category: InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1}}},
		{Version: "v1", CatalogVendor: "google", ModelID: "b", Aliases: []string{"same"}, MaxOutputTokens: 1, SourceURL: "https://example.test/b", RetrievedAt: "2026-07-25", Rates: []Rate{{Category: InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1}}},
	})
	if _, err := catalog.Resolve("same"); !errors.Is(err, ErrAmbiguousModel) {
		t.Fatalf("expected ambiguous model, got %v", err)
	}
	if _, err := NewCatalogChecked([]ModelPrice{{ModelID: "bad", Rates: []Rate{{Category: InputTokens, Unit: "token", PriceUnits: -1, UnitScale: 1}}}}); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("expected invalid catalog, got %v", err)
	}
}

func TestEmbeddedCatalogHasAuditableSources(t *testing.T) {
	if Default.Version() == "" {
		t.Fatal("embedded catalog version is empty")
	}
	for _, price := range Default.Models() {
		if price.CatalogVendor == "" || price.SourceURL == "" || price.RetrievedAt == "" {
			t.Fatalf("price is not auditable: %#v", price)
		}
	}
}

func TestEmbeddedCatalogIncludesReviewedMediaRates(t *testing.T) {
	image, err := Default.Resolve("gpt-image-2")
	if err != nil {
		t.Fatalf("resolve gpt-image-2: %v", err)
	}
	assertCatalogRate(t, image, InputTokens, "token", "", 500000000, 1000000)
	assertCatalogRate(t, image, OutputTokens, "token", "", 3000000000, 1000000)
	if image.MaxOutputTokens != 355785 {
		t.Fatalf("gpt-image-2 request output bound=%d, want 355785", image.MaxOutputTokens)
	}

	if _, err := Default.Resolve("sora-2-pro"); !errors.Is(err, ErrPriceUnavailable) {
		t.Fatalf("retired video pricing should be unavailable, got %v", err)
	}
}

func TestEmbeddedCatalogUsesConfirmedGPT54ContextRates(t *testing.T) {
	model, err := Default.Resolve("gpt-5.4")
	if err != nil {
		t.Fatal(err)
	}
	if model.ContextTierThresholdTokens != 272000 || model.MaxOutputTokens != 128000 || model.SourceURL != "https://developers.openai.com/api/docs/models/gpt-5.4" {
		t.Fatalf("gpt-5.4 metadata = %#v", model)
	}
	assertCatalogRate(t, model, InputTokens, "token", "short_context", 250000000, 1000000)
	assertCatalogRate(t, model, CacheRead, "token", "short_context", 25000000, 1000000)
	assertCatalogRate(t, model, OutputTokens, "token", "short_context", 1500000000, 1000000)
	assertCatalogRate(t, model, InputTokens, "token", "long_context", 500000000, 1000000)
	assertCatalogRate(t, model, CacheRead, "token", "long_context", 50000000, 1000000)
	assertCatalogRate(t, model, OutputTokens, "token", "long_context", 2250000000, 1000000)
}

func assertCatalogRate(t *testing.T, model ModelPrice, category Category, unit, tier string, priceUnits, unitScale int64) {
	t.Helper()
	for _, rate := range model.Rates {
		if rate.Category == category && rate.Unit == unit && rate.TierKey == tier {
			if rate.PriceUnits != priceUnits || rate.UnitScale != unitScale {
				t.Fatalf("rate %s/%s/%s = %#v", category, unit, tier, rate)
			}
			return
		}
	}
	t.Fatalf("missing rate %s/%s/%s in %#v", category, unit, tier, model)
}

func TestCatalogRejectsUnsafeOutputBoundAndMissingAuditMetadata(t *testing.T) {
	base := ModelPrice{Version: "v1", CatalogVendor: "openai", ModelID: "m", MaxOutputTokens: 1, SourceURL: "https://example.test", RetrievedAt: "2026-07-25", Rates: []Rate{{Category: InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1}}}
	for _, model := range []ModelPrice{
		func() ModelPrice { m := base; m.MaxOutputTokens = 0; return m }(),
		func() ModelPrice { m := base; m.MaxOutputTokens = -1; return m }(),
		func() ModelPrice { m := base; m.MaxOutputTokens = MaxSafeOutputTokens + 1; return m }(),
	} {
		if _, err := NewCatalogChecked([]ModelPrice{model}); !errors.Is(err, ErrUnsafeTokenUpperBound) {
			t.Fatalf("unsafe output bound should fail closed, got %v", err)
		}
	}
	for field := range map[string]struct{}{"version": {}, "source": {}, "retrieved": {}} {
		model := base
		switch field {
		case "version":
			model.Version = ""
		case "source":
			model.SourceURL = ""
		case "retrieved":
			model.RetrievedAt = ""
		}
		if _, err := NewCatalogChecked([]ModelPrice{model}); !errors.Is(err, ErrInvalidCatalog) {
			t.Fatalf("missing %s audit field should fail, got %v", field, err)
		}
	}
}

func TestCatalogDeepCopiesRowsAndRejectsDuplicateAlias(t *testing.T) {
	model := ModelPrice{Version: "v1", CatalogVendor: "openai", ModelID: "m", Aliases: []string{"alias"}, MaxOutputTokens: 1, SourceURL: "https://example.test", RetrievedAt: "2026-07-25", Rates: []Rate{{Category: InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1}}}
	catalog, err := NewCatalogChecked([]ModelPrice{model})
	if err != nil {
		t.Fatalf("construct catalog: %v", err)
	}
	model.Aliases[0] = "changed"
	model.Rates[0].PriceUnits = 99
	resolved, err := catalog.Resolve("alias")
	if err != nil || resolved.Rates[0].PriceUnits != 1 {
		t.Fatalf("catalog was mutated by source row: %#v, %v", resolved, err)
	}
	model.Aliases = []string{"alias", "alias"}
	if _, err := NewCatalogChecked([]ModelPrice{model}); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("duplicate alias should fail, got %v", err)
	}
}
