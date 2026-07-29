package officialmodel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/money"
)

const (
	ErrorCodeInvalidPriceSync = "ai.official_model.invalid_price_sync"
	ErrorCodeVersionConflict  = "ai.official_model.version_conflict"
	ErrorCodeModelNotFound    = "ai.official_model.not_found"

	maxSourceURLCharacters     = 2048
	maxDatabaseAdministratorID = uint64(1<<32 - 1)
)

var ErrOfficialModelNotFound = errors.New("official model not found")

func (service *Service) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, officialModelInternalError(err)
		}
	}
	models := service.catalogModels()
	return &PageInitResponse{Dict: PageInitDict{
		VendorOptions:        distinctOptions(models, func(model Model) string { return model.CatalogVendor }),
		FamilyOptions:        distinctOptions(models, func(model Model) string { return model.ModelFamily }),
		LifecycleOptions:     []OptionDTO{{Label: "启用", Value: string(LifecycleActive)}, {Label: "已弃用", Value: string(LifecycleDeprecated)}, {Label: "已退役", Value: string(LifecycleRetired)}},
		InputModalityOptions: []OptionDTO{{Label: "文本", Value: ModalityText}, {Label: "图片", Value: ModalityImage}, {Label: "音频", Value: ModalityAudio}, {Label: "原生文件", Value: ModalityFile}},
	}}, nil
}

func (service *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	if service == nil || service.catalog == nil || service.repository == nil {
		return nil, officialModelInternalError(ErrRepositoryNotConfigured)
	}
	query.Vendor = strings.TrimSpace(query.Vendor)
	query.Family = strings.TrimSpace(query.Family)
	query.InputModality = strings.TrimSpace(query.InputModality)
	needle := strings.ToLower(strings.TrimSpace(query.ModelID))
	if query.Lifecycle != "" && query.Lifecycle != LifecycleActive && query.Lifecycle != LifecycleDeprecated && query.Lifecycle != LifecycleRetired {
		return nil, invalidPriceSyncError(errors.New("invalid lifecycle filter"))
	}
	if query.InputModality != "" && !knownModality(query.InputModality) {
		return nil, invalidPriceSyncError(errors.New("invalid input modality filter"))
	}
	items := make([]OfficialModelDTO, 0)
	for _, model := range service.catalogModels() {
		if query.Vendor != "" && model.CatalogVendor != query.Vendor || query.Family != "" && model.ModelFamily != query.Family ||
			query.Lifecycle != "" && model.LifecycleStatus != query.Lifecycle || query.InputModality != "" && !containsString(model.Capabilities.InputModalities, query.InputModality) ||
			needle != "" && !strings.Contains(strings.ToLower(model.ModelID), needle) {
			continue
		}
		item, err := service.managementModel(ctx, model)
		if err != nil {
			return nil, officialModelInternalError(err)
		}
		items = append(items, item)
	}
	return &ListResponse{List: items}, nil
}

func (service *Service) Detail(ctx context.Context, modelID string) (*OfficialModelDTO, *apperror.Error) {
	model, appErr := service.canonicalModel(modelID)
	if appErr != nil {
		return nil, appErr
	}
	item, err := service.managementModel(ctx, model)
	if err != nil {
		return nil, officialModelInternalError(err)
	}
	return &item, nil
}

func (service *Service) SetPriceOverride(ctx context.Context, modelID string, input SetPriceOverrideInput) (*MutationSummary, *apperror.Error) {
	model, appErr := service.canonicalModel(modelID)
	if appErr != nil {
		return nil, appErr
	}
	if model.LifecycleStatus == LifecycleRetired {
		return nil, invalidPriceSyncError(errors.New("retired official model is read-only"))
	}
	command, err := service.replaceOverrideCommand(model, input)
	if err != nil {
		return nil, invalidPriceSyncError(err)
	}
	before, after, err := service.repository.ReplaceOverride(ctx, command, validateStoredOverride(model))
	if err != nil {
		return nil, mutationRepositoryError(err)
	}
	return mutationSummary(model, before, after)
}

