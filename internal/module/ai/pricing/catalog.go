package pricing

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
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
	Version         string   `json:"version"`
	CatalogVendor   string   `json:"catalog_vendor"`
	ModelID         string   `json:"model_id"`
	Aliases         []string `json:"aliases"`
	MaxOutputTokens int64    `json:"max_output_tokens"`
	SourceURL       string   `json:"source_url"`
	RetrievedAt     string   `json:"retrieved_at"`
	Rates           []Rate   `json:"rates"`
}

type catalogDocument struct {
	Version string       `json:"version"`
	Models  []ModelPrice `json:"models"`
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

//go:embed catalog/official_numeric_parity_v2.json
var catalogJSON []byte

var Default *Catalog
var DefaultCatalog *Catalog

func init() {
	var document catalogDocument
	if err := json.Unmarshal(catalogJSON, &document); err != nil {
		panic(err)
	}
	for i := range document.Models {
		if document.Models[i].Version == "" {
			document.Models[i].Version = document.Version
		}
	}
	var err error
	Default, err = NewCatalogChecked(document.Models)
	if err != nil {
		panic(err)
	}
	Default.version = document.Version
	DefaultCatalog = Default
}
