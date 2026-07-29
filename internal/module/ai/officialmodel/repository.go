package officialmodel

import (
	"context"
	"errors"
	"time"
)

var (
	ErrRepositoryNotConfigured  = errors.New("official model repository not configured")
	ErrVersionConflict          = errors.New("official model price override version conflict")
	ErrOverrideMappingAmbiguous = errors.New("official model price override mapping is ambiguous")
)

type ReplaceOverrideCommand struct {
	CatalogVendor   string
	ModelID         string
	ExpectedVersion uint64
	SourceURL       string
	VerifiedAt      time.Time
	UpdatedBy       uint64
	Rates           []PriceOverrideRate
}

type DeleteOverrideCommand struct {
	CatalogVendor   string
	ModelID         string
	ExpectedVersion uint64
}

type ExistingOverrideValidator func(*PriceOverride) error

type Repository interface {
	FindOverride(ctx context.Context, catalogVendor string, modelID string) (*PriceOverride, error)
	ReplaceOverride(ctx context.Context, command ReplaceOverrideCommand, validateExisting ExistingOverrideValidator) (before *PriceOverride, after *PriceOverride, err error)
	DeleteOverride(ctx context.Context, command DeleteOverrideCommand, validateExisting ExistingOverrideValidator) (before *PriceOverride, err error)
}
