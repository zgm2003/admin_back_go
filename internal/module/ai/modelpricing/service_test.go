package modelpricing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/shared/clock"
)

func TestAIModelPricingResolveUsesCanonicalIdentityWithoutCaching(t *testing.T) {
	catalog := testPricingCatalog(t, "")
	repository := &fakePricingRepository{}
	service := NewService(repository, WithCatalog(catalog), WithClock(clock.Func(func() time.Time {
		return time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	})))

	first, err := service.Resolve(context.Background(), " gpt-reviewed-alias ")
	if err != nil {
		t.Fatalf("Resolve official: %v", err)
	}
	if first.ModelID != "gpt-reviewed" || first.PriceSource != "official" || first.Version != "catalog-test-v1" || first.CatalogVersion != "catalog-test-v1" {
		t.Fatalf("official price = %#v", first)
	}
	if len(first.Rates) != 2 || first.Rates[0].PriceUnits != 100_000_000 || first.Rates[1].PriceUnits != 200_000_000 {
		t.Fatalf("official rates = %#v", first.Rates)
	}

	repository.override = validStoredOverride()
	second, err := service.Resolve(context.Background(), "gpt-reviewed-alias")
	if err != nil {
		t.Fatalf("Resolve override: %v", err)
	}
	if second.ModelID != "gpt-reviewed" || second.PriceSource != "override" || second.Version != "catalog-test-v1:override:2" || second.CatalogVersion != "catalog-test-v1" || second.OverrideVersion != 2 {
		t.Fatalf("override price = %#v", second)
	}
	if second.SourceURL != "https://openai.com/pricing" || second.RetrievedAt != "2026-07-26" {
		t.Fatalf("override audit metadata = %#v", second)
	}
	if len(second.Rates) != 2 || second.Rates[0].PriceUnits != 125_000_000 || second.Rates[1].PriceUnits != 350_000_000 {
		t.Fatalf("override rates = %#v", second.Rates)
	}
	if repository.findCalls != 2 {
		t.Fatalf("FindOverride calls = %d, want one database read per Resolve", repository.findCalls)
	}
	for _, identity := range repository.findIdentities {
		if identity.vendor != "openai" || identity.modelID != "gpt-reviewed" {
			t.Fatalf("override lookup used non-canonical identity: %#v", identity)
		}
	}
}

func TestAIModelPricingManagementReadModelsFiltersAndFormatsDecimal(t *testing.T) {
	repository := &fakePricingRepository{override: validStoredOverride()}
	service := NewService(repository, WithCatalog(testPricingCatalog(t, "")))

	result, appErr := service.List(context.Background(), ListQuery{Family: "gpt", ModelID: "reviewed"})
	if appErr != nil {
		t.Fatal(appErr)
	}
	if len(result.List) != 1 {
		t.Fatalf("list = %#v", result)
	}
	item := result.List[0]
	if item.ModelID != "gpt-reviewed" || item.ModelFamily != "gpt" || item.Official.Source != "official" || item.Effective.Source != "override" || item.Effective.OverrideVersion != 2 {
		t.Fatalf("model price item = %#v", item)
	}
	if len(item.Official.Rates) != 2 || item.Official.Rates[0].Price != "1" || len(item.Effective.Rates) != 2 || item.Effective.Rates[0].Price != "1.25" {
		t.Fatalf("decimal rates = official %#v effective %#v", item.Official.Rates, item.Effective.Rates)
	}
	if repository.findCalls != 1 {
		t.Fatalf("override reads = %d", repository.findCalls)
	}

	detail, appErr := service.Detail(context.Background(), "gpt-reviewed")
	if appErr != nil || detail.ModelID != "gpt-reviewed" {
		t.Fatalf("detail = %#v, %v", detail, appErr)
	}
	if _, appErr := service.Detail(context.Background(), "gpt-reviewed-alias"); appErr == nil || appErr.Code != ErrorCodeModelNotFound {
		t.Fatalf("alias detail error = %#v", appErr)
	}
}

func TestAIModelPricingManagementReadSerializesEmptyAliasesAsArray(t *testing.T) {
	service := NewService(&fakePricingRepository{}, WithCatalog(testPricingCatalog(t, "")))

	result, appErr := service.List(context.Background(), ListQuery{Family: "gpt", ModelID: "no-alias"})
	if appErr != nil || len(result.List) != 1 {
		t.Fatalf("list = %#v, %v", result, appErr)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal list response: %v", err)
	}
	if !strings.Contains(string(payload), `"aliases":[]`) {
		t.Fatalf("aliases must satisfy the array response contract: %s", payload)
	}
}

