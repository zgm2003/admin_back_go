package officialmodel

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/shared/money"
)

type LifecycleStatus string

type ModelKind string

const (
	ModelKindChat      ModelKind = "chat"
	ModelKindEmbedding ModelKind = "embedding"
	ModelKindRerank    ModelKind = "rerank"
	ModelKindImage     ModelKind = "image"
)

func (kind ModelKind) Validate() error {
	switch kind {
	case ModelKindChat, ModelKindEmbedding, ModelKindRerank, ModelKindImage:
		return nil
	default:
		return fmt.Errorf("invalid model kind %q", kind)
	}
}

type EmbeddingSpec struct {
	Dimensions     uint32 `json:"dimensions"`
	MaxInputTokens int64  `json:"max_input_tokens"`
	TokenCounterID string `json:"token_counter_id"`
}

const (
	LifecycleActive     LifecycleStatus = "active"
	LifecycleDeprecated LifecycleStatus = "deprecated"
	LifecycleRetired    LifecycleStatus = "retired"
)

var (
	ErrInvalidCatalog = errors.New("invalid official model catalog")
	ErrModelUnmapped  = errors.New("official model is unmapped")
	ErrModelAmbiguous = errors.New("official model identity is ambiguous")
)

type Model struct {
	CatalogVersion             string
	CatalogVendor              string
	ModelFamily                string
	ModelID                    string
	ModelKind                  ModelKind
	EmbeddingSpec              *EmbeddingSpec
	Aliases                    []string
	LifecycleStatus            LifecycleStatus
	ContextWindowTokens        int64
	MaxOutputTokens            int64
	TokenCounterID             string
	ContextTierThresholdTokens int64
	Capabilities               Capabilities
	PricingProfile             string
	OfficialPrice              pricing.PriceBook
	ModelSourceURL             string
	PricingSourceURL           string
	RetrievedAt                string
	ReviewAfter                string
}

type Catalog struct {
	version     string
	models      []Model
	byCanonical map[string]int
	byAlias     map[string]int
}

func NewCatalog(version string, models []Model) (*Catalog, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return nil, fmt.Errorf("%w: missing version", ErrInvalidCatalog)
	}
	catalog := &Catalog{
		version:     version,
		models:      make([]Model, len(models)),
		byCanonical: make(map[string]int, len(models)),
		byAlias:     make(map[string]int),
	}
	for index, source := range models {
		model := cloneModel(source)
		if err := validateModel(version, model); err != nil {
			return nil, err
		}
		if _, exists := catalog.byCanonical[model.ModelID]; exists {
			return nil, fmt.Errorf("%w: duplicate canonical model %q", ErrInvalidCatalog, model.ModelID)
		}
		catalog.models[index] = model
		catalog.byCanonical[model.ModelID] = index
	}
	for index, model := range catalog.models {
		seen := make(map[string]struct{}, len(model.Aliases))
		for _, alias := range model.Aliases {
			if alias == "" || strings.TrimSpace(alias) != alias {
				return nil, fmt.Errorf("%w: empty alias for %q", ErrInvalidCatalog, model.ModelID)
			}
			if _, duplicate := seen[alias]; duplicate {
				return nil, fmt.Errorf("%w: duplicate alias %q", ErrInvalidCatalog, alias)
			}
			seen[alias] = struct{}{}
			if _, canonical := catalog.byCanonical[alias]; canonical {
				return nil, fmt.Errorf("%w: alias %q conflicts with canonical model", ErrInvalidCatalog, alias)
			}
			if _, duplicate := catalog.byAlias[alias]; duplicate {
				return nil, fmt.Errorf("%w: duplicate reviewed alias %q", ErrInvalidCatalog, alias)
			}
			catalog.byAlias[alias] = index
		}
	}
	return catalog, nil
}

func (catalog *Catalog) ResolveIdentity(requestedID string) (Model, error) {
	if catalog == nil {
		return Model{}, ErrModelUnmapped
	}
	requestedID = strings.TrimSpace(requestedID)
	if requestedID == "" {
		return Model{}, ErrModelUnmapped
	}
	if index, ok := catalog.byCanonical[requestedID]; ok {
		return cloneModel(catalog.models[index]), nil
	}
	if index, ok := catalog.byAlias[requestedID]; ok {
		return cloneModel(catalog.models[index]), nil
	}
	return Model{}, ErrModelUnmapped
}

func (catalog *Catalog) Models() []Model {
	if catalog == nil {
		return nil
	}
	models := make([]Model, len(catalog.models))
	for index := range catalog.models {
		models[index] = cloneModel(catalog.models[index])
	}
	return models
}

