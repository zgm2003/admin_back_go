package aimessage

import (
	"context"
	"errors"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aimessage repository not configured")

type GormRepository struct {
	db      *gorm.DB
	replies replycommand.Repository
}

func NewGormRepository(client *database.Client, replyOptions ...replycommand.RepositoryOption) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm, replies: replycommand.NewGormRepository(client, replyOptions...)}
}

func (r *GormRepository) CreateReply(ctx context.Context, input replycommand.CreateReplyInput) (replycommand.CreateReplyResult, error) {
	if r == nil || r.replies == nil {
		return replycommand.CreateReplyResult{}, ErrRepositoryNotConfigured
	}
	return r.replies.CreateReply(ctx, input)
}

func (r *GormRepository) RequestCancel(ctx context.Context, conversationID int64, userID int64, requestID string, now time.Time) (*replycommand.Command, error) {
	if r == nil || r.replies == nil {
		return nil, ErrRepositoryNotConfigured
	}
	return r.replies.RequestCancel(ctx, conversationID, userID, requestID, now)
}

func (r *GormRepository) Conversation(ctx context.Context, id int64) (*Conversation, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Conversation
	err := r.db.WithContext(ctx).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) AgentForConversation(ctx context.Context, conversationID int64, userID int64) (*AgentRuntime, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row AgentRuntime
	err := r.db.WithContext(ctx).Table("ai_conversations c").
		Select(`a.id AS agent_id, a.provider_id AS provider_id, a.model_id AS model_id,
			a.model_display_name AS model_display_name, e.engine_type AS engine_type,
			a.billing_multiplier_ppm AS billing_multiplier_ppm, a.max_output_tokens AS max_output_tokens,
			a.status AS status, a.scenes_json AS scenes_json`).
		Joins("JOIN ai_agents a ON a.id = c.agent_id AND a.is_del = ?", enum.CommonNo).
		Joins("JOIN ai_providers e ON e.id = a.provider_id AND e.is_del = ? AND e.status = ?", enum.CommonNo, enum.CommonYes).
		Where("c.id = ? AND c.user_id = ? AND c.is_del = ?", conversationID, userID, enum.CommonNo).
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

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]Message, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, ErrRepositoryNotConfigured
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}
	db := r.db.WithContext(ctx).Table("ai_messages m").
		Select("m.id, m.conversation_id, m.role, m.content_type, m.content, m.meta_json, m.is_del, m.created_at, m.updated_at").
		Joins("JOIN ai_conversations c ON c.id = m.conversation_id AND c.user_id = ? AND c.is_del = ?", query.UserID, enum.CommonNo).
		Where("m.conversation_id = ?", query.ConversationID).
		Where("m.is_del = ?", enum.CommonNo)
	if query.BeforeID > 0 {
		db = db.Where("m.id < ?", query.BeforeID)
	}
	var rows []Message
	err := db.Order("m.id DESC").Limit(limit + 1).Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	return rows, hasMore, nil
}