func TestAIModelPricingReviewAfterOnlyBlocksOfficialFallback(t *testing.T) {
	catalog := testPricingCatalog(t, "2026-07-27")
	repository := &fakePricingRepository{}
	service := NewService(repository, WithCatalog(catalog), WithClock(clock.Func(func() time.Time {
		return time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	})))

	if _, err := service.Resolve(context.Background(), "gpt-reviewed-alias"); !errors.Is(err, ErrOfficialPriceExpired) || !errors.Is(err, pricing.ErrPriceUnavailable) {
		t.Fatalf("expired official fallback error = %v", err)
	}
	if repository.findCalls != 1 || repository.findIdentities[0].modelID != "gpt-reviewed" {
		t.Fatalf("identity must resolve before review_after: calls=%d identities=%#v", repository.findCalls, repository.findIdentities)
	}

	repository.override = validStoredOverride()
	resolved, err := service.Resolve(context.Background(), "gpt-reviewed-alias")
	if err != nil || resolved.PriceSource != "override" || resolved.Version != "catalog-test-v1:override:2" {
		t.Fatalf("valid override after review_after = %#v, %v", resolved, err)
	}
	if summary, appErr := service.RestoreOfficial(context.Background(), "gpt-reviewed", 2, 7); summary != nil || appErr == nil || appErr.Code != ErrorCodeInvalidOverride || !errors.Is(appErr, ErrOfficialPriceExpired) {
		t.Fatalf("restore to expired official price = %#v, %#v", summary, appErr)
	}
	if repository.deleteWrites != 0 || repository.override == nil || repository.override.Version != 2 {
		t.Fatalf("expired official restore crossed write boundary: writes=%d stored=%#v", repository.deleteWrites, repository.override)
	}
}

func TestAIModelPricingResolveFailsClosedForStoredCorruption(t *testing.T) {
	mutations := map[string]func(*PriceOverride){
		"missing rate": func(row *PriceOverride) { row.Rates = row.Rates[:1] },
		"duplicate rate": func(row *PriceOverride) {
			row.Rates = append(row.Rates, row.Rates[0])
		},
		"additional rate": func(row *PriceOverride) {
			row.Rates = append(row.Rates, PriceOverrideRate{OverrideID: row.ID, Category: pricing.CacheRead, Unit: "token", TierKey: "", PriceUnits: 1, UnitScale: 1_000_000})
		},
		"wrong unit scale": func(row *PriceOverride) { row.Rates[0].UnitScale = 1 },
		"negative price":   func(row *PriceOverride) { row.Rates[0].PriceUnits = -1 },
		"invalid url":      func(row *PriceOverride) { row.SourceURL = "https://example.test/pricing" },
		"wrong vendor url": func(row *PriceOverride) { row.SourceURL = "https://anthropic.com/pricing" },
		"wrong identity":   func(row *PriceOverride) { row.ModelID = "another-model" },
		"zero version":     func(row *PriceOverride) { row.Version = 0 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			row := validStoredOverride()
			mutate(row)
			service := NewService(&fakePricingRepository{override: row}, WithCatalog(testPricingCatalog(t, "")))
			price, err := service.Resolve(context.Background(), "gpt-reviewed")
			if !errors.Is(err, ErrInvalidOverride) {
				t.Fatalf("Resolve corrupted override = %#v, %v", price, err)
			}
			if price.PriceSource == "official" {
				t.Fatalf("corrupted override fell back to official: %#v", price)
			}
		})
	}

	dependencyErr := errors.New("database unavailable")
	if _, err := NewService(&fakePricingRepository{findErr: dependencyErr}, WithCatalog(testPricingCatalog(t, ""))).Resolve(context.Background(), "gpt-reviewed"); !errors.Is(err, dependencyErr) {
		t.Fatalf("database error was hidden: %v", err)
	}
	if _, err := NewService(&fakePricingRepository{findErr: ErrOverrideMappingAmbiguous}, WithCatalog(testPricingCatalog(t, ""))).Resolve(context.Background(), "gpt-reviewed"); !errors.Is(err, ErrOverrideMappingAmbiguous) {
		t.Fatalf("mapping ambiguity was hidden: %v", err)
	}
}