func (catalog *Catalog) Version() string {
	if catalog == nil {
		return ""
	}
	return catalog.version
}

func cloneModel(model Model) Model {
	model.Aliases = cloneStrings(model.Aliases)
	model.Capabilities = cloneCapabilities(model.Capabilities)
	model.OfficialPrice.Rates = append([]pricing.Rate(nil), model.OfficialPrice.Rates...)
	if model.EmbeddingSpec != nil {
		spec := *model.EmbeddingSpec
		model.EmbeddingSpec = &spec
	}
	return model
}

func validateModel(version string, model Model) error {
	if model.CatalogVersion != version || strings.TrimSpace(model.CatalogVendor) != model.CatalogVendor || model.CatalogVendor == "" ||
		strings.TrimSpace(model.ModelFamily) != model.ModelFamily || model.ModelFamily == "" ||
		strings.TrimSpace(model.ModelID) != model.ModelID || model.ModelID == "" {
		return fmt.Errorf("%w: invalid identity", ErrInvalidCatalog)
	}
	switch model.LifecycleStatus {
	case LifecycleActive, LifecycleDeprecated, LifecycleRetired:
	default:
		return fmt.Errorf("%w: invalid lifecycle for %q", ErrInvalidCatalog, model.ModelID)
	}
	if model.ContextWindowTokens <= 0 || model.MaxOutputTokens <= 0 || model.MaxOutputTokens > model.ContextWindowTokens ||
		model.ContextTierThresholdTokens < 0 || model.ContextTierThresholdTokens > model.ContextWindowTokens {
		return fmt.Errorf("%w: invalid limits for %q", ErrInvalidCatalog, model.ModelID)
	}
	if _, err := infraai.ResolveTokenCounter(model.TokenCounterID); err != nil {
		return fmt.Errorf("%w: invalid token counter for %q", ErrInvalidCatalog, model.ModelID)
	}
	if err := validateCapabilities(model.Capabilities); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrInvalidCatalog, model.ModelID, err)
	}
	if err := validateModelKind(model); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrInvalidCatalog, model.ModelID, err)
	}
	if model.OfficialPrice.ModelID != model.ModelID ||
		model.OfficialPrice.ContextTierThresholdTokens != model.ContextTierThresholdTokens ||
		len(model.OfficialPrice.Rates) == 0 {
		return fmt.Errorf("%w: invalid official price for %q", ErrInvalidCatalog, model.ModelID)
	}
	seenRates := make(map[string]struct{}, len(model.OfficialPrice.Rates))
	positiveRate := false
	for _, rate := range model.OfficialPrice.Rates {
		if !validRateCategory(rate.Category) || strings.TrimSpace(rate.Unit) == "" || rate.Unit != strings.TrimSpace(rate.Unit) || rate.TierKey != strings.TrimSpace(rate.TierKey) || rate.UnitScale <= 0 || rate.PriceUnits < 0 {
			return fmt.Errorf("%w: invalid rate for %q", ErrInvalidCatalog, model.ModelID)
		}
		key := string(rate.Category) + "\x00" + rate.Unit + "\x00" + rate.TierKey
		if _, duplicate := seenRates[key]; duplicate {
			return fmt.Errorf("%w: duplicate rate for %q", ErrInvalidCatalog, model.ModelID)
		}
		seenRates[key] = struct{}{}
		positiveRate = positiveRate || rate.PriceUnits > 0
	}
	if !positiveRate || !validOfficialSource(model.CatalogVendor, model.ModelSourceURL) || !validOfficialSource(model.CatalogVendor, model.PricingSourceURL) {
		return fmt.Errorf("%w: invalid sources or prices for %q", ErrInvalidCatalog, model.ModelID)
	}
	retrieved, retrievedErr := time.Parse(time.DateOnly, model.RetrievedAt)
	reviewAfter, reviewErr := time.Parse(time.DateOnly, model.ReviewAfter)
	if retrievedErr != nil || reviewErr != nil || !reviewAfter.After(retrieved) {
		return fmt.Errorf("%w: invalid review dates for %q", ErrInvalidCatalog, model.ModelID)
	}
	return nil
}

