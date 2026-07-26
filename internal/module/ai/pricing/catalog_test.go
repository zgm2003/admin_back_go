package pricing

import (
	"errors"
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

	video, err := Default.Resolve("sora-2-pro")
	if err != nil {
		t.Fatalf("resolve sora-2-pro: %v", err)
	}
	assertCatalogRate(t, video, MediaUnits, "second", "720p", 30000000, 1)
	assertCatalogRate(t, video, MediaUnits, "second", "1024p", 50000000, 1)
	assertCatalogRate(t, video, MediaUnits, "second", "1080p", 70000000, 1)
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
