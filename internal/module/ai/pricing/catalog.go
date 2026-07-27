package pricing

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strings"
	"time"

	"admin_back_go/internal/shared/money"
)

const MaxSafeOutputTokens int64 = math.MaxInt32

type Category string

const (
	InputTokens  Category = "input"
	OutputTokens Category = "output"
	CacheRead    Category = "cache_read"
	CacheWrite   Category = "cache_write"
	MediaUnits   Category = "media"
)

type Rate struct {
	Category   Category `json:"category"`
	Unit       string   `json:"unit"`
	TierKey    string   `json:"tier_key"`
	PriceUnits int64    `json:"price_units"`
	UnitScale  int64    `json:"unit_scale"`
}

type ModelPrice struct {
	Version                    string   `json:"version"`
	CatalogVersion             string   `json:"catalog_version,omitempty"`
	OverrideVersion            uint64   `json:"override_version,omitempty"`
	CatalogVendor              string   `json:"catalog_vendor"`
	ModelFamily                string   `json:"model_family,omitempty"`
	ModelID                    string   `json:"model_id"`
	Aliases                    []string `json:"aliases"`
	PricingProfile             string   `json:"pricing_profile,omitempty"`
	MaxOutputTokens            int64    `json:"max_output_tokens"`
	ContextTierThresholdTokens int64    `json:"context_tier_threshold_tokens,omitempty"`
	PriceSource                string   `json:"price_source,omitempty"`
	SourceURL                  string   `json:"source_url"`
	RetrievedAt                string   `json:"retrieved_at"`
	ReviewAfter                string   `json:"review_after,omitempty"`
	Rates                      []Rate   `json:"rates"`
}

type catalogDocument struct {
	Version          string               `json:"version"`
	OfficialCurrency string               `json:"official_currency"`
	BillingCurrency  string               `json:"billing_currency"`
	ConversionPolicy string               `json:"conversion_policy"`
	Models           []officialModelPrice `json:"models"`
}

type officialModelPrice struct {
	CatalogVendor              string         `json:"catalog_vendor"`
	ModelFamily                string         `json:"model_family,omitempty"`
	ModelID                    string         `json:"model_id"`
	Aliases                    []string       `json:"aliases"`
	PricingProfile             string         `json:"pricing_profile,omitempty"`
	MaxOutputTokens            int64          `json:"max_output_tokens"`
	ContextTierThresholdTokens int64          `json:"context_tier_threshold_tokens,omitempty"`
	SourceURL                  string         `json:"source_url"`
	RetrievedAt                string         `json:"retrieved_at"`
	ReviewAfter                string         `json:"review_after,omitempty"`
	Rates                      []officialRate `json:"rates"`
}

type officialRate struct {
	Category  Category `json:"category"`
	Unit      string   `json:"unit"`
	TierKey   string   `json:"tier_key"`
	Price     string   `json:"price"`
	UnitScale int64    `json:"unit_scale"`
}

var (
	ErrPriceUnavailable      = errors.New("price unavailable")
	ErrMissingModel          = errors.New("model identity is required")
	ErrAmbiguousModel        = errors.New("model alias is ambiguous")
	ErrInvalidCatalog        = errors.New("invalid pricing catalog")
	ErrUnsupportedUsage      = errors.New("unsupported usage")
	ErrInvalidMultiplier     = errors.New("billing multiplier must be positive")
	ErrDuplicateLine         = errors.New("duplicate quote line")
	ErrQuoteOverflow         = errors.New("quote exceeds int64")
	ErrUnsafeTokenUpperBound = errors.New("unsafe token upper bound")
)

// Catalog is an immutable, validated collection of official prices.
type Catalog struct {
	version string
	models  []ModelPrice
	byID    map[string]ModelPrice
	aliases map[string][]ModelPrice
}

func NewCatalog(models []ModelPrice) *Catalog {
	catalog, _ := NewCatalogChecked(models)
	return catalog
}

