package modelpricing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/clock"
	"admin_back_go/internal/shared/money"
)

const (
	ErrorCodeInvalidOverride = "ai.model_pricing.invalid_override"
	ErrorCodeVersionConflict = "ai.model_pricing.version_conflict"
	ErrorCodeModelNotFound   = "ai.model_pricing.model_not_found"

	maxSourceURLCharacters     = 2048
	minMySQLDateYear           = 1000
	maxMySQLDateYear           = 9999
	maxDatabaseAdministratorID = uint64(1<<32 - 1)
)

var (
	ErrInvalidOverride      = errors.New("invalid model pricing override")
	ErrModelNotFound        = errors.New("managed model pricing identity not found")
	ErrOfficialPriceExpired = errors.New("official model price requires review")
)

type Resolver interface {
	Resolve(context.Context, string) (pricing.ModelPrice, error)
}

type ResolverFunc func(context.Context, string) (pricing.ModelPrice, error)

func (resolve ResolverFunc) Resolve(ctx context.Context, modelID string) (pricing.ModelPrice, error) {
	if resolve == nil {
		return pricing.ModelPrice{}, ErrRepositoryNotConfigured
	}
	return resolve(ctx, modelID)
}

type RatePriceInput struct {
	Category  pricing.Category
	Unit      string
	TierKey   string
	UnitScale int64
	Price     string
}

type SetOverrideInput struct {
	ExpectedVersion int64
	Prices          []RatePriceInput
	SourceURL       string
	VerifiedAt      string
	AdministratorID uint64
}

type PriceSummary struct {
	pricing.ModelPrice
}

type MutationSummary struct {
	Before PriceSummary
	After  PriceSummary
}

type Service struct {
	repository Repository
	catalog    *pricing.Catalog
	clock      clock.Clock
}

type Option func(*Service)

func WithCatalog(catalog *pricing.Catalog) Option {
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
	service := &Service{repository: repository, catalog: pricing.Default, clock: clock.SystemClock{}}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (service *Service) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, internalPricingError(err)
		}
	}
	return &PageInitResponse{Dict: PageInitDict{FamilyOptions: []OptionDTO{
		{Label: "GPT", Value: "gpt"},
		{Label: "Claude", Value: "claude"},
	}}}, nil
}

func (service *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	if service == nil || service.catalog == nil || service.repository == nil {
		return nil, internalPricingError(ErrRepositoryNotConfigured)
	}
	family := strings.TrimSpace(query.Family)
	if family != "" && family != "gpt" && family != "claude" {
		return nil, invalidOverrideError(fmt.Errorf("%w: invalid model family", ErrInvalidOverride))
	}
	needle := strings.ToLower(strings.TrimSpace(query.ModelID))
	models := service.managedModels()
	items := make([]ModelPriceDTO, 0, len(models))
	for _, official := range models {
		if family != "" && official.ModelFamily != family {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(official.ModelID), needle) {
			continue
		}
		item, err := service.managementModel(ctx, official)
		if err != nil {
			return nil, internalPricingError(err)
		}
		items = append(items, item)
	}
	return &ListResponse{List: items}, nil
}

func (service *Service) Detail(ctx context.Context, modelID string) (*ModelPriceDTO, *apperror.Error) {
	official, appErr := service.resolveManagedModel(modelID)
	if appErr != nil {
		return nil, appErr
	}
	item, err := service.managementModel(ctx, official)
	if err != nil {
		return nil, internalPricingError(err)
	}
	return &item, nil
}

func (service *Service) managedModels() []pricing.ModelPrice {
	if service == nil || service.catalog == nil {
		return nil
	}
	models := service.catalog.Models()
	managed := make([]pricing.ModelPrice, 0, len(models))
	for _, model := range models {
		model = officialPrice(model)
		if isReviewedManagedModel(model) {
			managed = append(managed, model)
		}
	}
	sort.Slice(managed, func(i, j int) bool {
		if managed[i].ModelFamily != managed[j].ModelFamily {
			return managed[i].ModelFamily < managed[j].ModelFamily
		}
		return managed[i].ModelID < managed[j].ModelID
	})
	return managed
}

