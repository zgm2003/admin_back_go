package aichat

import (
	"context"
	"errors"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aichat repository not configured")

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) ConversationForReply(ctx context.Context, id int64, userID int64) (*Conversation, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Conversation
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND is_del = ?", id, userID, enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) AgentForRuntime(ctx context.Context, agentID uint64) (*AgentEngineConfig, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if agentID == 0 {
		return nil, nil
	}
	var row AgentEngineConfig
	err := r.agentRuntimeDB(ctx).Where("a.id = ?", agentID).Limit(1).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.AgentID == 0 {
		return nil, nil
	}
	return &row, nil
}

func (r *GormRepository) LatestMessages(ctx context.Context, conversationID int64, limit int) ([]MessageHistory, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if limit <= 0 {
		limit = 20
	}
	var rows []MessageHistory
	err := r.db.WithContext(ctx).Table("ai_messages").
		Select("id, role, content_type, content, created_at").
		Where("conversation_id = ? AND is_del = ?", conversationID, enum.CommonNo).
		Order("id DESC").
		Limit(limit).
		Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) InsertAssistantMessage(ctx context.Context, input AssistantMessageRecord) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	message := Message{ConversationID: input.ConversationID, Role: enum.AIMessageRoleAssistant, ContentType: "text", Content: input.Content, IsDel: enum.CommonNo}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		return tx.Table("ai_conversations").Where("id = ? AND is_del = ?", input.ConversationID, enum.CommonNo).Update("last_message_at", now).Error
	})
	if err != nil {
		return 0, err
	}
	return message.ID, nil
}

func (r *GormRepository) TimeoutRuns(ctx context.Context, limit int, message string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if limit <= 0 {
		limit = defaultTimeoutLimit
	}
	var ids []int64
	if err := r.db.WithContext(ctx).Model(&Run{}).Where("is_del = ?", enum.CommonNo).Where("run_status = ?", enum.AIRunStatusRunning).Order("id ASC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&Run{}).Where("id IN ?", ids).Updates(map[string]any{"run_status": enum.AIRunStatusFail, "error_msg": message, "completed_at": now})
	return res.RowsAffected, res.Error
}

func (r *GormRepository) agentRuntimeDB(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Table("ai_agents AS a").
		Select(`a.id AS agent_id,
			a.name AS agent_name,
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
		Where("a.is_del = ? AND a.status = ?", enum.CommonNo, enum.CommonYes)
}
