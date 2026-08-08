package aiprovider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrRepositoryNotConfigured   = errors.New("aiprovider repository not configured")
	ErrProviderModelInUse        = errors.New("provider model is in use")
	ErrProviderModelKindConflict = errors.New("provider model kind conflicts with reviewed identity")
)

type ModelReconcileScope string

const (
	ModelReconcileChatOnly ModelReconcileScope = "chat_only"
	ModelReconcileAll      ModelReconcileScope = "all"
)

func (scope ModelReconcileScope) Validate() error {
	switch scope {
	case ModelReconcileChatOnly, ModelReconcileAll:
		return nil
	default:
		return fmt.Errorf("invalid provider model reconcile scope %q", scope)
	}
}

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]Provider, int64, error)
	Get(ctx context.Context, id uint64) (*Provider, error)
	ExistsByTypeName(ctx context.Context, engineType string, name string, excludeID uint64) (bool, error)
	Create(ctx context.Context, row Provider) (uint64, error)
	Update(ctx context.Context, id uint64, fields map[string]any) error
	ChangeStatus(ctx context.Context, id uint64, status int) error
	ListModels(ctx context.Context, providerID uint64) ([]ProviderModel, error)
	ListAllModels(ctx context.Context) ([]ProviderModel, error)
	UpdateModelMapping(ctx context.Context, id uint64, mapping officialmodel.IdentityMapping) error
	ReconcileModels(ctx context.Context, providerID uint64, scope ModelReconcileScope, models []ProviderModel) error
	Delete(ctx context.Context, id uint64) error
}

type discoveredModelMerger interface {
	MergeDiscoveredModels(context.Context, uint64, []ProviderModel) error
}