func (service *Service) managementModel(ctx context.Context, official pricing.ModelPrice) (ModelPriceDTO, error) {
	if service == nil || service.repository == nil {
		return ModelPriceDTO{}, ErrRepositoryNotConfigured
	}
	override, err := service.repository.FindOverride(ctx, official.CatalogVendor, official.ModelID)
	if err != nil {
		return ModelPriceDTO{}, err
	}
	effective := officialPrice(official)
	if override != nil {
		effective, err = priceFromOverride(official, override)
		if err != nil {
			return ModelPriceDTO{}, err
		}
	}
	officialAvailable := service.validateOfficialReview(official) == nil
	officialDTO, err := priceDTO(officialPrice(official), officialAvailable)
	if err != nil {
		return ModelPriceDTO{}, err
	}
	effectiveDTO, err := priceDTO(effective, effective.PriceSource == "override" || officialAvailable)
	if err != nil {
		return ModelPriceDTO{}, err
	}
	return ModelPriceDTO{
		CatalogVendor: official.CatalogVendor, ModelFamily: official.ModelFamily, ModelID: official.ModelID,
		Aliases: append([]string(nil), official.Aliases...), PricingProfile: official.PricingProfile,
		CatalogVersion: official.CatalogVersion, MaxOutputTokens: official.MaxOutputTokens,
		ContextTierThresholdTokens: official.ContextTierThresholdTokens, ReviewAfter: official.ReviewAfter,
		Official: officialDTO, Effective: effectiveDTO,
	}, nil
}

func priceDTO(model pricing.ModelPrice, available bool) (PriceDTO, error) {
	rates := make([]RateDTO, len(model.Rates))
	for index, rate := range model.Rates {
		formatted, err := money.FormatRMBUnits(rate.PriceUnits)
		if err != nil {
			return PriceDTO{}, err
		}
		rates[index] = RateDTO{Category: rate.Category, Unit: rate.Unit, TierKey: rate.TierKey, Price: formatted, UnitScale: rate.UnitScale}
	}
	return PriceDTO{
		PricingVersion: model.Version, Source: model.PriceSource, OverrideVersion: model.OverrideVersion,
		SourceURL: model.SourceURL, VerifiedAt: model.RetrievedAt, Available: available, Rates: rates,
	}, nil
}

func MutationResponseFromSummary(summary *MutationSummary) (*MutationResponse, error) {
	if summary == nil {
		return nil, errors.New("model pricing mutation summary is missing")
	}
	before, err := priceDTO(summary.Before.ModelPrice, true)
	if err != nil {
		return nil, err
	}
	after, err := priceDTO(summary.After.ModelPrice, true)
	if err != nil {
		return nil, err
	}
	return &MutationResponse{Before: before, After: after}, nil
}

func (service *Service) Resolve(ctx context.Context, requestedModelID string) (pricing.ModelPrice, error) {
	official, err := service.resolveOfficial(requestedModelID)
	if err != nil {
		return pricing.ModelPrice{}, err
	}
	if service == nil || service.repository == nil {
		return pricing.ModelPrice{}, ErrRepositoryNotConfigured
	}
	override, err := service.repository.FindOverride(ctx, official.CatalogVendor, official.ModelID)
	if err != nil {
		return pricing.ModelPrice{}, fmt.Errorf("find model price override: %w", err)
	}
	if override != nil {
		resolved, validationErr := priceFromOverride(official, override)
		if validationErr != nil {
			return pricing.ModelPrice{}, validationErr
		}
		return resolved, nil
	}
	if err := service.validateOfficialReview(official); err != nil {
		return pricing.ModelPrice{}, err
	}
	return officialPrice(official), nil
}

func (service *Service) SetOverride(ctx context.Context, modelID string, input SetOverrideInput) (*MutationSummary, *apperror.Error) {
	official, appErr := service.resolveManagedModel(modelID)
	if appErr != nil {
		return nil, appErr
	}
	if input.ExpectedVersion < 0 {
		return nil, versionConflictError(ErrVersionConflict)
	}
	command, err := buildReplaceCommand(official, input)
	if err != nil {
		return nil, invalidOverrideError(err)
	}
	if service == nil || service.repository == nil {
		return nil, internalPricingError(ErrRepositoryNotConfigured)
	}
	before, after, err := service.repository.ReplaceOverride(ctx, command, validateStoredOverride(official))
	if err != nil {
		return nil, mutationRepositoryError(err)
	}
	summary, err := mutationSummary(official, before, after)
	if err != nil {
		return nil, invalidOverrideError(err)
	}
	return summary, nil
}

