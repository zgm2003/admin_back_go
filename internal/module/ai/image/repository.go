package aiimage

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aiimage repository not configured")

type Repository interface {
	ListImageAgents(ctx context.Context, scene string) ([]AgentOption, error)
	ListTasks(ctx context.Context, userID uint64, query ListQuery) ([]ImageTask, int64, error)
	GetTask(ctx context.Context, userID uint64, taskID uint64, platform string) (*ImageTask, error)
	GetTaskForWorker(ctx context.Context, userID uint64, taskID uint64) (*ImageTask, error)
	LoadTaskFiles(ctx context.Context, taskID uint64) ([]ImageFile, error)
	CreateTaskWithFiles(ctx context.Context, task ImageTask, files TaskFileSet) (uint64, error)
	DeleteTask(ctx context.Context, userID uint64, taskID uint64, platform string) error
	LoadAgentRuntime(ctx context.Context, agentID uint64) (*AgentRuntime, error)
	ClaimTask(ctx context.Context, userID uint64, taskID uint64, startedAt time.Time) (bool, error)
	AppendTaskFiles(ctx context.Context, files []ImageFile) error
	FinishTaskSuccess(ctx context.Context, userID uint64, taskID uint64, actualParamsJSON *string, rawResponseJSON *string, elapsedMS int, finishedAt time.Time) error
	FinishTaskFailed(ctx context.Context, userID uint64, taskID uint64, message string, elapsedMS int, finishedAt time.Time) error
	LoadUploadConfig(ctx context.Context) (*UploadConfig, error)
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) ListImageAgents(ctx context.Context, scene string) ([]AgentOption, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	scene = strings.TrimSpace(scene)
	if scene == "" {
		return nil, errors.New("image agent scene is required")
	}
	var rows []AgentOption
	err := r.db.WithContext(ctx).Table("ai_agents AS a").
		Select("a.id AS id, a.name AS name, a.avatar AS avatar").
		Joins("JOIN ai_providers AS p ON p.id = a.provider_id AND p.is_del = ? AND p.status = ?", enum.CommonNo, enum.CommonYes).
		Joins("JOIN ai_provider_models AS m ON m.provider_id = a.provider_id AND m.model_id = a.model_id AND m.status = ?", enum.CommonYes).
		Where("a.is_del = ? AND a.status = ?", enum.CommonNo, enum.CommonYes).
		Where("JSON_CONTAINS(a.scenes_json, JSON_QUOTE(?))", scene).
		Order("a.id DESC").
		Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) ListTasks(ctx context.Context, userID uint64, query ListQuery) ([]ImageTask, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.tasks(ctx).Where("user_id = ?", userID)
	if strings.TrimSpace(query.Platform) != "" {
		db = db.Where("platform = ?", strings.TrimSpace(query.Platform))
	}
	if strings.TrimSpace(query.Status) != "" {
		db = db.Where("status = ?", strings.TrimSpace(query.Status))
	}
	var total int64
	if err := db.Model(&ImageTask{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ImageTask
	err := db.Order("id DESC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) GetTask(ctx context.Context, userID uint64, taskID uint64, platform string) (*ImageTask, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if userID == 0 || taskID == 0 {
		return nil, nil
	}
	var row ImageTask
	err := r.userTask(ctx, userID, taskID, platform).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) GetTaskForWorker(ctx context.Context, userID uint64, taskID uint64) (*ImageTask, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if userID == 0 || taskID == 0 {
		return nil, nil
	}
	var row ImageTask
	err := r.userTask(ctx, userID, taskID, "").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) LoadTaskFiles(ctx context.Context, taskID uint64) ([]ImageFile, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []ImageFile
	err := r.db.WithContext(ctx).Where("task_id = ?", taskID).Order("role ASC, sort_order ASC, id ASC").Find(&rows).Error
	if rows == nil {
		rows = []ImageFile{}
	}
	return rows, err
}

func (r *GormRepository) CreateTaskWithFiles(ctx context.Context, task ImageTask, files TaskFileSet) (uint64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		inputIDsBySort := make(map[int]uint64, len(files.Inputs))
		for i := range files.Inputs {
			files.Inputs[i].TaskID = task.ID
			if err := tx.Create(&files.Inputs[i]).Error; err != nil {
				return err
			}
			inputIDsBySort[files.Inputs[i].SortOrder] = files.Inputs[i].ID
		}
		if files.Mask != nil {
			mask := files.Mask.File
			mask.TaskID = task.ID
			if relatedID, ok := inputIDsBySort[files.Mask.RelatedSortOrder]; ok {
				mask.RelatedFileID = &relatedID
			}
			if err := tx.Create(&mask).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return task.ID, err
}

func (r *GormRepository) DeleteTask(ctx context.Context, userID uint64, taskID uint64, platform string) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	tx := r.userTask(ctx, userID, taskID, platform).Delete(&ImageTask{})
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *GormRepository) LoadAgentRuntime(ctx context.Context, agentID uint64) (*AgentRuntime, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if agentID == 0 {
		return nil, nil
	}
	var row AgentRuntime
	err := r.db.WithContext(ctx).Table("ai_agents AS a").
		Select(`a.id AS agent_id,
			a.name AS agent_name,
			a.scenes_json AS scenes_json,
			a.status AS agent_status,
			a.provider_id AS provider_id,
			p.name AS provider_name,
			p.engine_type AS engine_type,
			p.base_url AS base_url,
			p.api_key_enc AS api_key_enc,
			p.status AS provider_status,
			a.model_id AS model_id,
			COALESCE(NULLIF(m.display_name, ''), a.model_display_name) AS model_display_name,
			m.status AS model_status`).
		Joins("JOIN ai_providers AS p ON p.id = a.provider_id AND p.is_del = ?", enum.CommonNo).
		Joins("JOIN ai_provider_models AS m ON m.provider_id = a.provider_id AND m.model_id = a.model_id").
		Where("a.id = ? AND a.is_del = ?", agentID, enum.CommonNo).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) ClaimTask(ctx context.Context, userID uint64, taskID uint64, startedAt time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	tx := r.tasks(ctx).
		Where("user_id = ? AND id = ? AND status = ?", userID, taskID, StatusPending).
		Updates(map[string]any{"status": StatusRunning, "updated_at": startedAt})
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected > 0, nil
}

func (r *GormRepository) AppendTaskFiles(ctx context.Context, files []ImageFile) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if len(files) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&files).Error
}

