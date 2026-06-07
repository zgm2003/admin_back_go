package airun

import (
	"context"
	"strings"
	"time"

	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

func (r *GormRepository) StartRun(ctx context.Context, input StartRecord) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	startedAt := input.StartedAt
	run := Run{
		Platform:         input.Platform,
		Modality:         input.Modality,
		SourceType:       input.SourceType,
		SourceID:         input.SourceID,
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
		UsageStatus:      enum.AIRunUsagePending,
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

func (r *GormRepository) CompleteRun(ctx context.Context, input CompleteRecord) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	extra := map[string]any{
		"prompt_tokens":     input.PromptTokens,
		"completion_tokens": input.CompletionTokens,
		"total_tokens":      input.TotalTokens,
		"usage_status":      input.UsageStatus,
	}
	if input.AssistantMessageID != nil {
		extra["assistant_message_id"] = *input.AssistantMessageID
	}
	_, err := r.finishRecorderRun(ctx, input.RunID, enum.AIRunStatusSuccess, enum.AIRunEventCompleted, enum.AIRunEventLabels[enum.AIRunEventCompleted], input.FinishedAt, input.DurationMS, extra)
	return err
}

func (r *GormRepository) CompleteRunBySource(ctx context.Context, input CompleteSourceRecord) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	extra := map[string]any{
		"prompt_tokens":     input.PromptTokens,
		"completion_tokens": input.CompletionTokens,
		"total_tokens":      input.TotalTokens,
		"usage_status":      input.UsageStatus,
	}
	if input.AssistantMessageID != nil {
		extra["assistant_message_id"] = *input.AssistantMessageID
	}
	_, err := r.finishRecorderRunBySource(ctx, input.SourceType, input.SourceID, enum.AIRunStatusSuccess, enum.AIRunEventCompleted, enum.AIRunEventLabels[enum.AIRunEventCompleted], input.FinishedAt, input.DurationMS, extra)
	return err
}

func (r *GormRepository) FinishRun(ctx context.Context, input FinishRecord) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	extra := map[string]any{"usage_status": input.UsageStatus}
	_, err := r.finishRecorderRun(ctx, input.RunID, input.Status, input.EventType, input.Message, input.FinishedAt, input.DurationMS, extra)
	return err
}

func (r *GormRepository) FinishRunBySource(ctx context.Context, input FinishSourceRecord) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	extra := map[string]any{"usage_status": input.UsageStatus}
	_, err := r.finishRecorderRunBySource(ctx, input.SourceType, input.SourceID, input.Status, input.EventType, input.Message, input.FinishedAt, input.DurationMS, extra)
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

func (r *GormRepository) finishRecorderRunBySource(ctx context.Context, sourceType string, sourceID uint64, status string, eventType string, message string, finishedAt time.Time, durationMS uint, extra map[string]any) (bool, error) {
	var runID int64
	if err := r.db.WithContext(ctx).Model(&Run{}).Where("source_type = ? AND source_id = ? AND status = ?", sourceType, sourceID, enum.AIRunStatusRunning).Order("id DESC").Limit(1).Pluck("id", &runID).Error; err != nil {
		return false, err
	}
	if runID == 0 {
		return false, nil
	}
	return r.finishRecorderRun(ctx, runID, status, eventType, message, finishedAt, durationMS, extra)
}

func truncateRecorderRunMessage(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 1024 {
		return value
	}
	return value[:1024]
}