func (service *Service) RestoreOfficial(ctx context.Context, modelID string, expectedVersion int64, administratorID uint64) (*MutationSummary, *apperror.Error) {
	official, appErr := service.resolveManagedModel(modelID)
	if appErr != nil {
		return nil, appErr
	}
	if expectedVersion <= 0 {
		return nil, versionConflictError(ErrVersionConflict)
	}
	if !validAdministratorID(administratorID) {
		return nil, invalidOverrideError(fmt.Errorf("%w: administrator is required", ErrInvalidOverride))
	}
	if err := service.validateOfficialReview(official); err != nil {
		return nil, invalidOverrideError(err)
	}
	if service == nil || service.repository == nil {
		return nil, internalPricingError(ErrRepositoryNotConfigured)
	}
	before, err := service.repository.DeleteOverride(ctx, DeleteOverrideCommand{
		CatalogVendor: official.CatalogVendor, ModelID: official.ModelID, ExpectedVersion: uint64(expectedVersion),
	}, validateStoredOverride(official))
	if err != nil {
		return nil, mutationRepositoryError(err)
	}
	beforePrice, err := priceFromOverride(official, before)
	if err != nil {
		return nil, invalidOverrideError(err)
	}
	return &MutationSummary{
		Before: PriceSummary{ModelPrice: beforePrice},
		After:  PriceSummary{ModelPrice: officialPrice(official)},
	}, nil
}

func (service *Service) resolveOfficial(identity string) (pricing.ModelPrice, error) {
	if service == nil || service.catalog == nil {
		return pricing.ModelPrice{}, errors.Join(pricing.ErrPriceUnavailable, pricing.ErrInvalidCatalog)
	}
	model, err := service.catalog.Resolve(identity)
	if err != nil {
		return pricing.ModelPrice{}, fmt.Errorf("resolve official model identity: %w", errors.Join(pricing.ErrPriceUnavailable, err))
	}
	return officialPrice(model), nil
}

func (service *Service) resolveManagedModel(modelID string) (pricing.ModelPrice, *apperror.Error) {
	if modelID == "" || modelID != strings.TrimSpace(modelID) {
		return pricing.ModelPrice{}, modelNotFoundError(ErrModelNotFound)
	}
	official, err := service.resolveOfficial(modelID)
	if err != nil || official.ModelID != modelID || !isReviewedManagedModel(official) {
		return pricing.ModelPrice{}, modelNotFoundError(errors.Join(ErrModelNotFound, err))
	}
	return official, nil
}

func (service *Service) validateOfficialReview(official pricing.ModelPrice) error {
	if official.ReviewAfter == "" {
		return nil
	}
	reviewAfter, err := parseDate(official.ReviewAfter)
	if err != nil {
		return fmt.Errorf("invalid official review_after: %w", errors.Join(pricing.ErrPriceUnavailable, pricing.ErrInvalidCatalog))
	}
	now := time.Now()
	if service != nil && service.clock != nil {
		now = service.clock.Now()
	}
	if !now.UTC().Before(reviewAfter) {
		return fmt.Errorf("official price review_after reached: %w", errors.Join(pricing.ErrPriceUnavailable, ErrOfficialPriceExpired))
	}
	return nil
}

func buildReplaceCommand(official pricing.ModelPrice, input SetOverrideInput) (ReplaceOverrideCommand, error) {
	if !validAdministratorID(input.AdministratorID) {
		return ReplaceOverrideCommand{}, fmt.Errorf("%w: administrator is required", ErrInvalidOverride)
	}
	if !validOfficialSource(input.SourceURL, official.CatalogVendor) {
		return ReplaceOverrideCommand{}, fmt.Errorf("%w: source URL is not an approved official host", ErrInvalidOverride)
	}
	verifiedAt, err := parseDate(input.VerifiedAt)
	if err != nil || !validMySQLDate(verifiedAt) {
		return ReplaceOverrideCommand{}, fmt.Errorf("%w: invalid verified date", ErrInvalidOverride)
	}
	if len(input.Prices) != len(official.Rates) {
		return ReplaceOverrideCommand{}, fmt.Errorf("%w: rate set size differs from official catalog", ErrInvalidOverride)
	}

	officialRates := make(map[string]pricing.Rate, len(official.Rates))
	for _, rate := range official.Rates {
		officialRates[rateIdentity(rate.Category, rate.Unit, rate.TierKey)] = rate
	}
	parsed := make(map[string]PriceOverrideRate, len(input.Prices))
	for _, inputRate := range input.Prices {
		key := rateIdentity(inputRate.Category, inputRate.Unit, inputRate.TierKey)
		officialRate, exists := officialRates[key]
		if !exists {
			return ReplaceOverrideCommand{}, fmt.Errorf("%w: unknown rate key", ErrInvalidOverride)
		}
		if _, duplicate := parsed[key]; duplicate {
			return ReplaceOverrideCommand{}, fmt.Errorf("%w: duplicate rate key", ErrInvalidOverride)
		}
		if inputRate.UnitScale <= 0 || inputRate.UnitScale != officialRate.UnitScale {
			return ReplaceOverrideCommand{}, fmt.Errorf("%w: rate unit scale differs from official catalog", ErrInvalidOverride)
		}
		priceUnits, parseErr := money.ParseRMBUnits(inputRate.Price)
		if parseErr != nil {
			return ReplaceOverrideCommand{}, fmt.Errorf("%w: invalid decimal price", ErrInvalidOverride)
		}
		parsed[key] = PriceOverrideRate{
			Category: officialRate.Category, Unit: officialRate.Unit, TierKey: officialRate.TierKey,
			PriceUnits: priceUnits, UnitScale: officialRate.UnitScale,
		}
	}
	rates := make([]PriceOverrideRate, 0, len(official.Rates))
	for _, officialRate := range official.Rates {
		rate, exists := parsed[rateIdentity(officialRate.Category, officialRate.Unit, officialRate.TierKey)]
		if !exists {
			return ReplaceOverrideCommand{}, fmt.Errorf("%w: missing official rate key", ErrInvalidOverride)
		}
		rates = append(rates, rate)
	}
	return ReplaceOverrideCommand{
		CatalogVendor: official.CatalogVendor, ModelID: official.ModelID, ExpectedVersion: uint64(input.ExpectedVersion),
		SourceURL: input.SourceURL, VerifiedAt: verifiedAt, UpdatedBy: input.AdministratorID, Rates: rates,
	}, nil
}

