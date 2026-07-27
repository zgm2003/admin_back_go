package modelpricing

import (
	"context"
	"errors"
	"time"
)

var (
	ErrRepositoryNotConfigured  = errors.New("model pricing repository not configured")
	ErrVersionConflict          = errors.New("model pricing override version conflict")
	ErrOverrideMappingAmbiguous = errors.New("model pricing override mapping is ambiguous")
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

// ExistingOverrideValidator runs after the repository has locked and versioned
// the current row, but before any mutation is issued.
type ExistingOverrideValidator func(*PriceOverride) error

type Repository interface {
	FindOverride(ctx context.Context, catalogVendor string, modelID string) (*PriceOverride, error)
	ReplaceOverride(ctx context.Context, command ReplaceOverrideCommand, validateExisting ExistingOverrideValidator) (before *PriceOverride, after *PriceOverride, err error)
	DeleteOverride(ctx context.Context, command DeleteOverrideCommand, validateExisting ExistingOverrideValidator) (before *PriceOverride, err error)
}
