package aiknowledgemap

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aiknowledgemap repository not configured")

type MapWithEngine struct {
	KnowledgeMap
	EngineConnectionName string `gorm:"column:engine_connection_name"`
	EngineType           string `gorm:"column:engine_type"`
	EngineBaseURL        string `gorm:"column:engine_base_url"`
	EngineAPIKeyEnc      string `gorm:"column:engine_api_key_enc"`
}

type DocumentWithMap struct {
	Document
	EngineConnectionID uint64 `gorm:"column:engine_connection_id"`
	EngineDatasetID    string `gorm:"column:engine_dataset_id"`
	EngineType         string `gorm:"column:engine_type"`
	EngineBaseURL      string `gorm:"column:engine_base_url"`
	EngineAPIKeyEnc    string `gorm:"column:engine_api_key_enc"`
	MapStatus          int    `gorm:"column:map_status"`
}

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]MapWithEngine, int64, error)
	Get(ctx context.Context, id uint64) (*MapWithEngine, error)
	GetRaw(ctx context.Context, id uint64) (*KnowledgeMap, error)
	ListActiveConnections(ctx context.Context) ([]EngineConnection, error)
	GetActiveConnection(ctx context.Context, id uint64) (*EngineConnection, error)
	ExistsByCode(ctx context.Context, code string, excludeID uint64) (bool, error)
	Create(ctx context.Context, row KnowledgeMap) (uint64, error)
	Update(ctx context.Context, id uint64, fields map[string]any) error
	ChangeStatus(ctx context.Context, id uint64, status int) error
	Delete(ctx context.Context, id uint64) error
	ListDocuments(ctx context.Context, mapID uint64) ([]Document, error)
	CreateDocument(ctx context.Context, row Document) (uint64, error)
	GetDocument(ctx context.Context, id uint64) (*DocumentWithMap, error)
	UpdateDocument(ctx context.Context, id uint64, fields map[string]any) error
	ChangeDocumentStatus(ctx context.Context, id uint64, status int) error
	DeleteDocument(ctx context.Context, id uint64) error
	ListSyncableDocuments(ctx context.Context, mapID uint64) ([]Document, error)
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]MapWithEngine, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.mapSelectDB(ctx)
	if strings.TrimSpace(query.Name) != "" {
		db = db.Where("m.name LIKE ?", strings.TrimSpace(query.Name)+"%")
	}
	if strings.TrimSpace(query.Code) != "" {
		db = db.Where("m.code = ?", strings.TrimSpace(query.Code))
	}
	if strings.TrimSpace(query.Visibility) != "" {
		db = db.Where("m.visibility = ?", strings.TrimSpace(query.Visibility))
	}
	if query.EngineConnectionID > 0 {
		db = db.Where("m.engine_connection_id = ?", query.EngineConnectionID)
	}
	if query.Status != nil {
		db = db.Where("m.status = ?", *query.Status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []MapWithEngine
	if err := db.Order("m.id DESC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) Get(ctx context.Context, id uint64) (*MapWithEngine, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row MapWithEngine
	err := r.mapSelectDB(ctx).Where("m.id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) GetRaw(ctx context.Context, id uint64) (*KnowledgeMap, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row KnowledgeMap
	err := r.activeMaps(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) ListActiveConnections(ctx context.Context) ([]EngineConnection, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []EngineConnection
	err := r.db.WithContext(ctx).Where("is_del = ? AND status = ?", enum.CommonNo, enum.CommonYes).Order("id DESC").Find(&rows).Error
	return rows, err
}

func (r *GormRepository) GetActiveConnection(ctx context.Context, id uint64) (*EngineConnection, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row EngineConnection
	err := r.db.WithContext(ctx).Where("id = ? AND is_del = ? AND status = ?", id, enum.CommonNo, enum.CommonYes).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) ExistsByCode(ctx context.Context, code string, excludeID uint64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	db := r.activeMaps(ctx).Model(&KnowledgeMap{}).Where("code = ?", strings.TrimSpace(code))
	if excludeID > 0 {
		db = db.Where("id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) Create(ctx context.Context, row KnowledgeMap) (uint64, error) {
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
	return r.activeMaps(ctx).Where("id = ?", id).Updates(fields).Error
}

func (r *GormRepository) ChangeStatus(ctx context.Context, id uint64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeMaps(ctx).Where("id = ?", id).Update("status", status).Error
}

func (r *GormRepository) Delete(ctx context.Context, id uint64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeMaps(ctx).Where("id = ?", id).Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) ListDocuments(ctx context.Context, mapID uint64) ([]Document, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []Document
	err := r.db.WithContext(ctx).Where("is_del = ? AND knowledge_map_id = ?", enum.CommonNo, mapID).Order("id DESC").Find(&rows).Error
	return rows, err
}

func (r *GormRepository) CreateDocument(ctx context.Context, row Document) (uint64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) GetDocument(ctx context.Context, id uint64) (*DocumentWithMap, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row DocumentWithMap
	err := r.db.WithContext(ctx).Table("ai_knowledge_documents AS d").
		Select(`d.*, m.engine_connection_id, m.engine_dataset_id, m.status AS map_status, e.engine_type, e.base_url AS engine_base_url, e.api_key_enc AS engine_api_key_enc`).
		Joins("JOIN ai_knowledge_maps m ON m.id = d.knowledge_map_id AND m.is_del = ?", enum.CommonNo).
		Joins("LEFT JOIN ai_engine_connections e ON e.id = m.engine_connection_id AND e.is_del = ?", enum.CommonNo).
		Where("d.id = ? AND d.is_del = ?", id, enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) UpdateDocument(ctx context.Context, id uint64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Document{}).Where("id = ? AND is_del = ?", id, enum.CommonNo).Updates(fields).Error
}

func (r *GormRepository) ChangeDocumentStatus(ctx context.Context, id uint64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Document{}).Where("id = ? AND is_del = ?", id, enum.CommonNo).Update("status", status).Error
}

func (r *GormRepository) DeleteDocument(ctx context.Context, id uint64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Document{}).Where("id = ? AND is_del = ?", id, enum.CommonNo).Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) ListSyncableDocuments(ctx context.Context, mapID uint64) ([]Document, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []Document
	err := r.db.WithContext(ctx).Where("is_del = ? AND status = ? AND knowledge_map_id = ? AND source_type = ?", enum.CommonNo, enum.CommonYes, mapID, SourceTypeText).Order("id ASC").Find(&rows).Error
	return rows, err
}

func (r *GormRepository) mapSelectDB(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Table("ai_knowledge_maps AS m").
		Select(`m.*, e.name AS engine_connection_name, e.engine_type AS engine_type, e.base_url AS engine_base_url, e.api_key_enc AS engine_api_key_enc`).
		Joins("LEFT JOIN ai_engine_connections e ON e.id = m.engine_connection_id AND e.is_del = ?", enum.CommonNo).
		Where("m.is_del = ?", enum.CommonNo)
}

func (r *GormRepository) activeMaps(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo)
}
