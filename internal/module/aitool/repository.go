package aitool

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aitool repository not configured")

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]Tool, int64, error)
	Get(ctx context.Context, id int64) (*Tool, error)
	ExistsByCode(ctx context.Context, code string, excludeID int64) (bool, error)
	Create(ctx context.Context, row Tool) (int64, error)
	Update(ctx context.Context, id int64, fields map[string]any) error
	ChangeStatus(ctx context.Context, id int64, status int) error
	Delete(ctx context.Context, id int64) error
	HasActiveBindings(ctx context.Context, toolID int64) (bool, error)
	ActiveOptions(ctx context.Context) ([]ToolOptionRow, error)
	BoundToolIDs(ctx context.Context, agentID int64) ([]int64, error)
	SyncBindings(ctx context.Context, agentID int64, toolIDs []int64) error
}

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]Tool, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.activeToolDB(ctx)
	name := strings.TrimSpace(query.Name)
	if name != "" {
		db = db.Where("name LIKE ?", name+"%")
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	if query.ExecutorType != nil {
		db = db.Where("executor_type = ?", *query.ExecutorType)
	}

	var total int64
	if err := db.Model(&Tool{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Tool
	if err := db.Order("id DESC").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) Get(ctx context.Context, id int64) (*Tool, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}
	var row Tool
	err := r.activeToolDB(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) ExistsByCode(ctx context.Context, code string, excludeID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	db := r.activeToolDB(ctx).Model(&Tool{}).Where("code = ?", strings.TrimSpace(code))
	if excludeID > 0 {
		db = db.Where("id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) Create(ctx context.Context, row Tool) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) Update(ctx context.Context, id int64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeToolDB(ctx).Where("id = ?", id).Updates(fields).Error
}

func (r *GormRepository) ChangeStatus(ctx context.Context, id int64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeToolDB(ctx).Where("id = ?", id).Update("status", status).Error
}

func (r *GormRepository) Delete(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	suffix := "__del_" + int64String(id)
	maxPrefix := 60 - len(suffix)
	if maxPrefix < 0 {
		maxPrefix = 0
	}
	return r.activeToolDB(ctx).Where("id = ?", id).Updates(map[string]any{
		"is_del": enum.CommonYes,
		"code":   gorm.Expr("CONCAT(LEFT(code, ?), ?)", maxPrefix, suffix),
	}).Error
}

func (r *GormRepository) HasActiveBindings(ctx context.Context, toolID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&AssistantTool{}).
		Where("tool_id = ?", toolID).
		Where("is_del = ?", enum.CommonNo).
		Where("status = ?", enum.CommonYes).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) ActiveOptions(ctx context.Context) ([]ToolOptionRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []ToolOptionRow
	err := r.db.WithContext(ctx).Model(&Tool{}).
		Select("id, name, code").
		Where("is_del = ?", enum.CommonNo).
		Where("status = ?", enum.CommonYes).
		Order("id DESC").
		Find(&rows).Error
	return rows, err
}

func (r *GormRepository) BoundToolIDs(ctx context.Context, agentID int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var ids []int64
	err := r.db.WithContext(ctx).Model(&AssistantTool{}).
		Where("assistant_id = ?", agentID).
		Where("is_del = ?", enum.CommonNo).
		Where("status = ?", enum.CommonYes).
		Order("tool_id ASC").
		Pluck("tool_id", &ids).Error
	return ids, err
}

func (r *GormRepository) SyncBindings(ctx context.Context, agentID int64, toolIDs []int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := &GormRepository{db: tx}
		return repo.syncBindings(ctx, agentID, toolIDs)
	})
}

func (r *GormRepository) syncBindings(ctx context.Context, agentID int64, toolIDs []int64) error {
	var existing []AssistantTool
	if err := r.db.WithContext(ctx).
		Where("assistant_id = ?", agentID).
		Find(&existing).Error; err != nil {
		return err
	}

	nextSet := make(map[int64]struct{}, len(toolIDs))
	for _, id := range toolIDs {
		nextSet[id] = struct{}{}
	}
	for _, row := range existing {
		if _, ok := nextSet[row.ToolID]; ok {
			if row.IsDel != enum.CommonNo || row.Status != enum.CommonYes {
				if err := r.db.WithContext(ctx).Model(&AssistantTool{}).
					Where("id = ?", row.ID).
					Updates(map[string]any{"is_del": enum.CommonNo, "status": enum.CommonYes}).Error; err != nil {
					return err
				}
			}
			delete(nextSet, row.ToolID)
			continue
		}
		if row.IsDel == enum.CommonNo {
			if err := r.db.WithContext(ctx).Model(&AssistantTool{}).
				Where("id = ?", row.ID).
				Updates(map[string]any{"is_del": enum.CommonYes, "status": enum.CommonNo}).Error; err != nil {
				return err
			}
		}
	}
	for toolID := range nextSet {
		row := AssistantTool{AssistantID: agentID, ToolID: toolID, Status: enum.CommonYes, IsDel: enum.CommonNo}
		if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *GormRepository) activeToolDB(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo)
}
