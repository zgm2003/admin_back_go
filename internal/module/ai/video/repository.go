package aivideo

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aivideo repository not configured")

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

func (r *GormRepository) CreateTask(ctx context.Context, task VideoTask) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&task).Error; err != nil {
		return 0, err
	}
	return task.ID, nil
}

func (r *GormRepository) UpdateTask(ctx context.Context, platform string, userID int64, id int64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if strings.TrimSpace(platform) == "" || userID <= 0 || id <= 0 {
		return gorm.ErrRecordNotFound
	}
	tx := r.db.WithContext(ctx).Model(&VideoTask{}).
		Where("platform = ? AND user_id = ? AND id = ? AND is_del = ?", strings.TrimSpace(platform), userID, id, IsDelActive).
		Updates(fields)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *GormRepository) GetTask(ctx context.Context, platform string, userID int64, id int64) (*VideoTask, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if strings.TrimSpace(platform) == "" || userID <= 0 || id <= 0 {
		return nil, nil
	}
	var row VideoTask
	err := r.db.WithContext(ctx).
		Where("platform = ? AND user_id = ? AND id = ? AND is_del = ?", strings.TrimSpace(platform), userID, id, IsDelActive).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) LoadUploadConfig(ctx context.Context) (*UploadConfig, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row UploadConfig
	err := r.db.WithContext(ctx).
		Table("upload_setting AS s").
		Select(`s.id AS setting_id,
			d.driver, d.secret_id_enc, d.secret_key_enc, d.bucket, d.region, d.appid, d.endpoint, d.bucket_domain`).
		Joins("JOIN upload_driver AS d ON d.id = s.driver_id AND d.is_del = ?", enum.CommonNo).
		Joins("JOIN upload_rule AS rule ON rule.id = s.rule_id AND rule.is_del = ?", enum.CommonNo).
		Where("s.status = ?", enum.CommonYes).
		Where("s.is_del = ?", enum.CommonNo).
		Order("s.id DESC").
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.SettingID == 0 {
		return nil, nil
	}
	return &row, nil
}