func validateModelKind(model Model) error {
	if err := model.ModelKind.Validate(); err != nil {
		return err
	}
	hasInput := func(value string) bool { return containsString(model.Capabilities.InputModalities, value) }
	hasOutput := func(value string) bool { return containsString(model.Capabilities.OutputModalities, value) }
	switch model.ModelKind {
	case ModelKindChat:
		if !hasInput(ModalityText) || !hasOutput(ModalityText) || hasOutput(ModalityImage) {
			return errors.New("chat kind requires text input and text output")
		}
	case ModelKindImage:
		if !hasInput(ModalityText) || !hasOutput(ModalityImage) || hasOutput(ModalityText) || model.Capabilities.SupportsTools {
			return errors.New("image kind requires text input and image-only output")
		}
	case ModelKindEmbedding, ModelKindRerank:
		if !hasInput(ModalityText) || hasOutput(ModalityImage) || model.Capabilities.SupportsStreaming || model.Capabilities.SupportsTools || model.Capabilities.SupportsStructuredOutput {
			return errors.New("vector kind has an invalid execution capability")
		}
	}
	if model.ModelKind != ModelKindEmbedding {
		if model.EmbeddingSpec != nil {
			return errors.New("only embedding kind may define an embedding spec")
		}
		return nil
	}
	if model.EmbeddingSpec == nil || model.EmbeddingSpec.Dimensions == 0 || model.EmbeddingSpec.MaxInputTokens <= 0 {
		return errors.New("embedding kind requires a complete embedding spec")
	}
	if _, err := infraai.ResolveTokenCounter(model.EmbeddingSpec.TokenCounterID); err != nil {
		return errors.New("embedding kind requires a registered token counter")
	}
	return nil
}

func validateCapabilities(value Capabilities) error {
	input, err := validateModalities(value.InputModalities)
	if err != nil || len(input) == 0 {
		return errors.New("invalid input modalities")
	}
	output, err := validateModalities(value.OutputModalities)
	if err != nil || len(output) == 0 {
		return errors.New("invalid output modalities")
	}
	for _, parameter := range value.SupportedParameters {
		if parameter != ParameterTemperature {
			return fmt.Errorf("unsupported parameter %q", parameter)
		}
	}
	if duplicateStrings(value.SupportedParameters) {
		return errors.New("duplicate supported parameter")
	}
	_, hasImage := input[ModalityImage]
	if hasImage != (value.ImageInput != nil) {
		return errors.New("image modality and image input contract differ")
	}
	if value.ImageInput != nil {
		if len(value.ImageInput.MIMETypes) == 0 || value.ImageInput.MaxFiles <= 0 || value.ImageInput.MaxBytes <= 0 || duplicateStrings(value.ImageInput.MIMETypes) {
			return errors.New("invalid image input contract")
		}
		for _, mimeType := range value.ImageInput.MIMETypes {
			if !strings.HasPrefix(mimeType, "image/") || mimeType != strings.TrimSpace(mimeType) {
				return errors.New("invalid image MIME type")
			}
		}
	}
	_, hasFile := input[ModalityFile]
	if hasFile != value.NativeFileInput {
		return errors.New("file modality and native file input differ")
	}
	return nil
}

func validateModalities(values []string) (map[string]struct{}, error) {
	if duplicateStrings(values) {
		return nil, errors.New("duplicate modality")
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		switch value {
		case ModalityText, ModalityImage, ModalityAudio, ModalityFile:
			out[value] = struct{}{}
		default:
			return nil, fmt.Errorf("unsupported modality %q", value)
		}
	}
	return out, nil
}

func duplicateStrings(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" || value != strings.TrimSpace(value) {
			return true
		}
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func validRateCategory(category pricing.Category) bool {
	switch category {
	case pricing.InputTokens, pricing.OutputTokens, pricing.CacheRead, pricing.CacheWrite, pricing.MediaUnits:
		return true
	default:
		return false
	}
}

func validOfficialSource(vendor string, value string) bool {
	source, err := url.Parse(value)
	if err != nil || source.Scheme != "https" || source.Hostname() == "" || source.User != nil {
		return false
	}
	host := strings.ToLower(source.Hostname())
	switch vendor {
	case "openai":
		return host == "openai.com" || strings.HasSuffix(host, ".openai.com")
	case "anthropic":
		return host == "anthropic.com" || strings.HasSuffix(host, ".anthropic.com") || host == "claude.com" || strings.HasSuffix(host, ".claude.com")
	default:
		return false
	}
}

type catalogDocument struct {
	Version          string             `json:"version"`
	OfficialCurrency string             `json:"official_currency"`
	BillingCurrency  string             `json:"billing_currency"`
	ConversionPolicy string             `json:"conversion_policy"`
	Models           []catalogModelJSON `json:"models"`
}

