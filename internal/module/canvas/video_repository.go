package canvas

import (
	"context"
	"errors"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

type VideoGormRepository struct{ db *gorm.DB }

func NewVideoGormRepository(client *database.Client) *VideoGormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &VideoGormRepository{db: client.Gorm}
}

func (r *VideoGormRepository) AgentForVideoRuntime(ctx context.Context, agentID int64) (*VideoAgentRuntime, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if agentID <= 0 {
		return nil, nil
	}
	var row VideoAgentRuntime
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
		Joins("JOIN ai_providers e ON e.id = a.provider_id AND e.is_del = ? AND e.status = ?", enum.CommonNo, enum.CommonYes).
		Where("a.id = ? AND a.is_del = ? AND a.status = ?", agentID, enum.CommonNo, enum.CommonYes).
		Limit(1).
		Scan(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.AgentID == 0 {
		return nil, nil
	}
	return &row, nil
}