func TestAIModelPricingSetOverrideRequiresCompleteReviewedPrices(t *testing.T) {
	repository := &fakePricingRepository{}
	service := NewService(repository, WithCatalog(testPricingCatalog(t, "")))
	input := validSetOverrideInput(0)

	summary, appErr := service.SetOverride(context.Background(), "gpt-reviewed", input)
	if appErr != nil {
		t.Fatalf("SetOverride: %v", appErr)
	}
	if repository.replaceCalls != 1 || repository.lastReplace.CatalogVendor != "openai" || repository.lastReplace.ModelID != "gpt-reviewed" || repository.lastReplace.ExpectedVersion != 0 {
		t.Fatalf("replace command = %#v", repository.lastReplace)
	}
	if len(repository.lastReplace.Rates) != 2 || repository.lastReplace.Rates[0].PriceUnits != 125_000_000 || repository.lastReplace.Rates[1].PriceUnits != 350_000_000 {
		t.Fatalf("parsed decimal rates = %#v", repository.lastReplace.Rates)
	}
	if summary.Before.PriceSource != "official" || summary.Before.OverrideVersion != 0 || summary.After.PriceSource != "override" || summary.After.OverrideVersion != 1 {
		t.Fatalf("mutation summary = %#v", summary)
	}
	if summary.After.Version != "catalog-test-v1:override:1" || len(summary.After.Rates) != 2 {
		t.Fatalf("closed after summary = %#v", summary.After)
	}

	tests := map[string]struct {
		modelID string
		mutate  func(*SetOverrideInput)
		code    string
	}{
		"missing rate": {modelID: "gpt-reviewed", mutate: func(in *SetOverrideInput) { in.Prices = in.Prices[:1] }, code: ErrorCodeInvalidOverride},
		"duplicate rate": {modelID: "gpt-reviewed", mutate: func(in *SetOverrideInput) {
			in.Prices = append(in.Prices, in.Prices[0])
		}, code: ErrorCodeInvalidOverride},
		"additional rate": {modelID: "gpt-reviewed", mutate: func(in *SetOverrideInput) {
			in.Prices = append(in.Prices, RatePriceInput{Category: pricing.CacheRead, Unit: "token", UnitScale: 1_000_000, Price: "0.1"})
		}, code: ErrorCodeInvalidOverride},
		"wrong unit":                         {modelID: "gpt-reviewed", mutate: func(in *SetOverrideInput) { in.Prices[0].Unit = "request" }, code: ErrorCodeInvalidOverride},
		"wrong unit scale":                   {modelID: "gpt-reviewed", mutate: func(in *SetOverrideInput) { in.Prices[0].UnitScale = 1 }, code: ErrorCodeInvalidOverride},
		"negative":                           {modelID: "gpt-reviewed", mutate: func(in *SetOverrideInput) { in.Prices[0].Price = "-1" }, code: ErrorCodeInvalidOverride},
		"too precise":                        {modelID: "gpt-reviewed", mutate: func(in *SetOverrideInput) { in.Prices[0].Price = "0.123456789" }, code: ErrorCodeInvalidOverride},
		"invalid url":                        {modelID: "gpt-reviewed", mutate: func(in *SetOverrideInput) { in.SourceURL = "https://example.test/pricing" }, code: ErrorCodeInvalidOverride},
		"wrong vendor url":                   {modelID: "gpt-reviewed", mutate: func(in *SetOverrideInput) { in.SourceURL = "https://anthropic.com/pricing" }, code: ErrorCodeInvalidOverride},
		"custom source port":                 {modelID: "gpt-reviewed", mutate: func(in *SetOverrideInput) { in.SourceURL = "https://openai.com:8443/pricing" }, code: ErrorCodeInvalidOverride},
		"source url exceeds database column": {modelID: "gpt-reviewed", mutate: func(in *SetOverrideInput) { in.SourceURL = "https://openai.com/" + strings.Repeat("a", 2048) }, code: ErrorCodeInvalidOverride},
		"invalid verified date":              {modelID: "gpt-reviewed", mutate: func(in *SetOverrideInput) { in.VerifiedAt = "2026-7-27" }, code: ErrorCodeInvalidOverride},
		"verified date outside database range": {modelID: "gpt-reviewed", mutate: func(in *SetOverrideInput) {
			in.VerifiedAt = "0999-12-31"
		}, code: ErrorCodeInvalidOverride},
		"missing administrator": {modelID: "gpt-reviewed", mutate: func(in *SetOverrideInput) { in.AdministratorID = 0 }, code: ErrorCodeInvalidOverride},
		"administrator exceeds database column": {modelID: "gpt-reviewed", mutate: func(in *SetOverrideInput) {
			in.AdministratorID = uint64(1) << 32
		}, code: ErrorCodeInvalidOverride},
		"alias is not a management identity": {modelID: "gpt-reviewed-alias", mutate: func(*SetOverrideInput) {}, code: ErrorCodeModelNotFound},
		"unknown model":                      {modelID: "unknown", mutate: func(*SetOverrideInput) {}, code: ErrorCodeModelNotFound},
		"non-reviewed model":                 {modelID: "gpt-image-test", mutate: func(*SetOverrideInput) {}, code: ErrorCodeModelNotFound},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			repository := &fakePricingRepository{}
			service := NewService(repository, WithCatalog(testPricingCatalog(t, "")))
			input := validSetOverrideInput(0)
			test.mutate(&input)
			if summary, appErr := service.SetOverride(context.Background(), test.modelID, input); appErr == nil || appErr.Code != test.code || summary != nil {
				t.Fatalf("SetOverride = %#v, %#v; want code %q", summary, appErr, test.code)
			}
			if repository.replaceCalls != 0 {
				t.Fatalf("invalid request reached repository: %d calls", repository.replaceCalls)
			}
		})
	}
}

