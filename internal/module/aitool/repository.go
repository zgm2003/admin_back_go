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
	GetRaw(ctx context.Context, id uint64) (*Tool, error)
	ExistsByCode(ctx context.Context, code string, excludeID uint64) (bool, error)
	Create(ctx context.Context, row Tool) (uint64, error)
	Update(ctx context.Context, id uint64, fields map[string]any) error
	ChangeStatus(ctx context.Context, id uint64, status int) error
	Delete(ctx context.Context, id uint64) error
	AgentExists(ctx context.Context, agentID uint64) (bool, error)
	ListAllActiveToolIDs(ctx context.Context) ([]uint64, error)
	ListBoundToolIDs(ctx context.Context, agentID uint64) ([]uint64, error)
	ReplaceAgentTools(ctx context.Context, agentID uint64, toolIDs []uint64) error
	ListRuntimeTools(ctx context.Context, agentID uint64) ([]RuntimeToolRow, error)
	StartToolCall(ctx context.Context, input StartToolCallInput) (uint64, error)
	FinishToolCall(ctx context.Context, input FinishToolCallInput) error
	CountUsers(ctx context.Context) (UserCount, error)
}

type GormRepository struct{ db *gorm.DB }

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
	db := r.activeTools(ctx)
	if strings.TrimSpace(query.Name) != "" {
		db = db.Where("name LIKE ?", strings.TrimSpace(query.Name)+"%")
	}
	if strings.TrimSpace(query.Code) != "" {
		db = db.Where("code = ?", strings.TrimSpace(query.Code))
	}
	if strings.TrimSpace(query.RiskLevel) != "" {
		db = db.Where("risk_level = ?", strings.TrimSpace(query.RiskLevel))
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	var total int64
	if err := db.Model(&Tool{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Tool
	err := db.Order("id DESC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) GetRaw(ctx context.Context, id uint64) (*Tool, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id == 0 {
		return nil, nil
	}
	var row Tool
	err := r.activeTools(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) ExistsByCode(ctx context.Context, code string, excludeID uint64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	db := r.activeTools(ctx).Model(&Tool{}).Where("code = ?", strings.TrimSpace(code))
	if excludeID > 0 {
		db = db.Where("id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) Create(ctx context.Context, row Tool) (uint64, error) {
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
	return r.activeTools(ctx).Where("id = ?", id).Updates(fields).Error
}

func (r *GormRepository) ChangeStatus(ctx context.Context, id uint64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeTools(ctx).Where("id = ?", id).Update("status", status).Error
}

func (r *GormRepository) Delete(ctx context.Context, id uint64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Tool{}).Where("id = ? AND is_del = ?", id, enum.CommonNo).Update("is_del", enum.CommonYes).Error; err != nil {
			return err
		}
		return tx.Model(&AgentTool{}).Where("tool_id = ?", id).Update("status", enum.CommonNo).Error
	})
}

func (r *GormRepository) AgentExists(ctx context.Context, agentID uint64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&Agent{}).Where("id = ? AND is_del = ?", agentID, enum.CommonNo).Count(&count).Error
	return count > 0, err
}

func (r *GormRepository) ListAllActiveToolIDs(ctx context.Context) ([]uint64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var ids []uint64
	err := r.db.WithContext(ctx).Model(&Tool{}).Where("is_del = ? AND status = ?", enum.CommonNo, enum.CommonYes).Order("id ASC").Pluck("id", &ids).Error
	return ids, err
}

func (r *GormRepository) ListBoundToolIDs(ctx context.Context, agentID uint64) ([]uint64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var ids []uint64
	err := r.db.WithContext(ctx).Model(&AgentTool{}).Where("agent_id = ? AND status = ?", agentID, enum.CommonYes).Order("tool_id ASC").Pluck("tool_id", &ids).Error
	return ids, err
}

func (r *GormRepository) ReplaceAgentTools(ctx context.Context, agentID uint64, toolIDs []uint64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&AgentTool{}).Where("agent_id = ?", agentID).Update("status", enum.CommonNo).Error; err != nil {
			return err
		}
		for _, toolID := range toolIDs {
			row := AgentTool{AgentID: agentID, ToolID: toolID, Status: enum.CommonYes}
			if err := tx.Where("agent_id = ? AND tool_id = ?", agentID, toolID).Assign(AgentTool{Status: enum.CommonYes}).FirstOrCreate(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *GormRepository) ListRuntimeTools(ctx context.Context, agentID uint64) ([]RuntimeToolRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []RuntimeToolRow
	err := r.db.WithContext(ctx).Table("ai_agent_tools at").
		Select(`t.id AS tool_id, t.name, t.code, t.description, t.parameters_json, t.result_schema_json, t.risk_level, t.timeout_ms, t.status AS tool_status, at.status AS binding_status`).
		Joins("JOIN ai_tools t ON t.id = at.tool_id AND t.is_del = ?", enum.CommonNo).
		Where("at.agent_id = ?", agentID).
		Where("at.status = ?", enum.CommonYes).
		Where("t.status = ?", enum.CommonYes).
		Order("at.id ASC").
		Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) StartToolCall(ctx context.Context, input StartToolCallInput) (uint64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	startedAt := input.StartedAt
	if startedAt.IsZero() {
		startedAt = nowUTC()
	}
	args := compactJSON(input.ArgumentsJSON)
	callID := strings.TrimSpace(input.CallID)
	row := ToolCall{RunID: input.RunID, ToolID: input.ToolID, ToolCode: input.ToolCode, ToolName: input.ToolName, Status: ToolCallRunning, ArgumentsJSON: args, StartedAt: startedAt}
	if callID != "" {
		row.CallID = &callID
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) FinishToolCall(ctx context.Context, input FinishToolCallInput) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	finishedAt := input.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = nowUTC()
	}
	fields := map[string]any{
		"status":        input.Status,
		"error_message": strings.TrimSpace(input.ErrorMessage),
		"duration_ms":   input.DurationMS,
		"finished_at":   finishedAt,
	}
	if input.ResultJSON != nil {
		fields["result_json"] = compactJSON(*input.ResultJSON)
	}
	return r.db.WithContext(ctx).Model(&ToolCall{}).Where("id = ?", input.ID).Updates(fields).Error
}

func (r *GormRepository) CountUsers(ctx context.Context) (UserCount, error) {
	if r == nil || r.db == nil {
		return UserCount{}, ErrRepositoryNotConfigured
	}
	var row UserCount
	err := r.db.WithContext(ctx).Table("users").
		Select(`COUNT(*) AS total_users,
			COALESCE(SUM(CASE WHEN status = 1 THEN 1 ELSE 0 END), 0) AS enabled_users,
			COALESCE(SUM(CASE WHEN status = 2 THEN 1 ELSE 0 END), 0) AS disabled_users`).
		Where("is_del = ?", enum.CommonNo).
		Scan(&row).Error
	return row, err
}

func (r *GormRepository) activeTools(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo)
}
