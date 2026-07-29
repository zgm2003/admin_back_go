package officialmodel

import (
	"context"
	"errors"
	"strconv"
	"time"

	"admin_back_go/internal/module/ai/pricing"
)

const (
	PriceSourceOfficial = "official"
	PriceSourceOverride = "override"
)

var (
	ErrInvalidOverride      = errors.New("invalid official model price override")
	ErrPriceUnavailable     = errors.New("official model price unavailable")
	ErrOfficialPriceExpired = errors.New("official model price requires review")
)

type ResolvedModel struct {
	Model           Model
	EffectivePrice  pricing.PriceBook
	PriceSource     string
	OverrideVersion uint64
	PriceSourceURL  string
	PriceVerifiedAt time.Time
}

type Resolver interface {
	Resolve(context.Context, string) (ResolvedModel, error)
}

type ResolverFunc func(context.Context, string) (ResolvedModel, error)

func (resolve ResolverFunc) Resolve(ctx context.Context, modelID string) (ResolvedModel, error) {
	if resolve == nil {
		return ResolvedModel{}, ErrRepositoryNotConfigured
	}
	return resolve(ctx, modelID)
}

func (model ResolvedModel) PricingVersion() string {
	version := model.Model.CatalogVersion
	if model.PriceSource == PriceSourceOverride && model.OverrideVersion > 0 {
		return version + ":override:" + strconv.FormatUint(model.OverrideVersion, 10)
	}
	return version
}
