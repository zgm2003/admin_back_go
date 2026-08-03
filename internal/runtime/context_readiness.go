package runtime

import (
	"context"
	"errors"

	"admin_back_go/internal/infra/contextindex"
	contextqdrant "admin_back_go/internal/infra/contextindex/qdrant"
	"admin_back_go/internal/readiness"

	"gorm.io/gorm"
)

type ContextIndexReadiness interface {
	CheckReadiness(context.Context, []contextindex.ActiveCollection) error
}

type ContextSourceReadiness interface {
	ActiveCollections(context.Context) ([]contextindex.ActiveCollection, error)
}

type ContextReadiness struct {
	index    ContextIndexReadiness
	sources  ContextSourceReadiness
	required bool
}

type qdrantContextIndex struct {
	client           *contextqdrant.Client
	collectionPrefix string
}

func (index qdrantContextIndex) CheckReadiness(ctx context.Context, active []contextindex.ActiveCollection) error {
	if index.client == nil {
		return errors.New("Qdrant client is unavailable")
	}
	return index.client.CheckReadiness(ctx, index.collectionPrefix, active)
}

func NewContextReadiness(index ContextIndexReadiness, sources ContextSourceReadiness) *ContextReadiness {
	return &ContextReadiness{index: index, sources: sources}
}

func NewWorkerContextReadiness(index ContextIndexReadiness, sources ContextSourceReadiness) *ContextReadiness {
	return &ContextReadiness{index: index, sources: sources, required: true}
}

func (checker *ContextReadiness) Check(ctx context.Context) readiness.Check {
	if checker == nil || checker.index == nil || checker.sources == nil {
		return readiness.Check{Status: readiness.StatusDown, Message: "context readiness is not configured"}
	}
	collections, err := checker.sources.ActiveCollections(ctx)
	if err != nil {
		return readiness.Check{Status: readiness.StatusDown, Message: "context source readiness is unavailable"}
	}
	if err := checker.index.CheckReadiness(ctx, collections); err == nil {
		return readiness.Check{Status: readiness.StatusUp}
	}
	if checker.required || len(collections) > 0 {
		return readiness.Check{Status: readiness.StatusDown, Message: "context index is unavailable"}
	}
	return readiness.Check{Status: readiness.StatusDegraded, Message: "context index is unavailable"}
}

type gormContextSources struct {
	db *gorm.DB
}

func newGormContextSources(db *gorm.DB) *gormContextSources {
	return &gormContextSources{db: db}
}

func (sources *gormContextSources) ActiveCollections(ctx context.Context) ([]contextindex.ActiveCollection, error) {
	if sources == nil || sources.db == nil {
		return nil, errors.New("context source database is unavailable")
	}
	var rows []struct {
		ProfileID       uint64                `gorm:"column:profile_id"`
		IndexGeneration *uint64               `gorm:"column:index_generation"`
		IndexState      string                `gorm:"column:index_state"`
		DenseDimensions uint64                `gorm:"column:dense_dimensions"`
		DenseDistance   contextindex.Distance `gorm:"column:dense_distance"`
	}
	err := sources.db.WithContext(ctx).Raw(`
SELECT DISTINCT
  p.id AS profile_id,
  p.active_index_generation AS index_generation,
  p.index_state AS index_state,
  p.embedding_dimensions AS dense_dimensions,
  p.dense_distance AS dense_distance
FROM ai_context_profiles AS p
JOIN ai_context_spaces AS s
  ON s.profile_id = p.id
 AND s.status = 'enabled'
 AND s.deleted_at IS NULL
JOIN ai_context_documents AS d
  ON d.space_id = s.id
 AND d.status = 'enabled'
 AND d.deleted_at IS NULL
 AND d.active_version_id IS NOT NULL
JOIN ai_context_document_versions AS v
  ON v.id = d.active_version_id
 AND v.document_id = d.id
 AND v.profile_id = p.id
 AND v.state = 'ready'
WHERE p.status = 'enabled'
ORDER BY p.id ASC`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	collections := make([]contextindex.ActiveCollection, len(rows))
	for index, row := range rows {
		if row.IndexGeneration == nil || (row.IndexState != "ready" && row.IndexState != "rebuilding") {
			return nil, errors.New("active context source has no readable index generation")
		}
		collection := contextindex.ActiveCollection{
			ProfileID:       row.ProfileID,
			IndexGeneration: *row.IndexGeneration,
			DenseDimensions: row.DenseDimensions,
			DenseDistance:   row.DenseDistance,
		}
		if err := collection.Validate(); err != nil {
			return nil, err
		}
		collections[index] = collection
	}
	return collections, nil
}