func mutationSummary(official pricing.ModelPrice, before *PriceOverride, after *PriceOverride) (*MutationSummary, error) {
	if after == nil {
		return nil, fmt.Errorf("%w: repository returned no saved override", ErrInvalidOverride)
	}
	beforeSummary := PriceSummary{ModelPrice: officialPrice(official)}
	if before != nil {
		price, err := priceFromOverride(official, before)
		if err != nil {
			return nil, err
		}
		beforeSummary = PriceSummary{ModelPrice: price}
	}
	afterPrice, err := priceFromOverride(official, after)
	if err != nil {
		return nil, err
	}
	return &MutationSummary{
		Before: beforeSummary,
		After:  PriceSummary{ModelPrice: afterPrice},
	}, nil
}

func validateStoredOverride(official pricing.ModelPrice) ExistingOverrideValidator {
	return func(existing *PriceOverride) error {
		_, err := priceFromOverride(official, existing)
		return err
	}
}

func priceFromOverride(official pricing.ModelPrice, override *PriceOverride) (pricing.ModelPrice, error) {
	if override == nil {
		return pricing.ModelPrice{}, fmt.Errorf("%w: missing override", ErrInvalidOverride)
	}
	if override.ID == 0 || override.CatalogVendor != official.CatalogVendor || override.ModelID != official.ModelID || override.Version == 0 || override.UpdatedBy == 0 {
		return pricing.ModelPrice{}, fmt.Errorf("%w: invalid override header", ErrInvalidOverride)
	}
	if !validOfficialSource(override.SourceURL, official.CatalogVendor) || !validMySQLDate(override.VerifiedAt) {
		return pricing.ModelPrice{}, fmt.Errorf("%w: invalid override audit metadata", ErrInvalidOverride)
	}
	if len(override.Rates) != len(official.Rates) {
		return pricing.ModelPrice{}, fmt.Errorf("%w: incomplete override rate set", ErrInvalidOverride)
	}

	officialRates := make(map[string]pricing.Rate, len(official.Rates))
	for _, rate := range official.Rates {
		officialRates[rateIdentity(rate.Category, rate.Unit, rate.TierKey)] = rate
	}
	overrideRates := make(map[string]PriceOverrideRate, len(override.Rates))
	for _, rate := range override.Rates {
		if rate.OverrideID != override.ID || rate.PriceUnits < 0 || rate.UnitScale <= 0 {
			return pricing.ModelPrice{}, fmt.Errorf("%w: invalid override rate", ErrInvalidOverride)
		}
		key := rateIdentity(rate.Category, rate.Unit, rate.TierKey)
		officialRate, exists := officialRates[key]
		if !exists || officialRate.UnitScale != rate.UnitScale {
			return pricing.ModelPrice{}, fmt.Errorf("%w: override rate shape differs from official catalog", ErrInvalidOverride)
		}
		if _, duplicate := overrideRates[key]; duplicate {
			return pricing.ModelPrice{}, fmt.Errorf("%w: duplicate override rate", ErrInvalidOverride)
		}
		overrideRates[key] = rate
	}

	effectiveRates := make([]pricing.Rate, 0, len(official.Rates))
	for _, officialRate := range official.Rates {
		overrideRate, exists := overrideRates[rateIdentity(officialRate.Category, officialRate.Unit, officialRate.TierKey)]
		if !exists {
			return pricing.ModelPrice{}, fmt.Errorf("%w: missing override rate", ErrInvalidOverride)
		}
		effectiveRates = append(effectiveRates, pricing.Rate{
			Category: officialRate.Category, Unit: officialRate.Unit, TierKey: officialRate.TierKey,
			PriceUnits: overrideRate.PriceUnits, UnitScale: officialRate.UnitScale,
		})
	}
	effective := officialPrice(official)
	effective.PriceSource = "override"
	effective.OverrideVersion = override.Version
	effective.SourceURL = override.SourceURL
	effective.RetrievedAt = override.VerifiedAt.Format("2006-01-02")
	effective.Rates = effectiveRates
	effective.Version = effective.CatalogVersion + ":override:" + strconv.FormatUint(override.Version, 10)
	return effective, nil
}

