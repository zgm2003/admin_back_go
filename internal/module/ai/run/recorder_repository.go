package airun

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *GormRepository) StartRun(ctx context.Context, input StartRecord) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	startedAt := input.StartedAt
	var idempotencyKey *string
	if key := strings.TrimSpace(input.IdempotencyKey); key != "" {
		idempotencyKey = &key
	}
	run := Run{
		Platform:         input.Platform,
		IdempotencyKey:   idempotencyKey,
		ConversationID:   input.ConversationID,
		RequestID:        input.RequestID,
		UserMessageID:    input.UserMessageID,
		UserID:           input.UserID,
		AgentID:          input.AgentID,
		ProviderID:       input.ProviderID,
		ModelID:          input.ModelID,
		ModelDisplayName: input.ModelDisplayName,
		InputSnapshot:    input.InputSnapshot,
		Status:           enum.AIRunStatusRunning,
		StartedAt:        &startedAt,
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if idempotencyKey != nil {
			var existing Run
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("idempotency_key = ?", *idempotencyKey).First(&existing).Error
			if err == nil {
				run.ID = existing.ID
				if existing.Status == enum.AIRunStatusRunning || existing.Status == enum.AIRunStatusSuccess {
					return nil
				}
				updates := map[string]any{
					"status":               enum.AIRunStatusRunning,
					"assistant_message_id": nil,
					"prompt_tokens":        0,
					"completion_tokens":    0,
					"total_tokens":         0,
					"duration_ms":          nil,
					"error_message":        "",
					"started_at":           startedAt,
					"finished_at":          nil,
				}
				if err := tx.Model(&Run{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
					return err
				}
				var maxSeq uint
				if err := tx.Model(&RunEvent{}).Where("run_id = ?", existing.ID).Select("COALESCE(MAX(seq), 0)").Scan(&maxSeq).Error; err != nil {
					return err
				}
				return tx.Create(&RunEvent{RunID: existing.ID, Seq: maxSeq + 1, EventType: enum.AIRunEventStart, Message: enum.AIRunEventLabels[enum.AIRunEventStart]}).Error
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
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

func (r *GormRepository) CompleteRun(ctx context.Context, input CompleteRecord) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	extra := map[string]any{
		"prompt_tokens":     input.PromptTokens,
		"completion_tokens": input.CompletionTokens,
		"total_tokens":      input.TotalTokens,
	}
	if input.AssistantMessageID != nil {
		extra["assistant_message_id"] = *input.AssistantMessageID
	}
	_, err := r.finishRecorderRun(ctx, input.RunID, enum.AIRunStatusSuccess, enum.AIRunEventCompleted, enum.AIRunEventLabels[enum.AIRunEventCompleted], input.FinishedAt, input.DurationMS, extra)
	return err
}

func (r *GormRepository) FinishRun(ctx context.Context, input FinishRecord) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	_, err := r.finishRecorderRun(ctx, input.RunID, input.Status, input.EventType, input.Message, input.FinishedAt, input.DurationMS, nil)
	return err
}

func (r *GormRepository) finishRecorderRun(ctx context.Context, runID int64, status string, eventType string, message string, finishedAt time.Time, durationMS uint, extra map[string]any) (bool, error) {
	updates := map[string]any{
		"status":        status,
		"finished_at":   finishedAt,
		"duration_ms":   durationMS,
		"error_message": "",
	}
	if status != enum.AIRunStatusSuccess {
		updates["error_message"] = truncateRecorderRunMessage(message)
	}
	for key, value := range extra {
		updates[key] = value
	}
	changed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Run{}).Where("id = ? AND status = ?", runID, enum.AIRunStatusRunning).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		changed = true
		var maxSeq uint
		if err := tx.Model(&RunEvent{}).Where("run_id = ?", runID).Select("COALESCE(MAX(seq), 0)").Scan(&maxSeq).Error; err != nil {
			return err
		}
		return tx.Create(&RunEvent{RunID: runID, Seq: maxSeq + 1, EventType: eventType, Message: truncateRecorderRunMessage(message)}).Error
	})
	return changed, err
}

func truncateRecorderRunMessage(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 1024 {
		return value
	}
	return value[:1024]
}
