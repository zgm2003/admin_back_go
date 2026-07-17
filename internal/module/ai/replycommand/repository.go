package replycommand

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrRepositoryNotConfigured = errors.New("reply command repository not configured")
	ErrConversationUnavailable = errors.New("owned active conversation not found")
	ErrCreateInputInvalid      = errors.New("reply command create input is invalid")
)

const defaultMaxAttempts = 3

type Repository interface {
	CreateReply(context.Context, CreateReplyInput) (CreateReplyResult, error)
}

type GormRepository struct {
	db             *gorm.DB
	now            func() time.Time
	idempotencyKey func(int64, string) string
}

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return newGormRepository(client.Gorm)
}

func newGormRepository(db *gorm.DB) *GormRepository {
	if db == nil {
		return nil
	}
	return &GormRepository{db: db, now: time.Now, idempotencyKey: idempotencyKey}
}

func (r *GormRepository) CreateReply(ctx context.Context, input CreateReplyInput) (CreateReplyResult, error) {
	if r == nil || r.db == nil {
		return CreateReplyResult{}, ErrRepositoryNotConfigured
	}
	input.RequestID = strings.TrimSpace(input.RequestID)
	if input.ConversationID <= 0 || input.UserID <= 0 || input.RequestID == "" || utf8.RuneCountInString(input.RequestID) > 64 {
		return CreateReplyResult{}, fmt.Errorf("%w: conversation_id, user_id and request_id are required and request_id is at most 64 characters", ErrCreateInputInvalid)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	if r.now != nil {
		now = r.now()
	}
	var result CreateReplyResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversation replyConversation
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND is_del = ?", input.ConversationID, input.UserID, enum.CommonNo).
			First(&conversation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrConversationUnavailable
		}
		if err != nil {
			return err
		}

		var existing Command
		err = tx.Where("conversation_id = ? AND request_id = ?", input.ConversationID, input.RequestID).
			First(&existing).Error
		if err == nil {
			result = CreateReplyResult{
				UserMessageID: existing.UserMessageID,
				CommandID:     existing.ID,
				RequestID:     existing.RequestID,
				State:         existing.State,
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		message := replyMessage{
			ConversationID: input.ConversationID,
			Role:           enum.AIMessageRoleUser,
			ContentType:    "text",
			Content:        input.Content,
			MetaJSON:       input.MetaJSON,
			IsDel:          enum.CommonNo,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}

		key := idempotencyKey(input.ConversationID, input.RequestID)
		if r.idempotencyKey != nil {
			key = r.idempotencyKey(input.ConversationID, input.RequestID)
		}
		command := Command{
			RequestID:      input.RequestID,
			IdempotencyKey: key,
			Platform:       enum.PlatformAdmin,
			UserID:         input.UserID,
			ConversationID: input.ConversationID,
			UserMessageID:  message.ID,
			State:          StatePending,
			MaxAttempts:    defaultMaxAttempts,
			NextAttemptAt:  now,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(&command).Error; err != nil {
			return err
		}

		updates := map[string]any{"last_message_at": now, "updated_at": now}
		if err := tx.Model(&replyConversation{}).
			Where("id = ? AND user_id = ? AND is_del = ?", input.ConversationID, input.UserID, enum.CommonNo).
			Updates(updates).Error; err != nil {
			return err
		}
		if title := titleFromContent(input.Content); title != "" {
			if err := tx.Model(&replyConversation{}).
				Where("id = ? AND user_id = ? AND is_del = ? AND title = ''", input.ConversationID, input.UserID, enum.CommonNo).
				Update("title", title).Error; err != nil {
				return err
			}
		}
		result = CreateReplyResult{
			UserMessageID: message.ID,
			CommandID:     command.ID,
			RequestID:     command.RequestID,
			State:         command.State,
		}
		return nil
	})
	if err != nil {
		return CreateReplyResult{}, err
	}
	return result, nil
}

func idempotencyKey(conversationID int64, requestID string) string {
	sum := sha256.Sum256([]byte("admin-reply:" + strconv.FormatInt(conversationID, 10) + ":" + strings.TrimSpace(requestID)))
	return hex.EncodeToString(sum[:])
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

type replyConversation struct {
	ID      int64  `gorm:"column:id;primaryKey"`
	UserID  int64  `gorm:"column:user_id"`
	AgentID int64  `gorm:"column:agent_id"`
	Title   string `gorm:"column:title"`
	IsDel   int    `gorm:"column:is_del"`
}

func (replyConversation) TableName() string { return "ai_conversations" }

type replyMessage struct {
	ID             int64     `gorm:"column:id;primaryKey"`
	ConversationID int64     `gorm:"column:conversation_id"`
	ReplyCommandID *uint64   `gorm:"column:reply_command_id"`
	Role           int       `gorm:"column:role"`
	ContentType    string    `gorm:"column:content_type"`
	Content        string    `gorm:"column:content"`
	MetaJSON       *string   `gorm:"column:meta_json"`
	IsDel          int       `gorm:"column:is_del"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (replyMessage) TableName() string { return "ai_messages" }