func officialPrice(model pricing.ModelPrice) pricing.ModelPrice {
	model.Aliases = append([]string(nil), model.Aliases...)
	model.Rates = append([]pricing.Rate(nil), model.Rates...)
	if model.CatalogVersion == "" {
		model.CatalogVersion = model.Version
	}
	model.Version = model.CatalogVersion
	model.OverrideVersion = 0
	model.PriceSource = "official"
	return model
}

func isReviewedManagedModel(model pricing.ModelPrice) bool {
	if model.PricingProfile != "standard_global" {
		return false
	}
	return (model.CatalogVendor == "openai" && model.ModelFamily == "gpt") ||
		(model.CatalogVendor == "anthropic" && model.ModelFamily == "claude")
}

func rateIdentity(category pricing.Category, unit string, tierKey string) string {
	return string(category) + "\x00" + unit + "\x00" + tierKey
}

func parseDate(value string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Format("2006-01-02") != value {
		return time.Time{}, errors.New("invalid UTC date")
	}
	return parsed, nil
}

func validOfficialSource(value string, catalogVendor string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxSourceURLCharacters {
		return false
	}
	source, err := url.Parse(value)
	if err != nil || source.Scheme != "https" || source.Hostname() == "" || source.User != nil || source.Port() != "" {
		return false
	}
	host := strings.ToLower(source.Hostname())
	var allowedHosts []string
	switch catalogVendor {
	case "openai":
		allowedHosts = []string{"openai.com"}
	case "anthropic":
		allowedHosts = []string{"anthropic.com", "claude.com"}
	default:
		return false
	}
	for _, allowed := range allowedHosts {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

func validMySQLDate(value time.Time) bool {
	return !value.IsZero() && value.Year() >= minMySQLDateYear && value.Year() <= maxMySQLDateYear
}

func validAdministratorID(value uint64) bool {
	return value > 0 && value <= maxDatabaseAdministratorID
}

func mutationRepositoryError(err error) *apperror.Error {
	if errors.Is(err, ErrVersionConflict) {
		return versionConflictError(err)
	}
	if errors.Is(err, ErrInvalidOverride) {
		return invalidOverrideError(err)
	}
	return internalPricingError(err)
}

func invalidOverrideError(cause error) *apperror.Error {
	return apperror.Wrap(ErrorCodeInvalidOverride, apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent,
		ErrorCodeInvalidOverride, nil, "模型价格覆盖无效", cause)
}

func versionConflictError(cause error) *apperror.Error {
	return apperror.Wrap(ErrorCodeVersionConflict, apperror.CategoryConflict, http.StatusConflict, apperror.Permanent,
		ErrorCodeVersionConflict, nil, "模型价格版本已变化，请刷新后重试", cause)
}

func modelNotFoundError(cause error) *apperror.Error {
	return apperror.Wrap(ErrorCodeModelNotFound, apperror.CategoryNotFound, http.StatusNotFound, apperror.Permanent,
		ErrorCodeModelNotFound, nil, "模型不在受审定价目录中", cause)
}

func internalPricingError(cause error) *apperror.Error {
	return apperror.Wrap("internal.unknown", apperror.CategoryInternal, http.StatusInternalServerError, apperror.Permanent,
		"", nil, "模型定价服务异常", cause)
}

var _ Resolver = (*Service)(nil)
var _ Repository = (*GormRepository)(nil)