func (service *Service) RestoreOfficialPrice(ctx context.Context, modelID string, expectedVersion int64, administratorID uint64) (*MutationSummary, *apperror.Error) {
	model, appErr := service.canonicalModel(modelID)
	if appErr != nil {
		return nil, appErr
	}
	if expectedVersion <= 0 || uint64(expectedVersion) > uint64(^uint64(0)) || !validAdministratorID(administratorID) {
		return nil, invalidPriceSyncError(errors.New("invalid restore input"))
	}
	before, err := service.repository.DeleteOverride(ctx, DeleteOverrideCommand{
		CatalogVendor: model.CatalogVendor, ModelID: model.ModelID, ExpectedVersion: uint64(expectedVersion),
	}, validateStoredOverride(model))
	if err != nil {
		return nil, mutationRepositoryError(err)
	}
	return mutationSummary(model, before, nil)
}

func MutationResponseFromSummary(summary *MutationSummary) (*MutationResponse, error) {
	if summary == nil {
		return nil, errors.New("missing official model mutation summary")
	}
	before, err := priceDTO(summary.Before, true)
	if err != nil {
		return nil, err
	}
	after, err := priceDTO(summary.After, true)
	if err != nil {
		return nil, err
	}
	return &MutationResponse{Before: before, After: after}, nil
}

func (service *Service) catalogModels() []Model {
	if service == nil || service.catalog == nil {
		return nil
	}
	models := service.catalog.Models()
	sort.Slice(models, func(i, j int) bool {
		if models[i].CatalogVendor != models[j].CatalogVendor {
			return models[i].CatalogVendor < models[j].CatalogVendor
		}
		if models[i].ModelFamily != models[j].ModelFamily {
			return models[i].ModelFamily < models[j].ModelFamily
		}
		return models[i].ModelID < models[j].ModelID
	})
	return models
}

func (service *Service) canonicalModel(modelID string) (Model, *apperror.Error) {
	if service == nil || service.catalog == nil || service.repository == nil {
		return Model{}, officialModelInternalError(ErrRepositoryNotConfigured)
	}
	requested := strings.TrimSpace(modelID)
	model, err := service.catalog.ResolveIdentity(requested)
	if err != nil || requested != model.ModelID {
		return Model{}, officialModelNotFoundError(errors.Join(ErrOfficialModelNotFound, err))
	}
	return model, nil
}

func (service *Service) managementModel(ctx context.Context, model Model) (OfficialModelDTO, error) {
	official := officialResolvedModel(model)
	effective := official
	override, err := service.repository.FindOverride(ctx, model.CatalogVendor, model.ModelID)
	if err != nil {
		return OfficialModelDTO{}, err
	}
	if override != nil {
		effective, err = resolvedFromOverride(model, override)
		if err != nil {
			return OfficialModelDTO{}, err
		}
	}
	officialDTO, err := priceDTO(official, service.validateOfficialReview(model) == nil)
	if err != nil {
		return OfficialModelDTO{}, err
	}
	effectiveDTO, err := priceDTO(effective, override != nil || service.validateOfficialReview(model) == nil)
	if err != nil {
		return OfficialModelDTO{}, err
	}
	capabilities := model.Capabilities
	return OfficialModelDTO{
		CatalogVendor: model.CatalogVendor, ModelFamily: model.ModelFamily, ModelID: model.ModelID,
		Aliases: cloneStrings(model.Aliases), LifecycleStatus: model.LifecycleStatus, CatalogVersion: model.CatalogVersion,
		ContextWindowTokens: model.ContextWindowTokens, MaxOutputTokens: model.MaxOutputTokens,
		ContextTierThresholdTokens: model.ContextTierThresholdTokens,
		Capabilities: CapabilityDTO{
			InputModalities: cloneStrings(capabilities.InputModalities), OutputModalities: cloneStrings(capabilities.OutputModalities),
			SupportsStreaming: capabilities.SupportsStreaming, SupportsTools: capabilities.SupportsTools,
			SupportsStructuredOutput: capabilities.SupportsStructuredOutput, SupportedParameters: cloneStrings(capabilities.SupportedParameters),
			NativeFileInput: capabilities.NativeFileInput, ImageInput: cloneCapabilities(capabilities).ImageInput,
		},
		PricingProfile: model.PricingProfile, Official: officialDTO, Effective: effectiveDTO,
		ModelSourceURL: model.ModelSourceURL, PricingSourceURL: model.PricingSourceURL,
		RetrievedAt: model.RetrievedAt, ReviewAfter: model.ReviewAfter,
	}, nil
}