func NewCatalogChecked(models []ModelPrice) (*Catalog, error) {
	c := &Catalog{models: make([]ModelPrice, len(models)), byID: make(map[string]ModelPrice), aliases: make(map[string][]ModelPrice)}
	for i := range models {
		c.models[i] = cloneModel(models[i])
	}
	for i := range c.models {
		model := c.models[i]
		model.ModelID = strings.TrimSpace(model.ModelID)
		model.CatalogVendor = strings.TrimSpace(model.CatalogVendor)
		if strings.TrimSpace(model.ModelID) == "" || strings.TrimSpace(model.CatalogVendor) == "" {
			return nil, fmt.Errorf("%w: missing model identity", ErrInvalidCatalog)
		}
		if strings.TrimSpace(model.Version) == "" || strings.TrimSpace(model.SourceURL) == "" || strings.TrimSpace(model.RetrievedAt) == "" {
			return nil, fmt.Errorf("%w: missing audit metadata", ErrInvalidCatalog)
		}
		source, err := url.Parse(model.SourceURL)
		if err != nil || (source.Scheme != "http" && source.Scheme != "https") || source.Host == "" {
			return nil, fmt.Errorf("%w: invalid source url", ErrInvalidCatalog)
		}
		if model.MaxOutputTokens <= 0 || model.MaxOutputTokens > MaxSafeOutputTokens {
			return nil, ErrUnsafeTokenUpperBound
		}
		if _, exists := c.byID[model.ModelID]; exists {
			return nil, fmt.Errorf("%w: duplicate canonical model", ErrInvalidCatalog)
		}
		if c.version == "" {
			c.version = model.Version
		}
		seenRates := map[string]struct{}{}
		for i := range model.Rates {
			rate := &model.Rates[i]
			rate.Unit = strings.TrimSpace(rate.Unit)
			rate.TierKey = strings.TrimSpace(rate.TierKey)
			if rate.Unit == "" || rate.UnitScale <= 0 || rate.PriceUnits < 0 || !validCategory(rate.Category) {
				return nil, fmt.Errorf("%w: invalid rate", ErrInvalidCatalog)
			}
			key := rateKey(rate.Category, rate.Unit, rate.TierKey)
			if _, exists := seenRates[key]; exists {
				return nil, fmt.Errorf("%w: duplicate rate", ErrInvalidCatalog)
			}
			seenRates[key] = struct{}{}
		}
		c.models[i] = model
		c.byID[model.ModelID] = model
	}
	for _, model := range c.models {
		seenAliases := map[string]struct{}{}
		for _, alias := range model.Aliases {
			alias = strings.TrimSpace(alias)
			if alias == "" || alias == model.ModelID {
				continue
			}
			if _, canonical := c.byID[alias]; canonical {
				return nil, fmt.Errorf("%w: alias conflicts with canonical model", ErrInvalidCatalog)
			}
			if _, duplicate := seenAliases[alias]; duplicate {
				return nil, fmt.Errorf("%w: duplicate alias", ErrInvalidCatalog)
			}
			seenAliases[alias] = struct{}{}
			c.aliases[alias] = append(c.aliases[alias], model)
		}
	}
	return c, nil
}

func (c *Catalog) Resolve(identity string) (ModelPrice, error) {
	if c == nil {
		return ModelPrice{}, ErrPriceUnavailable
	}
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return ModelPrice{}, ErrMissingModel
	}
	if model, ok := c.byID[identity]; ok {
		return cloneModel(model), nil
	}
	models := c.aliases[identity]
	if len(models) == 0 {
		return ModelPrice{}, ErrPriceUnavailable
	}
	if len(models) > 1 {
		return ModelPrice{}, ErrAmbiguousModel
	}
	return cloneModel(models[0]), nil
}

func (c *Catalog) Version() string {
	if c == nil {
		return ""
	}
	return c.version
}

func ResolveModel(identity string) (ModelPrice, error) { return Default.Resolve(identity) }
func CatalogVersion() string                           { return Default.Version() }
func (c *Catalog) Models() []ModelPrice {
	if c == nil {
		return nil
	}
	out := make([]ModelPrice, len(c.models))
	for i := range c.models {
		out[i] = cloneModel(c.models[i])
	}
	return out
}

func cloneModel(model ModelPrice) ModelPrice {
	model.Aliases = append([]string(nil), model.Aliases...)
	model.Rates = append([]Rate(nil), model.Rates...)
	return model
}

func validCategory(category Category) bool {
	switch category {
	case InputTokens, OutputTokens, CacheRead, CacheWrite, MediaUnits:
		return true
	default:
		return false
	}
}
func rateKey(category Category, unit, tier string) string {
	return string(category) + "\x00" + unit + "\x00" + tier
}

