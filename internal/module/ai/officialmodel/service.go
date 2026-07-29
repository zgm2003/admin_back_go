package officialmodel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/shared/clock"
)

type Service struct {
	repository Repository
	catalog    *Catalog
	clock      clock.Clock
}

type Option func(*Service)

func WithCatalog(catalog *Catalog) Option {
	return func(service *Service) {
		if catalog != nil {
			service.catalog = catalog
		}
	}
}

func WithClock(value clock.Clock) Option {
	return func(service *Service) {
		if value != nil {
			service.clock = value
		}
	}
}

func NewService(repository Repository, options ...Option) *Service {
	service := &Service{repository: repository, catalog: Default, clock: clock.SystemClock{}}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (service *Service) Resolve(ctx context.Context, requestedModelID string) (ResolvedModel, error) {
	if service == nil || service.catalog == nil {
		return ResolvedModel{}, errors.Join(ErrModelUnmapped, ErrInvalidCatalog)
	}
	model, err := service.catalog.ResolveIdentity(requestedModelID)
	if err != nil {
		return ResolvedModel{}, fmt.Errorf("resolve official model identity: %w", err)
	}
	if err := validatePriceBook(model, model.OfficialPrice); err != nil {
		return ResolvedModel{}, err
	}
	if service.repository == nil {
		return ResolvedModel{}, ErrRepositoryNotConfigured
	}
	override, err := service.repository.FindOverride(ctx, model.CatalogVendor, model.ModelID)
	if err != nil {
		return ResolvedModel{}, fmt.Errorf("find official model price override: %w", err)
	}
	if override != nil {
		return resolvedFromOverride(model, override)
	}
	if err := service.validateOfficialReview(model); err != nil {
		return ResolvedModel{}, err
	}
	verifiedAt, err := time.Parse(time.DateOnly, model.RetrievedAt)
	if err != nil {
		return ResolvedModel{}, errors.Join(ErrPriceUnavailable, ErrInvalidCatalog)
	}
	return ResolvedModel{
		Model: cloneModel(model), EffectivePrice: clonePriceBook(model.OfficialPrice),
		PriceSource: PriceSourceOfficial, PriceSourceURL: model.PricingSourceURL, PriceVerifiedAt: verifiedAt,
	}, nil
}

func resolvedFromOverride(model Model, override *PriceOverride) (ResolvedModel, error) {
	if override == nil || override.ID == 0 || override.CatalogVendor != model.CatalogVendor || override.ModelID != model.ModelID ||
		override.Version == 0 || override.UpdatedBy == 0 || override.VerifiedAt.IsZero() ||
		!validOfficialSource(model.CatalogVendor, override.SourceURL) {
		return ResolvedModel{}, fmt.Errorf("%w: invalid override header", ErrInvalidOverride)
	}
	if len(override.Rates) != len(model.OfficialPrice.Rates) {
		return ResolvedModel{}, fmt.Errorf("%w: incomplete rate set", ErrInvalidOverride)
	}
	overrideRates := make(map[string]PriceOverrideRate, len(override.Rates))
	for _, rate := range override.Rates {
		key := rateIdentity(rate.Category, rate.Unit, rate.TierKey)
		if rate.OverrideID != override.ID || !validRateCategory(rate.Category) || strings.TrimSpace(rate.Unit) == "" ||
			rate.Unit != strings.TrimSpace(rate.Unit) || rate.TierKey != strings.TrimSpace(rate.TierKey) ||
			rate.UnitScale <= 0 || rate.PriceUnits < 0 {
			return ResolvedModel{}, fmt.Errorf("%w: invalid override rate", ErrInvalidOverride)
		}
		if _, duplicate := overrideRates[key]; duplicate {
			return ResolvedModel{}, fmt.Errorf("%w: duplicate override rate", ErrInvalidOverride)
		}
		overrideRates[key] = rate
	}
	effective := pricing.PriceBook{
		ModelID: model.ModelID, ContextTierThresholdTokens: model.ContextTierThresholdTokens,
		Rates: make([]pricing.Rate, 0, len(model.OfficialPrice.Rates)),
	}
	for _, officialRate := range model.OfficialPrice.Rates {
		overrideRate, exists := overrideRates[rateIdentity(officialRate.Category, officialRate.Unit, officialRate.TierKey)]
		if !exists || overrideRate.UnitScale != officialRate.UnitScale {
			return ResolvedModel{}, fmt.Errorf("%w: override rate shape differs from catalog", ErrInvalidOverride)
		}
		effective.Rates = append(effective.Rates, pricing.Rate{
			Category: officialRate.Category, Unit: officialRate.Unit, TierKey: officialRate.TierKey,
			PriceUnits: overrideRate.PriceUnits, UnitScale: officialRate.UnitScale,
		})
	}
	if err := validatePriceBook(model, effective); err != nil {
		return ResolvedModel{}, errors.Join(ErrInvalidOverride, err)
	}
	return ResolvedModel{
		Model: cloneModel(model), EffectivePrice: effective, PriceSource: PriceSourceOverride,
		OverrideVersion: override.Version, PriceSourceURL: override.SourceURL, PriceVerifiedAt: override.VerifiedAt,
	}, nil
}

func validatePriceBook(model Model, book pricing.PriceBook) error {
	if book.ModelID != model.ModelID || book.ContextTierThresholdTokens != model.ContextTierThresholdTokens || len(book.Rates) == 0 {
		return ErrPriceUnavailable
	}
	positive := false
	seen := make(map[string]struct{}, len(book.Rates))
	for _, rate := range book.Rates {
		if !validRateCategory(rate.Category) || strings.TrimSpace(rate.Unit) == "" || rate.UnitScale <= 0 || rate.PriceUnits < 0 {
			return ErrPriceUnavailable
		}
		key := rateIdentity(rate.Category, rate.Unit, rate.TierKey)
		if _, duplicate := seen[key]; duplicate {
			return ErrPriceUnavailable
		}
		seen[key] = struct{}{}
		positive = positive || rate.PriceUnits > 0
	}
	if !positive {
		return ErrPriceUnavailable
	}
	return nil
}

func (service *Service) validateOfficialReview(model Model) error {
	reviewAfter, err := time.Parse(time.DateOnly, model.ReviewAfter)
	if err != nil {
		return errors.Join(ErrPriceUnavailable, ErrInvalidCatalog)
	}
	now := time.Now()
	if service != nil && service.clock != nil {
		now = service.clock.Now()
	}
	if now.UTC().After(reviewAfter.Add(24*time.Hour - time.Nanosecond)) {
		return ErrOfficialPriceExpired
	}
	return nil
}

func clonePriceBook(book pricing.PriceBook) pricing.PriceBook {
	book.Rates = append([]pricing.Rate(nil), book.Rates...)
	return book
}

func rateIdentity(category pricing.Category, unit string, tierKey string) string {
	return string(category) + "\x00" + unit + "\x00" + tierKey
}

var _ Resolver = (*Service)(nil)
var _ Repository = (*GormRepository)(nil)
