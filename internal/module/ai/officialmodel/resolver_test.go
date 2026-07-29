package officialmodel

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/shared/clock"
)

type fakeOverrideRepository struct {
	override *PriceOverride
	findErr  error
}

func (repository *fakeOverrideRepository) FindOverride(context.Context, string, string) (*PriceOverride, error) {
	if repository.findErr != nil {
		return nil, repository.findErr
	}
	return cloneOverride(repository.override), nil
}

func (repository *fakeOverrideRepository) ReplaceOverride(context.Context, ReplaceOverrideCommand, ExistingOverrideValidator) (*PriceOverride, *PriceOverride, error) {
	return nil, nil, errors.New("unexpected ReplaceOverride")
}

func (repository *fakeOverrideRepository) DeleteOverride(context.Context, DeleteOverrideCommand, ExistingOverrideValidator) (*PriceOverride, error) {
	return nil, errors.New("unexpected DeleteOverride")
}

func resolverTestCatalog(t *testing.T) *Catalog {
	t.Helper()
	model := validCatalogModel("gpt-reviewed")
	model.Aliases = []string{"gpt-reviewed-alias"}
	model.ContextTierThresholdTokens = 0
	model.OfficialPrice = pricing.PriceBook{
		ModelID: model.ModelID,
		Rates: []pricing.Rate{
			{Category: pricing.InputTokens, Unit: "token", PriceUnits: 100, UnitScale: 1_000_000},
			{Category: pricing.OutputTokens, Unit: "token", PriceUnits: 200, UnitScale: 1_000_000},
		},
	}
	catalog, err := NewCatalog("test-v1", []Model{model})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func newResolverTestService(t *testing.T, repository Repository) *Service {
	t.Helper()
	return NewService(repository,
		WithCatalog(resolverTestCatalog(t)),
		WithClock(clock.Func(func() time.Time {
			return time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
		})),
	)
}

func TestResolverReturnsCatalogFactsWithEffectiveOverridePrice(t *testing.T) {
	verifiedAt := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	repository := &fakeOverrideRepository{override: &PriceOverride{
		ID: 9, CatalogVendor: "openai", ModelID: "gpt-reviewed", Version: 2,
		SourceURL: "https://developers.openai.com/api/docs/pricing", VerifiedAt: verifiedAt, UpdatedBy: 7,
		Rates: []PriceOverrideRate{
			{OverrideID: 9, Category: pricing.InputTokens, Unit: "token", PriceUnits: 125, UnitScale: 1_000_000},
			{OverrideID: 9, Category: pricing.OutputTokens, Unit: "token", PriceUnits: 350, UnitScale: 1_000_000},
		},
	}}

	resolved, err := newResolverTestService(t, repository).Resolve(context.Background(), "gpt-reviewed-alias")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Model.ModelID != "gpt-reviewed" || resolved.Model.MaxOutputTokens != 4096 || resolved.Model.CatalogVersion != "test-v1" {
		t.Fatalf("catalog facts changed: %#v", resolved.Model)
	}
	if resolved.PriceSource != PriceSourceOverride || resolved.OverrideVersion != 2 || resolved.PriceSourceURL != repository.override.SourceURL || !resolved.PriceVerifiedAt.Equal(verifiedAt) {
		t.Fatalf("override metadata missing: %#v", resolved)
	}
	if got := resolved.EffectivePrice.Rates; len(got) != 2 || got[0].PriceUnits != 125 || got[1].PriceUnits != 350 {
		t.Fatalf("effective rates=%#v", got)
	}
}

func TestResolverNeverFallsBackFromCorruptOverride(t *testing.T) {
	repository := &fakeOverrideRepository{override: &PriceOverride{
		ID: 9, CatalogVendor: "openai", ModelID: "gpt-reviewed", Version: 2,
		SourceURL:  "https://developers.openai.com/api/docs/pricing",
		VerifiedAt: time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC), UpdatedBy: 7,
		Rates: []PriceOverrideRate{
			{OverrideID: 9, Category: pricing.InputTokens, Unit: "token", PriceUnits: 125, UnitScale: 1_000_000},
		},
	}}
	if resolved, err := newResolverTestService(t, repository).Resolve(context.Background(), "gpt-reviewed"); !errors.Is(err, ErrInvalidOverride) || resolved.Model.ModelID != "" {
		t.Fatalf("resolved=%#v err=%v", resolved, err)
	}
}

