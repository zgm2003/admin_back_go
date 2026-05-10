package aichat

import (
	"context"
	"errors"
	"strings"
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
		Select("id, role, content_type, content, meta_json, created_at").
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
		return tx.Table("ai_conversations").
			Where("id = ? AND is_del = ?", input.ConversationID, enum.CommonNo).
			Updates(map[string]any{"last_message_at": now, "updated_at": now}).Error
	})
	if err != nil {
		return 0, err
	}
	return message.ID, nil
}

func (r *GormRepository) CreateRun(ctx context.Context, input CreateRunRecord) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	startedAt := input.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	run := Run{
		ConversationID:   input.ConversationID,
		RequestID:        strings.TrimSpace(input.RequestID),
		UserMessageID:    input.UserMessageID,
		UserID:           input.UserID,
		AgentID:          input.AgentID,
		ProviderID:       input.ProviderID,
		ModelID:          strings.TrimSpace(input.ModelID),
		ModelDisplayName: strings.TrimSpace(input.ModelDisplayName),
		Status:           enum.AIRunStatusRunning,
		StartedAt:        &startedAt,
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		return tx.Create(&RunEvent{RunID: run.ID, Seq: 1, EventType: enum.AIRunEventStart, Message: enum.AIRunEventLabels[enum.AIRunEventStart]}).Error
	})
	if err != nil {
		return 0, err
	}
	return run.ID, nil
}

func (r *GormRepository) CompleteRun(ctx context.Context, input CompleteRunRecord) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.finishRun(ctx, input.RunID, enum.AIRunStatusSuccess, enum.AIRunEventCompleted, enum.AIRunEventLabels[enum.AIRunEventCompleted], input.FinishedAt, input.DurationMS, map[string]any{
		"assistant_message_id": input.AssistantMessageID,
		"prompt_tokens":        nonNegativeInt(input.PromptTokens),
		"completion_tokens":    nonNegativeInt(input.CompletionTokens),
		"total_tokens":         nonNegativeInt(input.TotalTokens),
	})
}

func (r *GormRepository) FinishRun(ctx context.Context, input FinishRunRecord) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	eventType := enum.AIRunEventFailed
	switch input.Status {
	case enum.AIRunStatusCanceled:
		eventType = enum.AIRunEventCanceled
	case enum.AIRunStatusTimeout:
		eventType = enum.AIRunEventTimeout
	case enum.AIRunStatusFailed:
		eventType = enum.AIRunEventFailed
	default:
		return errors.New("invalid AI run terminal status")
	}
	message := strings.TrimSpace(input.Message)
	if message == "" {
		message = enum.AIRunStatusLabels[input.Status]
	}
	return r.finishRun(ctx, input.RunID, input.Status, eventType, message, input.FinishedAt, input.DurationMS, nil)
}

func (r *GormRepository) finishRun(ctx context.Context, runID int64, status string, eventType string, message string, finishedAt time.Time, durationMS uint, extra map[string]any) error {
	if finishedAt.IsZero() {
		finishedAt = time.Now()
	}
	updates := map[string]any{
		"status":        status,
		"finished_at":   finishedAt,
		"duration_ms":   durationMS,
		"error_message": "",
	}
	if status != enum.AIRunStatusSuccess {
		updates["error_message"] = truncateRunMessage(message)
	}
	for key, value := range extra {
		updates[key] = value
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Run{}).Where("id = ? AND status = ?", runID, enum.AIRunStatusRunning).Updates(updates).Error; err != nil {
			return err
		}
		var maxSeq uint
		if err := tx.Model(&RunEvent{}).Where("run_id = ?", runID).Select("COALESCE(MAX(seq), 0)").Scan(&maxSeq).Error; err != nil {
			return err
		}
		return tx.Create(&RunEvent{RunID: runID, Seq: maxSeq + 1, EventType: eventType, Message: truncateRunMessage(message)}).Error
	})
}
func (r *GormRepository) TimeoutRuns(ctx context.Context, limit int, message string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if limit <= 0 {
		limit = defaultTimeoutLimit
	}
	var runs []Run
	if err := r.db.WithContext(ctx).Where("status = ?", enum.AIRunStatusRunning).Order("id ASC").Limit(limit).Find(&runs).Error; err != nil {
		return 0, err
	}
	if len(runs) == 0 {
		return 0, nil
	}
	now := time.Now()
	var changed int64
	for _, run := range runs {
		if err := r.FinishRun(ctx, FinishRunRecord{RunID: run.ID, Status: enum.AIRunStatusTimeout, Message: message, FinishedAt: now, DurationMS: durationSince(run.StartedAt, now)}); err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
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

func nonNegativeInt(value int) uint {
	if value < 0 {
		return 0
	}
	return uint(value)
}

func durationSince(startedAt *time.Time, finishedAt time.Time) uint {
	if startedAt == nil || startedAt.IsZero() || finishedAt.Before(*startedAt) {
		return 0
	}
	return uint(finishedAt.Sub(*startedAt).Milliseconds())
}

func truncateRunMessage(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 1024 {
		return string(runes[:1024])
	}
	return value
}