func TestAIModelPricingOptimisticVersionRules(t *testing.T) {
	t.Run("create only accepts zero and starts at one", func(t *testing.T) {
		repository := &fakePricingRepository{}
		service := NewService(repository, WithCatalog(testPricingCatalog(t, "")))
		summary, appErr := service.SetOverride(context.Background(), "gpt-reviewed", validSetOverrideInput(0))
		if appErr != nil || summary.After.OverrideVersion != 1 || repository.override.Version != 1 {
			t.Fatalf("create = %#v, %#v, stored=%#v", summary, appErr, repository.override)
		}
	})

	for name, test := range map[string]struct {
		existing *PriceOverride
		expected int64
	}{
		"create against existing": {existing: validStoredOverride(), expected: 0},
		"update against missing":  {existing: nil, expected: 2},
		"stale update":            {existing: validStoredOverride(), expected: 1},
		"negative version":        {existing: validStoredOverride(), expected: -1},
	} {
		t.Run(name, func(t *testing.T) {
			repository := &fakePricingRepository{override: cloneOverrideForTest(test.existing)}
			service := NewService(repository, WithCatalog(testPricingCatalog(t, "")))
			input := validSetOverrideInput(test.expected)
			if summary, appErr := service.SetOverride(context.Background(), "gpt-reviewed", input); summary != nil || appErr == nil || appErr.Code != ErrorCodeVersionConflict || !errors.Is(appErr, ErrVersionConflict) {
				t.Fatalf("version conflict = %#v, %#v", summary, appErr)
			}
		})
	}

	repository := &fakePricingRepository{override: validStoredOverride()}
	service := NewService(repository, WithCatalog(testPricingCatalog(t, "")))
	updated, appErr := service.SetOverride(context.Background(), "gpt-reviewed", validSetOverrideInput(2))
	if appErr != nil || updated.Before.OverrideVersion != 2 || updated.After.OverrideVersion != 3 || repository.override.Version != 3 {
		t.Fatalf("update = %#v, %#v, stored=%#v", updated, appErr, repository.override)
	}

	for _, expected := range []int64{0, -1, 2} {
		repository := &fakePricingRepository{override: validStoredOverride()}
		if expected == 2 {
			repository.override.Version = 3
		}
		service := NewService(repository, WithCatalog(testPricingCatalog(t, "")))
		if summary, appErr := service.RestoreOfficial(context.Background(), "gpt-reviewed", expected, 7); summary != nil || appErr == nil || appErr.Code != ErrorCodeVersionConflict || !errors.Is(appErr, ErrVersionConflict) {
			t.Fatalf("RestoreOfficial(%d) = %#v, %#v", expected, summary, appErr)
		}
	}

	repository = &fakePricingRepository{override: validStoredOverride()}
	service = NewService(repository, WithCatalog(testPricingCatalog(t, "")))
	restored, appErr := service.RestoreOfficial(context.Background(), "gpt-reviewed", 2, 7)
	if appErr != nil || restored.Before.OverrideVersion != 2 || restored.After.OverrideVersion != 0 || restored.After.PriceSource != "official" || repository.override != nil {
		t.Fatalf("restore = %#v, %#v, stored=%#v", restored, appErr, repository.override)
	}
}