func TestResolverPreservesOverrideUntilExplicitRestore(t *testing.T) {
	model := resolverTestCatalog(t).Models()[0]
	rates := make([]PriceOverrideRate, len(model.OfficialPrice.Rates))
	for index, rate := range model.OfficialPrice.Rates {
		rates[index] = PriceOverrideRate{
			OverrideID: 9, Category: rate.Category, Unit: rate.Unit, TierKey: rate.TierKey,
			PriceUnits: rate.PriceUnits, UnitScale: rate.UnitScale,
		}
	}
	repository := &fakeOverrideRepository{override: &PriceOverride{
		ID: 9, CatalogVendor: model.CatalogVendor, ModelID: model.ModelID, Version: 3,
		SourceURL: model.PricingSourceURL, VerifiedAt: time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC),
		UpdatedBy: 7, Rates: rates,
	}}
	resolved, err := newResolverTestService(t, repository).Resolve(context.Background(), model.ModelID)
	if err != nil || resolved.PriceSource != PriceSourceOverride || resolved.OverrideVersion != 3 {
		t.Fatalf("resolved=%#v err=%v", resolved, err)
	}
}

func TestResolverRejectsUnmappedAmbiguousAndMissingPrice(t *testing.T) {
	service := newResolverTestService(t, &fakeOverrideRepository{})
	if _, err := service.Resolve(context.Background(), "GPT-REVIEWED"); !errors.Is(err, ErrModelUnmapped) {
		t.Fatalf("case mismatch error=%v", err)
	}

	service = newResolverTestService(t, &fakeOverrideRepository{findErr: ErrOverrideMappingAmbiguous})
	if _, err := service.Resolve(context.Background(), "gpt-reviewed"); !errors.Is(err, ErrOverrideMappingAmbiguous) {
		t.Fatalf("ambiguous override error=%v", err)
	}

	model := resolverTestCatalog(t).Models()[0]
	model.OfficialPrice = pricing.PriceBook{ModelID: model.ModelID}
	corruptCatalog := &Catalog{
		version: "test-v1", models: []Model{model},
		byCanonical: map[string]int{model.ModelID: 0}, byAlias: map[string]int{},
	}
	service = NewService(&fakeOverrideRepository{}, WithCatalog(corruptCatalog))
	if _, err := service.Resolve(context.Background(), model.ModelID); !errors.Is(err, ErrPriceUnavailable) {
		t.Fatalf("missing price error=%v", err)
	}
}

func TestResolveMappedRouteRejectsMappingThatDoesNotMatchRequestedIdentity(t *testing.T) {
	service := newResolverTestService(t, &fakeOverrideRepository{})

	_, err := ResolveMappedRoute(
		context.Background(),
		service,
		"private-upstream-model",
		"gpt-reviewed",
		"test-v1",
		MappingStatusMapped,
	)
	if !errors.Is(err, ErrModelMappingStale) {
		t.Fatalf("tampered route mapping error=%v", err)
	}

	resolved, err := ResolveMappedRoute(
		context.Background(),
		service,
		"gpt-reviewed-alias",
		"gpt-reviewed",
		"test-v1",
		MappingStatusMapped,
	)
	if err != nil || resolved.Model.ModelID != "gpt-reviewed" {
		t.Fatalf("reviewed alias mapping resolved=%#v err=%v", resolved, err)
	}
}
