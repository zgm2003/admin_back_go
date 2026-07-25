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
		{Version: "v1", CatalogVendor: "openai", ModelID: "a", Aliases: []string{"same"}, Rates: []Rate{{Category: InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1}}},
		{Version: "v1", CatalogVendor: "google", ModelID: "b", Aliases: []string{"same"}, Rates: []Rate{{Category: InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1}}},
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