func (r *GormRepository) MergeDiscoveredModels(ctx context.Context, providerID uint64, models []ProviderModel) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if providerID == 0 {
		return errors.New("provider id is required")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing []ProviderModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("provider_id = ?", providerID).Order("id ASC").Find(&existing).Error; err != nil {
			return err
		}
		byModelID := make(map[string][]ProviderModel, len(existing))
		for _, row := range existing {
			byModelID[row.ModelID] = append(byModelID[row.ModelID], row)
		}
		type mergeMutation struct {
			current *ProviderModel
			desired ProviderModel
		}
		mutations := make([]mergeMutation, 0, len(models))
		seen := make(map[providerModelIdentity]struct{}, len(models))
		for _, model := range models {
			model.ProviderID = providerID
			model.ModelID = strings.TrimSpace(model.ModelID)
			if model.ModelID == "" || model.MappingStatus != officialmodel.MappingStatusMapped || model.OfficialModelID == nil || model.OfficialCatalogVersion == nil || model.MappedAt == nil || model.ModelKind.Validate() != nil {
				return errors.New("discovered provider model is not a reviewed mapping")
			}
			if model.ModelKind == ModelKindEmbedding && !providerModelHasCompleteEmbeddingSpec(model) {
				return errors.New("reviewed embedding model has no complete spec")
			}
			identity := providerModelIdentity{modelID: model.ModelID, kind: model.ModelKind}
			if _, duplicate := seen[identity]; duplicate {
				continue
			}
			seen[identity] = struct{}{}
			var current *ProviderModel
			for _, row := range byModelID[model.ModelID] {
				if row.ModelKind != model.ModelKind {
					return ErrProviderModelKindConflict
				}
				matched := row
				current = &matched
			}
			if current != nil {
				model.ID = current.ID
				model.DisplayName = current.DisplayName
				model.Status = current.Status
				if !providerModelEmbeddingSpecEqual(*current, model) {
					used, err := providerModelReferenced(tx, current.ID)
					if err != nil {
						return err
					}
					if used {
						return ErrProviderModelInUse
					}
				}
			} else {
				model.Status = enum.CommonYes
				if strings.TrimSpace(model.DisplayName) == "" {
					model.DisplayName = model.ModelID
				}
			}
			mutations = append(mutations, mergeMutation{current: current, desired: model})
		}
		for _, mutation := range mutations {
			if mutation.current == nil {
				mutation.desired.ID = 0
				if err := tx.Create(&mutation.desired).Error; err != nil {
					return err
				}
				continue
			}
			if err := tx.Model(&ProviderModel{}).Where("id = ?", mutation.current.ID).Updates(providerModelMutableFields(mutation.desired)).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *GormRepository) ListAllModels(ctx context.Context) ([]ProviderModel, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []ProviderModel
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *GormRepository) UpdateModelMapping(ctx context.Context, id uint64, mapping officialmodel.IdentityMapping) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	fields := map[string]any{
		"official_model_id":        nil,
		"official_catalog_version": nil,
		"mapping_status":           mapping.Status,
		"mapped_at":                nil,
	}
	if mapping.Status == officialmodel.MappingStatusMapped {
		fields["official_model_id"] = mapping.OfficialModelID
		fields["official_catalog_version"] = mapping.CatalogVersion
		fields["mapped_at"] = mapping.MappedAt
	}
	return r.db.WithContext(ctx).
		Model(&ProviderModel{}).
		Where("id = ?", id).
		UpdateColumns(fields).Error
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]Provider, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.activeDB(ctx)
	if strings.TrimSpace(query.Name) != "" {
		db = db.Where("name LIKE ?", strings.TrimSpace(query.Name)+"%")
	}
	if strings.TrimSpace(query.EngineType) != "" {
		db = db.Where("engine_type = ?", strings.TrimSpace(query.EngineType))
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	var total int64
	if err := db.Model(&Provider{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Provider
	if err := db.Order("id DESC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) Get(ctx context.Context, id uint64) (*Provider, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id == 0 {
		return nil, nil
	}
	var row Provider
	err := r.activeDB(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) ExistsByTypeName(ctx context.Context, engineType string, name string, excludeID uint64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	db := r.activeDB(ctx).Model(&Provider{}).Where("engine_type = ?", strings.TrimSpace(engineType)).Where("name = ?", strings.TrimSpace(name))
	if excludeID > 0 {
		db = db.Where("id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) Create(ctx context.Context, row Provider) (uint64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeDB(ctx).Model(&Provider{}).Where("id = ?", id).Updates(fields).Error
}

func (r *GormRepository) ListModels(ctx context.Context, providerID uint64) ([]ProviderModel, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if providerID == 0 {
		return nil, nil
	}
	var rows []ProviderModel
	if err := r.db.WithContext(ctx).Where("provider_id = ?", providerID).Order("model_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *GormRepository) ReconcileModels(ctx context.Context, providerID uint64, scope ModelReconcileScope, models []ProviderModel) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if providerID == 0 {
		return errors.New("provider id is required")
	}
	if err := scope.Validate(); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing []ProviderModel
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("provider_id = ?", providerID)
		if scope == ModelReconcileChatOnly {
			query = query.Where("model_kind = ?", ModelKindChat)
		}
		if err := query.Order("id ASC").Find(&existing).Error; err != nil {
			return err
		}

		existingByID := make(map[uint64]ProviderModel, len(existing))
		existingByIdentity := make(map[providerModelIdentity]ProviderModel, len(existing))
		for _, row := range existing {
			existingByID[row.ID] = row
			existingByIdentity[providerModelIdentity{modelID: row.ModelID, kind: row.ModelKind}] = row
		}
		type modelMutation struct {
			current *ProviderModel
			desired ProviderModel
		}
		mutations := make([]modelMutation, 0, len(models))
		seen := make(map[providerModelIdentity]struct{}, len(models))
		seenIDs := make(map[uint64]struct{}, len(models))
		for _, model := range models {
			model.ModelID = strings.TrimSpace(model.ModelID)
			if model.ModelID == "" {
				return errors.New("provider model id is required")
			}
			if err := model.ModelKind.Validate(); err != nil {
				return err
			}
			if scope == ModelReconcileChatOnly && model.ModelKind != ModelKindChat {
				return errors.New("chat-only reconciliation received a non-chat model")
			}
			identity := providerModelIdentity{modelID: model.ModelID, kind: model.ModelKind}
			if _, duplicated := seen[identity]; duplicated {
				return fmt.Errorf("duplicate provider model identity %q/%q", model.ModelID, model.ModelKind)
			}
			seen[identity] = struct{}{}
			model.ProviderID = providerID
			if model.Status == 0 {
				model.Status = enum.CommonYes
			}
			var current *ProviderModel
			if model.ID > 0 {
				if _, duplicate := seenIDs[model.ID]; duplicate {
					return fmt.Errorf("duplicate provider model id %d", model.ID)
				}
				seenIDs[model.ID] = struct{}{}
				row, exists := existingByID[model.ID]
				if !exists {
					return fmt.Errorf("provider model id %d does not belong to provider %d", model.ID, providerID)
				}
				current = &row
				if conflict, exists := existingByIdentity[identity]; exists && conflict.ID != row.ID {
					return ErrProviderModelKindConflict
				}
			} else if row, exists := existingByIdentity[identity]; exists {
				current = &row
				model.ID = row.ID
			}
			if current != nil {
				if err := preserveLegacyEmbeddingSpec(&model, *current); err != nil {
					return err
				}
				identityChanged := current.ModelID != model.ModelID || current.ModelKind != model.ModelKind
				specChanged := !providerModelEmbeddingSpecEqual(*current, model)
				if identityChanged || specChanged {
					used, err := providerModelReferenced(tx, current.ID)
					if err != nil {
						return err
					}
					if used {
						return ErrProviderModelInUse
					}
				}
				model.ID = current.ID
			} else if model.ModelKind == ModelKindEmbedding && !providerModelHasCompleteEmbeddingSpec(model) {
				return errors.New("new embedding provider model requires a complete spec")
			}
			mutations = append(mutations, modelMutation{current: current, desired: model})
		}
		for _, row := range existing {
			identity := providerModelIdentity{modelID: row.ModelID, kind: row.ModelKind}
			if _, retained := seen[identity]; retained || row.Status == enum.CommonNo {
				continue
			}
			used, err := providerModelReferenced(tx, row.ID)
			if err != nil {
				return err
			}
			if used {
				return ErrProviderModelInUse
			}
			mutations = append(mutations, modelMutation{current: &row, desired: ProviderModel{ID: row.ID, Status: enum.CommonNo}})
		}
		for _, mutation := range mutations {
			if mutation.current == nil {
				mutation.desired.ID = 0
				if err := tx.Create(&mutation.desired).Error; err != nil {
					return err
				}
				continue
			}
			if mutation.desired.ModelID == "" {
				if err := tx.Model(&ProviderModel{}).Where("id = ?", mutation.current.ID).Update("status", enum.CommonNo).Error; err != nil {
					return err
				}
				continue
			}
			if err := tx.Model(&ProviderModel{}).Where("id = ?", mutation.current.ID).Updates(providerModelMutableFields(mutation.desired)).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

type providerModelIdentity struct {
	modelID string
	kind    ModelKind
}

func providerModelMutableFields(model ProviderModel) map[string]any {
	return map[string]any{
		"model_id":                   model.ModelID,
		"model_kind":                 model.ModelKind,
		"display_name":               model.DisplayName,
		"official_model_id":          model.OfficialModelID,
		"official_catalog_version":   model.OfficialCatalogVersion,
		"mapping_status":             model.MappingStatus,
		"mapped_at":                  model.MappedAt,
		"embedding_dimensions":       model.EmbeddingDimensions,
		"embedding_max_input_tokens": model.EmbeddingMaxInputTokens,
		"embedding_token_counter_id": model.EmbeddingTokenCounterID,
		"status":                     model.Status,
	}
}

func preserveLegacyEmbeddingSpec(model *ProviderModel, current ProviderModel) error {
	if model == nil || model.ModelKind != ModelKindEmbedding || model.EmbeddingDimensions != nil || model.EmbeddingMaxInputTokens != nil || model.EmbeddingTokenCounterID != nil {
		return nil
	}
	if current.ModelKind != ModelKindEmbedding {
		return errors.New("embedding provider model requires a complete spec")
	}
	if providerModelHasCompleteEmbeddingSpec(current) {
		model.EmbeddingDimensions = current.EmbeddingDimensions
		model.EmbeddingMaxInputTokens = current.EmbeddingMaxInputTokens
		model.EmbeddingTokenCounterID = current.EmbeddingTokenCounterID
		return nil
	}
	if current.Status == enum.CommonNo && model.Status == enum.CommonNo {
		return nil
	}
	return errors.New("embedding provider model requires a complete spec")
}

func providerModelHasCompleteEmbeddingSpec(model ProviderModel) bool {
	return model.EmbeddingDimensions != nil && *model.EmbeddingDimensions > 0 &&
		model.EmbeddingMaxInputTokens != nil && *model.EmbeddingMaxInputTokens > 0 &&
		model.EmbeddingTokenCounterID != nil && strings.TrimSpace(*model.EmbeddingTokenCounterID) != ""
}

func providerModelEmbeddingSpecEqual(left ProviderModel, right ProviderModel) bool {
	return equalUint32(left.EmbeddingDimensions, right.EmbeddingDimensions) &&
		equalInt64(left.EmbeddingMaxInputTokens, right.EmbeddingMaxInputTokens) &&
		equalString(left.EmbeddingTokenCounterID, right.EmbeddingTokenCounterID)
}

func equalUint32(left *uint32, right *uint32) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func equalInt64(left *int64, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func equalString(left *string, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func providerModelReferenced(tx *gorm.DB, id uint64) (bool, error) {
	var referenced bool
	err := tx.Raw(`SELECT EXISTS(
		SELECT 1 FROM ai_agents WHERE provider_model_id = ?
		UNION ALL SELECT 1 FROM ai_context_profiles WHERE embedding_provider_model_id = ?
		UNION ALL SELECT 1 FROM ai_context_profiles WHERE reranker_provider_model_id = ?
		UNION ALL SELECT 1 FROM ai_context_profiles WHERE memory_provider_model_id = ?
	)`, id, id, id, id).Scan(&referenced).Error
	return referenced, err
}

func (r *GormRepository) ChangeStatus(ctx context.Context, id uint64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeDB(ctx).Model(&Provider{}).Where("id = ?", id).Update("status", status).Error
}

func (r *GormRepository) Delete(ctx context.Context, id uint64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeDB(ctx).Model(&Provider{}).Where("id = ?", id).Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) activeDB(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo)
}