func TestAIModelPricingMutationsRejectCorruptedExistingOverrideBeforeWrite(t *testing.T) {
	corrupted := validStoredOverride()
	corrupted.Rates = corrupted.Rates[:1]

	t.Run("replace", func(t *testing.T) {
		repository := &fakePricingRepository{override: cloneOverrideForTest(corrupted)}
		service := NewService(repository, WithCatalog(testPricingCatalog(t, "")))
		summary, appErr := service.SetOverride(context.Background(), "gpt-reviewed", validSetOverrideInput(2))
		if summary != nil || appErr == nil || appErr.Code != ErrorCodeInvalidOverride || !errors.Is(appErr, ErrInvalidOverride) {
			t.Fatalf("SetOverride corrupted existing = %#v, %#v", summary, appErr)
		}
		if repository.replaceWrites != 0 || repository.override == nil || repository.override.Version != 2 || len(repository.override.Rates) != 1 {
			t.Fatalf("corrupted replace crossed write boundary: writes=%d stored=%#v", repository.replaceWrites, repository.override)
		}
	})

	t.Run("restore", func(t *testing.T) {
		repository := &fakePricingRepository{override: cloneOverrideForTest(corrupted)}
		service := NewService(repository, WithCatalog(testPricingCatalog(t, "")))
		summary, appErr := service.RestoreOfficial(context.Background(), "gpt-reviewed", 2, 7)
		if summary != nil || appErr == nil || appErr.Code != ErrorCodeInvalidOverride || !errors.Is(appErr, ErrInvalidOverride) {
			t.Fatalf("RestoreOfficial corrupted existing = %#v, %#v", summary, appErr)
		}
		if repository.deleteWrites != 0 || repository.override == nil || repository.override.Version != 2 || len(repository.override.Rates) != 1 {
			t.Fatalf("corrupted restore crossed write boundary: writes=%d stored=%#v", repository.deleteWrites, repository.override)
		}
	})
}

type pricingIdentity struct {
	vendor  string
	modelID string
}

type fakePricingRepository struct {
	override       *PriceOverride
	findErr        error
	findCalls      int
	findIdentities []pricingIdentity
	replaceCalls   int
	replaceWrites  int
	lastReplace    ReplaceOverrideCommand
	deleteCalls    int
	deleteWrites   int
}

func (repository *fakePricingRepository) FindOverride(_ context.Context, vendor string, modelID string) (*PriceOverride, error) {
	repository.findCalls++
	repository.findIdentities = append(repository.findIdentities, pricingIdentity{vendor: vendor, modelID: modelID})
	return cloneOverrideForTest(repository.override), repository.findErr
}

func (repository *fakePricingRepository) ReplaceOverride(_ context.Context, command ReplaceOverrideCommand, validateExisting ExistingOverrideValidator) (*PriceOverride, *PriceOverride, error) {
	repository.replaceCalls++
	repository.lastReplace = command
	current := cloneOverrideForTest(repository.override)
	if command.ExpectedVersion == 0 {
		if current != nil {
			return nil, nil, ErrVersionConflict
		}
	} else if current == nil || current.Version != command.ExpectedVersion {
		return nil, nil, ErrVersionConflict
	}
	if current != nil && validateExisting != nil {
		if err := validateExisting(cloneOverrideForTest(current)); err != nil {
			return nil, nil, err
		}
	}
	version := uint64(1)
	id := uint64(9)
	createdAt := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	if current != nil {
		version = current.Version + 1
		id = current.ID
		createdAt = current.CreatedAt
	}
	after := &PriceOverride{
		ID: id, CatalogVendor: command.CatalogVendor, ModelID: command.ModelID, Version: version,
		SourceURL: command.SourceURL, VerifiedAt: command.VerifiedAt, UpdatedBy: command.UpdatedBy,
		CreatedAt: createdAt, UpdatedAt: time.Date(2026, 7, 27, 1, 0, 0, 0, time.UTC),
		Rates: append([]PriceOverrideRate(nil), command.Rates...),
	}
	for index := range after.Rates {
		after.Rates[index].OverrideID = id
	}
	repository.override = cloneOverrideForTest(after)
	repository.replaceWrites++
	return current, cloneOverrideForTest(after), nil
}

