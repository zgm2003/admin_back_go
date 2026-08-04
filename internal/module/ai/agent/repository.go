package aiagent

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/ai/officialmodel"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aiagent repository not configured")

type AgentWithProvider struct {
	Agent
	ProviderName           string                      `gorm:"column:provider_name"`
	EngineType             string                      `gorm:"column:engine_type"`
	APIProtocol            string                      `gorm:"column:api_protocol"`
	ProviderStatus         int                         `gorm:"column:provider_status"`
	ProviderModelStatus    int                         `gorm:"column:provider_model_status"`
	OfficialModelID        string                      `gorm:"column:official_model_id"`
	OfficialCatalogVersion string                      `gorm:"column:official_catalog_version"`
	MappingStatus          officialmodel.MappingStatus `gorm:"column:mapping_status"`
}

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]AgentWithProvider, int64, error)
	Get(ctx context.Context, id uint64) (*AgentWithProvider, error)
	GetRaw(ctx context.Context, id uint64) (*Agent, error)
	ListActiveProviders(ctx context.Context) ([]Provider, error)
	GetActiveProvider(ctx context.Context, id uint64) (*Provider, error)
	ListProviderModels(ctx context.Context, providerID uint64) ([]ProviderModel, error)
	Create(ctx context.Context, row Agent) (uint64, error)
	Update(ctx context.Context, id uint64, fields map[string]any) error
	ChangeStatus(ctx context.Context, id uint64, status int) error
	Delete(ctx context.Context, id uint64) error
	ListVisibleAgents(ctx context.Context, query OptionQuery) ([]AgentWithProvider, error)
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]AgentWithProvider, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.agentSelectDB(ctx)
	if strings.TrimSpace(query.Name) != "" {
		db = db.Where("a.name LIKE ?", strings.TrimSpace(query.Name)+"%")
	}
	if strings.TrimSpace(query.Scene) != "" {
		db = db.Where("JSON_CONTAINS(a.scenes_json, JSON_QUOTE(?))", strings.TrimSpace(query.Scene))
	}
	if query.ProviderID > 0 {
		db = db.Where("a.provider_id = ?", query.ProviderID)
	}
	if query.Status != nil {
		db = db.Where("a.status = ?", *query.Status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []AgentWithProvider
	if err := db.Order("a.id DESC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) Get(ctx context.Context, id uint64) (*AgentWithProvider, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id == 0 {
		return nil, nil
	}
	var row AgentWithProvider
	err := r.agentSelectDB(ctx).Where("a.id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) GetRaw(ctx context.Context, id uint64) (*Agent, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id == 0 {
		return nil, nil
	}
	var row Agent
	err := r.activeAgents(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) ListActiveProviders(ctx context.Context) ([]Provider, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []Provider
	err := r.db.WithContext(ctx).Where("is_del = ? AND status = ?", enum.CommonNo, enum.CommonYes).Order("id DESC").Find(&rows).Error
	return rows, err
}

func (r *GormRepository) GetActiveProvider(ctx context.Context, id uint64) (*Provider, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id == 0 {
		return nil, nil
	}
	var row Provider
	err := r.db.WithContext(ctx).Where("id = ? AND is_del = ? AND status = ?", id, enum.CommonNo, enum.CommonYes).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) ListProviderModels(ctx context.Context, providerID uint64) ([]ProviderModel, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if providerID == 0 {
		return nil, nil
	}
	var rows []ProviderModel
	if err := r.db.WithContext(ctx).Where("provider_id = ? AND model_kind = ? AND status = ?", providerID, aiprovider.ModelKindChat, enum.CommonYes).Order("model_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *GormRepository) Create(ctx context.Context, row Agent) (uint64, error) {
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
	return r.db.WithContext(ctx).Model(&Agent{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Updates(fields).Error
}

func (r *GormRepository) ChangeStatus(ctx context.Context, id uint64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Agent{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Update("status", status).Error
}

func (r *GormRepository) Delete(ctx context.Context, id uint64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Agent{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) ListVisibleAgents(ctx context.Context, query OptionQuery) ([]AgentWithProvider, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	scene := strings.TrimSpace(query.Scene)
	if scene == "" {
		scene = sceneChat
	}
	var rows []AgentWithProvider
	if err := r.agentSelectDB(ctx).
		Where("e.status = ?", enum.CommonYes).
		Where("pm.status = ? AND pm.mapping_status = ?", enum.CommonYes, officialmodel.MappingStatusMapped).
		Where("a.is_del = ?", enum.CommonNo).
		Where("a.status = ?", enum.CommonYes).
		Where("JSON_CONTAINS(a.scenes_json, JSON_QUOTE(?))", scene).
		Order("a.id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *GormRepository) agentSelectDB(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Table("ai_agents AS a").Select(`
		a.*, e.name AS provider_name, e.engine_type AS engine_type,
		e.api_protocol AS api_protocol, e.status AS provider_status,
		pm.status AS provider_model_status,
		pm.official_model_id AS official_model_id,
		pm.official_catalog_version AS official_catalog_version,
		pm.mapping_status AS mapping_status`).
		Joins("LEFT JOIN ai_providers e ON e.id = a.provider_id AND e.is_del = ?", enum.CommonNo).
		Joins("LEFT JOIN ai_provider_models pm ON pm.id = a.provider_model_id AND pm.provider_id = a.provider_id AND pm.model_id = a.model_id AND pm.model_kind = ?", aiprovider.ModelKindChat).
		Where("a.is_del = ?", enum.CommonNo)
}

func (r *GormRepository) activeAgents(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo)
}
