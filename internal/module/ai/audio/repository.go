package aiaudio

import (
	"context"
	"errors"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aiaudio repository not configured")

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) AgentForRuntime(ctx context.Context, agentID int64) (*AgentRuntime, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if agentID <= 0 {
		return nil, nil
	}
	var row AgentRuntime
	err := r.db.WithContext(ctx).Table("ai_agents AS a").
		Select(`a.id AS agent_id,
			a.provider_id AS provider_id,
			a.model_id AS model_id,
			a.model_display_name AS model_display_name,
			a.system_prompt AS system_prompt,
			a.scenes_json AS scenes_json,
			a.status AS agent_status,
			e.engine_type AS engine_type,
			e.base_url AS engine_base_url,
			e.api_key_enc AS engine_api_key_enc,
			e.status AS engine_status`).
		Joins("JOIN ai_providers e ON e.id = a.provider_id AND e.is_del = ?", enum.CommonNo).
		Where("a.id = ? AND a.is_del = ?", agentID, enum.CommonNo).
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.AgentID == 0 {
		return nil, nil
	}
	return &row, nil
}
