package officialmodel

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrModelMappingStale = errors.New("official model mapping is stale")
	ErrModelRetired      = errors.New("official model is retired")
)

type MappingStatus string

const (
	MappingStatusMapped   MappingStatus = "mapped"
	MappingStatusUnmapped MappingStatus = "unmapped"
)

type IdentityMapping struct {
	RequestedModelID string
	OfficialModelID  string
	CatalogVersion   string
	ModelKind        ModelKind
	EmbeddingSpec    *EmbeddingSpec
	Status           MappingStatus
	MappedAt         *time.Time
}

type IdentityMatcher interface {
	MatchIdentity(requestedModelID string, mappedAt time.Time) IdentityMapping
}

type CatalogMatcher struct {
	catalog *Catalog
}

func NewIdentityMatcher(catalog *Catalog) *CatalogMatcher {
	return &CatalogMatcher{catalog: catalog}
}

func (matcher *CatalogMatcher) MatchIdentity(requestedModelID string, mappedAt time.Time) IdentityMapping {
	return matchIdentity(matcherCatalog(matcher), requestedModelID, mappedAt)
}

func (service *Service) MatchIdentity(requestedModelID string, mappedAt time.Time) IdentityMapping {
	if service == nil {
		return unmappedIdentity(requestedModelID)
	}
	return matchIdentity(service.catalog, requestedModelID, mappedAt)
}

func matchIdentity(catalog *Catalog, requestedModelID string, mappedAt time.Time) IdentityMapping {
	requestedModelID = strings.TrimSpace(requestedModelID)
	model, err := catalog.ResolveIdentity(requestedModelID)
	if err != nil {
		return unmappedIdentity(requestedModelID)
	}
	if mappedAt.IsZero() {
		mappedAt = time.Now()
	}
	mappedAt = mappedAt.UTC()
	return IdentityMapping{
		RequestedModelID: requestedModelID,
		OfficialModelID:  model.ModelID,
		CatalogVersion:   model.CatalogVersion,
		ModelKind:        model.ModelKind,
		EmbeddingSpec:    cloneEmbeddingSpec(model.EmbeddingSpec),
		Status:           MappingStatusMapped,
		MappedAt:         &mappedAt,
	}
}

func cloneEmbeddingSpec(spec *EmbeddingSpec) *EmbeddingSpec {
	if spec == nil {
		return nil
	}
	cloned := *spec
	return &cloned
}

func matcherCatalog(matcher *CatalogMatcher) *Catalog {
	if matcher == nil {
		return nil
	}
	return matcher.catalog
}

func unmappedIdentity(requestedModelID string) IdentityMapping {
	return IdentityMapping{
		RequestedModelID: strings.TrimSpace(requestedModelID),
		Status:           MappingStatusUnmapped,
	}
}

func ResolveMappedRoute(ctx context.Context, resolver Resolver, requestedModelID, officialModelID, catalogVersion string, status MappingStatus) (ResolvedModel, error) {
	requestedModelID = strings.TrimSpace(requestedModelID)
	officialModelID = strings.TrimSpace(officialModelID)
	catalogVersion = strings.TrimSpace(catalogVersion)
	if resolver == nil || requestedModelID == "" || officialModelID == "" || catalogVersion == "" || status != MappingStatusMapped {
		return ResolvedModel{}, ErrModelUnmapped
	}
	model, err := resolver.Resolve(ctx, requestedModelID)
	if err != nil {
		if errors.Is(err, ErrModelUnmapped) {
			return ResolvedModel{}, ErrModelMappingStale
		}
		return ResolvedModel{}, err
	}
	if model.Model.ModelID != officialModelID || model.Model.CatalogVersion != catalogVersion {
		return ResolvedModel{}, ErrModelMappingStale
	}
	if model.Model.LifecycleStatus == LifecycleRetired {
		return ResolvedModel{}, ErrModelRetired
	}
	return model, nil
}