func officialResolvedModel(model Model) ResolvedModel {
	verifiedAt, _ := time.Parse(time.DateOnly, model.RetrievedAt)
	return ResolvedModel{
		Model: cloneModel(model), EffectivePrice: clonePriceBook(model.OfficialPrice), PriceSource: PriceSourceOfficial,
		PriceSourceURL: model.PricingSourceURL, PriceVerifiedAt: verifiedAt,
	}
}

func priceDTO(model ResolvedModel, available bool) (PriceDTO, error) {
	rates := make([]RateDTO, len(model.EffectivePrice.Rates))
	for index, rate := range model.EffectivePrice.Rates {
		price, err := money.FormatRMBUnits(rate.PriceUnits)
		if err != nil {
			return PriceDTO{}, err
		}
		rates[index] = RateDTO{Category: rate.Category, Unit: rate.Unit, TierKey: rate.TierKey, Price: price, UnitScale: rate.UnitScale}
	}
	verifiedAt := ""
	if !model.PriceVerifiedAt.IsZero() {
		verifiedAt = model.PriceVerifiedAt.UTC().Format(time.DateOnly)
	}
	return PriceDTO{
		PricingVersion: model.PricingVersion(), Source: model.PriceSource, OverrideVersion: model.OverrideVersion,
		SourceURL: model.PriceSourceURL, VerifiedAt: verifiedAt, Available: available, Rates: rates,
	}, nil
}

func (service *Service) replaceOverrideCommand(model Model, input SetPriceOverrideInput) (ReplaceOverrideCommand, error) {
	if input.ExpectedVersion < 0 || !validAdministratorID(input.AdministratorID) || len(input.Prices) != len(model.OfficialPrice.Rates) ||
		utf8.RuneCountInString(input.SourceURL) > maxSourceURLCharacters || !validOfficialSource(model.CatalogVendor, strings.TrimSpace(input.SourceURL)) {
		return ReplaceOverrideCommand{}, ErrInvalidOverride
	}
	verifiedAt, err := time.Parse(time.DateOnly, strings.TrimSpace(input.VerifiedAt))
	if err != nil || verifiedAt.Year() < 1000 || verifiedAt.Year() > 9999 || verifiedAt.After(service.clock.Now().UTC()) {
		return ReplaceOverrideCommand{}, ErrInvalidOverride
	}
	officialRates := make(map[string]pricing.Rate, len(model.OfficialPrice.Rates))
	for _, rate := range model.OfficialPrice.Rates {
		officialRates[rateIdentity(rate.Category, rate.Unit, rate.TierKey)] = rate
	}
	rates := make([]PriceOverrideRate, 0, len(input.Prices))
	seen := make(map[string]struct{}, len(input.Prices))
	for _, value := range input.Prices {
		value.Unit, value.TierKey = strings.TrimSpace(value.Unit), strings.TrimSpace(value.TierKey)
		key := rateIdentity(value.Category, value.Unit, value.TierKey)
		officialRate, exists := officialRates[key]
		if !exists || value.UnitScale != officialRate.UnitScale {
			return ReplaceOverrideCommand{}, ErrInvalidOverride
		}
		if _, duplicate := seen[key]; duplicate {
			return ReplaceOverrideCommand{}, ErrInvalidOverride
		}
		seen[key] = struct{}{}
		priceUnits, err := money.ParseRMBUnits(strings.TrimSpace(value.Price))
		if err != nil || priceUnits < 0 {
			return ReplaceOverrideCommand{}, ErrInvalidOverride
		}
		rates = append(rates, PriceOverrideRate{Category: value.Category, Unit: value.Unit, TierKey: value.TierKey, PriceUnits: priceUnits, UnitScale: value.UnitScale})
	}
	return ReplaceOverrideCommand{
		CatalogVendor: model.CatalogVendor, ModelID: model.ModelID, ExpectedVersion: uint64(input.ExpectedVersion),
		SourceURL: strings.TrimSpace(input.SourceURL), VerifiedAt: verifiedAt, UpdatedBy: input.AdministratorID, Rates: rates,
	}, nil
}