func (repository *fakePricingRepository) DeleteOverride(_ context.Context, command DeleteOverrideCommand, validateExisting ExistingOverrideValidator) (*PriceOverride, error) {
	repository.deleteCalls++
	current := cloneOverrideForTest(repository.override)
	if command.ExpectedVersion == 0 || current == nil || current.Version != command.ExpectedVersion {
		return nil, ErrVersionConflict
	}
	if validateExisting != nil {
		if err := validateExisting(cloneOverrideForTest(current)); err != nil {
			return nil, err
		}
	}
	repository.override = nil
	repository.deleteWrites++
	return current, nil
}

func testPricingCatalog(t *testing.T, reviewAfter string) *pricing.Catalog {
	t.Helper()
	catalog, err := pricing.NewCatalogChecked([]pricing.ModelPrice{
		{
			Version: "catalog-test-v1", CatalogVersion: "catalog-test-v1", CatalogVendor: "openai", ModelFamily: "gpt",
			ModelID: "gpt-reviewed", Aliases: []string{"gpt-reviewed-alias"}, PricingProfile: "standard_global", MaxOutputTokens: 4096,
			PriceSource: "official", SourceURL: "https://openai.com/api/pricing", RetrievedAt: "2026-07-26", ReviewAfter: reviewAfter,
			Rates: []pricing.Rate{
				{Category: pricing.InputTokens, Unit: "token", TierKey: "", PriceUnits: 100_000_000, UnitScale: 1_000_000},
				{Category: pricing.OutputTokens, Unit: "token", TierKey: "", PriceUnits: 200_000_000, UnitScale: 1_000_000},
			},
		},
		{
			Version: "catalog-test-v1", CatalogVersion: "catalog-test-v1", CatalogVendor: "openai", ModelFamily: "image",
			ModelID: "gpt-image-test", MaxOutputTokens: 1, PriceSource: "official", SourceURL: "https://openai.com/api/pricing", RetrievedAt: "2026-07-26",
			Rates: []pricing.Rate{{Category: pricing.MediaUnits, Unit: "image", PriceUnits: 1, UnitScale: 1}},
		},
		{
			Version: "catalog-test-v1", CatalogVersion: "catalog-test-v1", CatalogVendor: "openai", ModelFamily: "gpt",
			ModelID: "gpt-no-alias", Aliases: []string{}, PricingProfile: "standard_global", MaxOutputTokens: 4096,
			PriceSource: "official", SourceURL: "https://openai.com/api/pricing", RetrievedAt: "2026-07-26",
			Rates: []pricing.Rate{{Category: pricing.InputTokens, Unit: "token", PriceUnits: 100_000_000, UnitScale: 1_000_000}},
		},
	})
	if err != nil {
		t.Fatalf("construct pricing catalog: %v", err)
	}
	return catalog
}

func validStoredOverride() *PriceOverride {
	return &PriceOverride{
		ID: 9, CatalogVendor: "openai", ModelID: "gpt-reviewed", Version: 2,
		SourceURL: "https://openai.com/pricing", VerifiedAt: time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC), UpdatedBy: 7,
		CreatedAt: time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 7, 26, 1, 0, 0, 0, time.UTC),
		Rates: []PriceOverrideRate{
			{ID: 1, OverrideID: 9, Category: pricing.InputTokens, Unit: "token", TierKey: "", PriceUnits: 125_000_000, UnitScale: 1_000_000},
			{ID: 2, OverrideID: 9, Category: pricing.OutputTokens, Unit: "token", TierKey: "", PriceUnits: 350_000_000, UnitScale: 1_000_000},
		},
	}
}

func validSetOverrideInput(expectedVersion int64) SetOverrideInput {
	return SetOverrideInput{
		ExpectedVersion: expectedVersion,
		Prices: []RatePriceInput{
			{Category: pricing.InputTokens, Unit: "token", TierKey: "", UnitScale: 1_000_000, Price: "1.25"},
			{Category: pricing.OutputTokens, Unit: "token", TierKey: "", UnitScale: 1_000_000, Price: "3.5"},
		},
		SourceURL: "https://openai.com/pricing", VerifiedAt: "2026-07-27", AdministratorID: 7,
	}
}

func cloneOverrideForTest(row *PriceOverride) *PriceOverride {
	if row == nil {
		return nil
	}
	clone := *row
	clone.Rates = append([]PriceOverrideRate(nil), row.Rates...)
	return &clone
}
