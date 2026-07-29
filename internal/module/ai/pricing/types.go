package pricing

import (
	"errors"
	"math"
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

var (
	ErrPriceUnavailable      = errors.New("price unavailable")
	ErrMissingModel          = errors.New("model identity is required")
	ErrInvalidCatalog        = errors.New("invalid price book")
	ErrUnsupportedUsage      = errors.New("unsupported usage")
	ErrInvalidMultiplier     = errors.New("billing multiplier must be positive")
	ErrDuplicateLine         = errors.New("duplicate quote line")
	ErrQuoteOverflow         = errors.New("quote exceeds int64")
	ErrUnsafeTokenUpperBound = errors.New("unsafe token upper bound")
)

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