func (r *GormRepository) FinishTaskSuccess(ctx context.Context, userID uint64, taskID uint64, actualParamsJSON *string, rawResponseJSON *string, elapsedMS int, finishedAt time.Time) error {
	return r.finishTask(ctx, userID, taskID, map[string]any{
		"status":             StatusSuccess,
		"error_message":      "",
		"actual_params_json": actualParamsJSON,
		"raw_response_json":  rawResponseJSON,
		"elapsed_ms":         elapsedMS,
		"finished_at":        finishedAt,
		"updated_at":         finishedAt,
	})
}

func (r *GormRepository) FinishTaskFailed(ctx context.Context, userID uint64, taskID uint64, message string, elapsedMS int, finishedAt time.Time) error {
	return r.finishTask(ctx, userID, taskID, map[string]any{
		"status":        StatusFailed,
		"error_message": message,
		"elapsed_ms":    elapsedMS,
		"finished_at":   finishedAt,
		"updated_at":    finishedAt,
	})
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

func (r *GormRepository) tasks(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Model(&ImageTask{})
}

func (r *GormRepository) userTask(ctx context.Context, userID uint64, taskID uint64, platform string) *gorm.DB {
	tx := r.tasks(ctx).Where("user_id = ? AND id = ?", userID, taskID)
	if strings.TrimSpace(platform) != "" {
		tx = tx.Where("platform = ?", strings.TrimSpace(platform))
	}
	return tx
}

func (r *GormRepository) finishTask(ctx context.Context, userID uint64, taskID uint64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	tx := r.tasks(ctx).Where("user_id = ? AND id = ?", userID, taskID).Updates(fields)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