func loadOfficialCatalog(data []byte) (*Catalog, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document catalogDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("%w: decode official catalog: %v", ErrInvalidCatalog, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing catalog data", ErrInvalidCatalog)
	}
	if document.Version != "official_numeric_parity_v3" || document.OfficialCurrency != "USD" || document.BillingCurrency != "CNY" || document.ConversionPolicy != "numeric_parity" {
		return nil, fmt.Errorf("%w: invalid catalog policy", ErrInvalidCatalog)
	}
	models := make([]ModelPrice, len(document.Models))
	identities := make(map[string]struct{})
	for i, raw := range document.Models {
		modelID := strings.TrimSpace(raw.ModelID)
		if _, exists := identities[modelID]; modelID == "" || modelID != raw.ModelID || exists {
			return nil, fmt.Errorf("%w: duplicate canonical model", ErrInvalidCatalog)
		}
		if strings.TrimSpace(raw.ModelFamily) == "" || strings.TrimSpace(raw.ModelFamily) != raw.ModelFamily {
			return nil, fmt.Errorf("%w: missing model family", ErrInvalidCatalog)
		}
		identities[modelID] = struct{}{}
		if !validOfficialSource(raw.SourceURL) || !validUTCDate(raw.RetrievedAt) || (raw.ReviewAfter != "" && !validUTCDate(raw.ReviewAfter)) {
			return nil, fmt.Errorf("%w: invalid official source metadata", ErrInvalidCatalog)
		}
		if (raw.ModelFamily == "gpt" || raw.ModelFamily == "claude") && raw.PricingProfile != "standard_global" {
			return nil, fmt.Errorf("%w: invalid managed pricing profile", ErrInvalidCatalog)
		}
		model := ModelPrice{
			Version: document.Version, CatalogVersion: document.Version, CatalogVendor: raw.CatalogVendor,
			ModelFamily: raw.ModelFamily, ModelID: raw.ModelID, Aliases: raw.Aliases, PricingProfile: raw.PricingProfile,
			MaxOutputTokens: raw.MaxOutputTokens, ContextTierThresholdTokens: raw.ContextTierThresholdTokens,
			PriceSource: "official", SourceURL: raw.SourceURL, RetrievedAt: raw.RetrievedAt, ReviewAfter: raw.ReviewAfter,
			Rates: make([]Rate, len(raw.Rates)),
		}
		positive := false
		for j, rawRate := range raw.Rates {
			if (raw.ModelFamily == "gpt" || raw.ModelFamily == "claude") && rawRate.Unit != "token" {
				return nil, fmt.Errorf("%w: managed model rate must use token", ErrInvalidCatalog)
			}
			units, err := money.ParseRMBUnits(rawRate.Price)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid decimal price", ErrInvalidCatalog)
			}
			positive = positive || units > 0
			model.Rates[j] = Rate{Category: rawRate.Category, Unit: rawRate.Unit, TierKey: rawRate.TierKey, PriceUnits: units, UnitScale: rawRate.UnitScale}
		}
		if !positive {
			return nil, fmt.Errorf("%w: model has no positive rate", ErrInvalidCatalog)
		}
		models[i] = model
	}
	for _, raw := range document.Models {
		for _, alias := range raw.Aliases {
			normalized := strings.TrimSpace(alias)
			if normalized == "" || normalized != alias {
				return nil, fmt.Errorf("%w: empty alias", ErrInvalidCatalog)
			}
			if _, exists := identities[normalized]; exists {
				return nil, fmt.Errorf("%w: duplicate model identity", ErrInvalidCatalog)
			}
			identities[normalized] = struct{}{}
		}
	}
	catalog, err := NewCatalogChecked(models)
	if err != nil {
		return nil, err
	}
	catalog.version = document.Version
	return catalog, nil
}

func validUTCDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Location() == time.UTC && parsed.Format("2006-01-02") == value
}

func validOfficialSource(value string) bool {
	source, err := url.Parse(value)
	if err != nil || source.Scheme != "https" || source.Hostname() == "" || source.User != nil {
		return false
	}
	host := strings.ToLower(source.Hostname())
	for _, allowed := range []string{"openai.com", "anthropic.com", "claude.com"} {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

//go:embed catalog/official_numeric_parity_v3.json
var catalogJSON []byte

var Default *Catalog
var DefaultCatalog *Catalog

func init() {
	var err error
	Default, err = loadOfficialCatalog(catalogJSON)
	if err != nil {
		panic(err)
	}
	DefaultCatalog = Default
}
