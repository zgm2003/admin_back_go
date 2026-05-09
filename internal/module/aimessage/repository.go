package aimessage

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aimessage repository not configured")

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
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
		Select("a.id AS agent_id, a.status AS status, a.scenes_json AS scenes_json").
		Joins("JOIN ai_agents a ON a.id = c.agent_id AND a.is_del = ?", enum.CommonNo).
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
		Select("m.id, m.conversation_id, m.role, m.content_type, m.content, m.is_del, m.created_at, m.updated_at").
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

func (r *GormRepository) InsertUserMessage(ctx context.Context, input SendRecord) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	now := time.Now()
	message := Message{
		ConversationID: input.ConversationID,
		Role:           input.Role,
		ContentType:    input.ContentType,
		Content:        input.Content,
		IsDel:          enum.CommonNo,
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		if err := tx.Table("ai_conversations").Where("id = ? AND is_del = ?", input.ConversationID, enum.CommonNo).Update("last_message_at", now).Error; err != nil {
			return err
		}
		title := titleFromContent(input.Content)
		if title != "" {
			if err := tx.Table("ai_conversations").Where("id = ? AND is_del = ? AND title = ''", input.ConversationID, enum.CommonNo).Update("title", title).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return message.ID, nil
}

func titleFromContent(content string) string {
	content = strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if content == "" {
		return ""
	}
	runes := []rune(content)
	if len(runes) > 30 {
		return string(runes[:30])
	}
	return content
}