type catalogModelJSON struct {
	CatalogVendor              string                `json:"catalog_vendor"`
	ModelFamily                string                `json:"model_family"`
	ModelID                    string                `json:"model_id"`
	ModelKind                  ModelKind             `json:"model_kind"`
	EmbeddingSpec              *EmbeddingSpec        `json:"embedding_spec,omitempty"`
	Aliases                    []string              `json:"aliases"`
	LifecycleStatus            LifecycleStatus       `json:"lifecycle_status"`
	ContextWindowTokens        int64                 `json:"context_window_tokens"`
	MaxOutputTokens            int64                 `json:"max_output_tokens"`
	TokenCounterID             string                `json:"token_counter_id"`
	ContextTierThresholdTokens int64                 `json:"context_tier_threshold_tokens,omitempty"`
	InputModalities            []string              `json:"input_modalities"`
	OutputModalities           []string              `json:"output_modalities"`
	SupportsStreaming          bool                  `json:"supports_streaming"`
	SupportsTools              bool                  `json:"supports_tools"`
	SupportsStructuredOutput   bool                  `json:"supports_structured_output"`
	SupportedParameters        []string              `json:"supported_parameters"`
	NativeFileInput            bool                  `json:"native_file_input"`
	ImageInput                 *ImageInputCapability `json:"image_input,omitempty"`
	PricingProfile             string                `json:"pricing_profile,omitempty"`
	Rates                      []catalogRateJSON     `json:"rates"`
	ModelSourceURL             string                `json:"model_source_url"`
	PricingSourceURL           string                `json:"pricing_source_url"`
	RetrievedAt                string                `json:"retrieved_at"`
	ReviewAfter                string                `json:"review_after"`
}

type catalogRateJSON struct {
	Category  pricing.Category `json:"category"`
	Unit      string           `json:"unit"`
	TierKey   string           `json:"tier_key"`
	Price     string           `json:"price"`
	UnitScale int64            `json:"unit_scale"`
}

func loadCatalog(data []byte) (*Catalog, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document catalogDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrInvalidCatalog, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing data", ErrInvalidCatalog)
	}
	if document.Version != "official_models_v1" || document.OfficialCurrency != "USD" || document.BillingCurrency != "CNY" || document.ConversionPolicy != "numeric_parity" {
		return nil, fmt.Errorf("%w: invalid document policy", ErrInvalidCatalog)
	}
	models := make([]Model, len(document.Models))
	for index, raw := range document.Models {
		rates := make([]pricing.Rate, len(raw.Rates))
		for rateIndex, rawRate := range raw.Rates {
			priceUnits, err := money.ParseRMBUnits(rawRate.Price)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid decimal price for %q", ErrInvalidCatalog, raw.ModelID)
			}
			rates[rateIndex] = pricing.Rate{
				Category: rawRate.Category, Unit: rawRate.Unit, TierKey: rawRate.TierKey,
				PriceUnits: priceUnits, UnitScale: rawRate.UnitScale,
			}
		}
		models[index] = Model{
			CatalogVersion: document.Version, CatalogVendor: raw.CatalogVendor, ModelFamily: raw.ModelFamily,
			ModelID: raw.ModelID, ModelKind: raw.ModelKind, EmbeddingSpec: raw.EmbeddingSpec,
			Aliases: raw.Aliases, LifecycleStatus: raw.LifecycleStatus,
			ContextWindowTokens: raw.ContextWindowTokens, MaxOutputTokens: raw.MaxOutputTokens,
			TokenCounterID:             raw.TokenCounterID,
			ContextTierThresholdTokens: raw.ContextTierThresholdTokens,
			Capabilities: Capabilities{
				InputModalities: raw.InputModalities, OutputModalities: raw.OutputModalities,
				SupportsStreaming: raw.SupportsStreaming, SupportsTools: raw.SupportsTools,
				SupportsStructuredOutput: raw.SupportsStructuredOutput, SupportedParameters: raw.SupportedParameters,
				NativeFileInput: raw.NativeFileInput, ImageInput: raw.ImageInput,
			},
			PricingProfile: raw.PricingProfile,
			OfficialPrice: pricing.PriceBook{
				ModelID: raw.ModelID, ContextTierThresholdTokens: raw.ContextTierThresholdTokens, Rates: rates,
			},
			ModelSourceURL: raw.ModelSourceURL, PricingSourceURL: raw.PricingSourceURL,
			RetrievedAt: raw.RetrievedAt, ReviewAfter: raw.ReviewAfter,
		}
	}
	return NewCatalog(document.Version, models)
}

//go:embed catalog/official_models_v1.json
var catalogJSON []byte

var Default *Catalog

func init() {
	var err error
	Default, err = loadCatalog(catalogJSON)
	if err != nil {
		panic(err)
	}
}
