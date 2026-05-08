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

func (r *GormRepository) ActiveAgentExists(ctx context.Context, id int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	var count int64
	err := r.db.WithContext(ctx).Table("ai_agents").Where("id = ?", id).Where("status = ?", enum.CommonYes).Where("is_del = ?", enum.CommonNo).Count(&count).Error
	return count > 0, err
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

func (r *GormRepository) CreateRun(ctx context.Context, input CreateRunRecord) (*RunStartRecord, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	record := &RunStartRecord{RequestID: input.RequestID, AgentID: input.AgentID, IsNew: input.ConversationID == 0}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		conversationID := input.ConversationID
		if conversationID == 0 {
			conversation := Conversation{UserID: input.UserID, AgentID: input.AgentID, Title: titleFromContent(input.Content), LastMessageAt: &now, Status: enum.CommonYes, IsDel: enum.CommonNo}
			if err := tx.Create(&conversation).Error; err != nil {
				return err
			}
			conversationID = conversation.ID
		}
		message := Message{ConversationID: conversationID, Role: enum.AIMessageRoleUser, Content: input.Content, MetaJSON: input.MetaJSON, IsDel: enum.CommonNo}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		run := Run{RequestID: input.RequestID, UserID: input.UserID, AgentID: input.AgentID, ConversationID: conversationID, UserMessageID: &message.ID, RunStatus: enum.AIRunStatusRunning, MetaJSON: input.MetaJSON, IsDel: enum.CommonNo}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		if err := tx.Model(&Conversation{}).Where("id = ?", conversationID).Updates(map[string]any{"last_message_at": now}).Error; err != nil {
			return err
		}
		record.RunID = run.ID
		record.ConversationID = conversationID
		record.UserMessageID = message.ID
		return nil
	})
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (r *GormRepository) RunForUser(ctx context.Context, runID int64, userID int64) (*Run, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Run
	err := r.db.WithContext(ctx).Where("id = ?", runID).Where("is_del = ?", enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) RunForExecute(ctx context.Context, runID int64) (*RunExecutionRecord, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var run Run
	err := r.db.WithContext(ctx).Where("id = ?", runID).Where("is_del = ?", enum.CommonNo).First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	content := ""
	if run.UserMessageID != nil {
		var msg Message
		if err := r.db.WithContext(ctx).Where("id = ?", *run.UserMessageID).Where("is_del = ?", enum.CommonNo).First(&msg).Error; err == nil {
			content = msg.Content
		}
	}
	return &RunExecutionRecord{Run: run, UserMessageContent: content}, nil
}

func (r *GormRepository) AssistantMessage(ctx context.Context, id int64) (*Message, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Message
	err := r.db.WithContext(ctx).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) MarkCanceled(ctx context.Context, runID int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Run{}).Where("id = ?", runID).Where("run_status = ?", enum.AIRunStatusRunning).Updates(map[string]any{"run_status": enum.AIRunStatusCanceled}).Error
}

func (r *GormRepository) MarkSuccess(ctx context.Context, input RunSuccessRecord) (*Message, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var message Message
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		message = Message{ConversationID: input.ConversationID, Role: enum.AIMessageRoleAssistant, Content: input.Content, IsDel: enum.CommonNo}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		total := input.PromptTokens + input.CompletionTokens
		return tx.Model(&Run{}).Where("id = ?", input.RunID).Updates(map[string]any{
			"assistant_message_id": message.ID,
			"run_status":           enum.AIRunStatusSuccess,
			"model_snapshot":       input.ModelSnapshot,
			"prompt_tokens":        input.PromptTokens,
			"completion_tokens":    input.CompletionTokens,
			"total_tokens":         total,
			"latency_ms":           input.LatencyMS,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return &message, nil
}

func (r *GormRepository) MarkFailed(ctx context.Context, runID int64, message string) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Run{}).Where("id = ?", runID).Where("is_del = ?", enum.CommonNo).Updates(map[string]any{"run_status": enum.AIRunStatusFail, "error_msg": message}).Error
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
	res := r.db.WithContext(ctx).Model(&Run{}).Where("id IN ?", ids).Updates(map[string]any{"run_status": enum.AIRunStatusFail, "error_msg": message})
	return res.RowsAffected, res.Error
}

func titleFromContent(content string) string {
	runes := []rune(content)
	if len(runes) > 30 {
		runes = runes[:30]
	}
	if len(runes) == 0 {
		return "新会话"
	}
	return string(runes)
}
