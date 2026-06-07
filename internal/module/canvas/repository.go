package canvas

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("canvas repository not configured")

type Repository interface {
	ListAgentsByScene(ctx context.Context, scene string) ([]CanvasAgentOption, error)
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) ListAgentsByScene(ctx context.Context, scene string) ([]CanvasAgentOption, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	scene = strings.TrimSpace(scene)
	if scene == "" {
		return []CanvasAgentOption{}, nil
	}
	var rows []CanvasAgentOption
	err := r.db.WithContext(ctx).Table("ai_agents AS a").
		Select(`a.id AS id,
			a.name AS name,
			a.avatar AS avatar,
			a.model_id AS model_id,
			COALESCE(NULLIF(a.model_display_name, ''), NULLIF(m.display_name, ''), a.model_id) AS model_display_name,
			? AS scene`, scene).
		Joins("JOIN ai_providers AS p ON p.id = a.provider_id AND p.is_del = ? AND p.status = ?", enum.CommonNo, enum.CommonYes).
		Joins("JOIN ai_provider_models AS m ON m.provider_id = a.provider_id AND m.model_id = a.model_id AND m.status = ?", enum.CommonYes).
		Where("a.is_del = ? AND a.status = ?", enum.CommonNo, enum.CommonYes).
		Where("JSON_CONTAINS(a.scenes_json, JSON_QUOTE(?))", scene).
		Order("a.id DESC").
		Scan(&rows).Error
	if rows == nil {
		rows = []CanvasAgentOption{}
	}
	return rows, err
}