func validateStoredOverride(model Model) ExistingOverrideValidator {
	return func(override *PriceOverride) error {
		_, err := resolvedFromOverride(model, override)
		return err
	}
}

func mutationSummary(model Model, before *PriceOverride, after *PriceOverride) (*MutationSummary, *apperror.Error) {
	beforeResolved := officialResolvedModel(model)
	if before != nil {
		var err error
		beforeResolved, err = resolvedFromOverride(model, before)
		if err != nil {
			return nil, officialModelInternalError(err)
		}
	}
	afterResolved := officialResolvedModel(model)
	if after != nil {
		var err error
		afterResolved, err = resolvedFromOverride(model, after)
		if err != nil {
			return nil, officialModelInternalError(err)
		}
	}
	return &MutationSummary{Before: beforeResolved, After: afterResolved}, nil
}

func distinctOptions(models []Model, value func(Model) string) []OptionDTO {
	seen := make(map[string]struct{})
	values := make([]string, 0)
	for _, model := range models {
		item := value(model)
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		values = append(values, item)
	}
	sort.Strings(values)
	options := make([]OptionDTO, len(values))
	for index, value := range values {
		options[index] = OptionDTO{Label: value, Value: value}
	}
	return options
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func knownModality(value string) bool {
	switch value {
	case ModalityText, ModalityImage, ModalityAudio, ModalityFile:
		return true
	default:
		return false
	}
}

func validAdministratorID(value uint64) bool {
	return value > 0 && value <= maxDatabaseAdministratorID
}

func mutationRepositoryError(err error) *apperror.Error {
	if errors.Is(err, ErrVersionConflict) {
		return apperror.Wrap(ErrorCodeVersionConflict, apperror.CategoryConflict, http.StatusConflict, apperror.Permanent, ErrorCodeVersionConflict, nil, "官方模型价格版本冲突", err)
	}
	if errors.Is(err, ErrInvalidOverride) || errors.Is(err, ErrOverrideMappingAmbiguous) {
		return invalidPriceSyncError(err)
	}
	return officialModelInternalError(err)
}

func invalidPriceSyncError(err error) *apperror.Error {
	return apperror.Wrap(ErrorCodeInvalidPriceSync, apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent, ErrorCodeInvalidPriceSync, nil, "官方模型价格同步参数无效", err)
}

func officialModelNotFoundError(err error) *apperror.Error {
	return apperror.Wrap(ErrorCodeModelNotFound, apperror.CategoryNotFound, http.StatusNotFound, apperror.Permanent, ErrorCodeModelNotFound, nil, "官方模型不存在", err)
}

func officialModelInternalError(err error) *apperror.Error {
	return apperror.Wrap("ai.official_model.internal", apperror.CategoryInternal, http.StatusInternalServerError, apperror.Retryable, "", nil, "官方模型服务异常", err)
}

func (service *Service) String() string {
	if service == nil || service.catalog == nil {
		return "officialmodel.Service(<nil>)"
	}
	return fmt.Sprintf("officialmodel.Service(%s)", service.catalog.Version())
}
