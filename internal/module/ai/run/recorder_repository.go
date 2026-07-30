package airun

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/module/ai/requestidentity"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrTerminalRunCannotRestart = errors.New("terminal AI run cannot be restarted")

func (r *GormRepository) StartRun(ctx context.Context, input StartRecord) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	startedAt := input.StartedAt
	var idempotencyKey *string
	if key := strings.TrimSpace(input.IdempotencyKey); key != "" {
		idempotencyKey = &key
	}
	run := runFromStartRecord(input, startedAt, idempotencyKey)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if idempotencyKey != nil {
			var existing Run
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("idempotency_key = ?", *idempotencyKey).First(&existing).Error
			if err == nil {
				run.ID = existing.ID
				if existing.Status == enum.AIRunStatusRunning || existing.Status == enum.AIRunStatusSuccess {
					return nil
				}
				return ErrTerminalRunCannotRestart
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

func runFromStartRecord(input StartRecord, startedAt time.Time, idempotencyKey *string) Run {
	return Run{
		Platform:              input.Platform,
		IdempotencyKey:        idempotencyKey,
		ConversationID:        input.ConversationID,
		RequestID:             input.RequestID,
		RequestIdentityStatus: string(requestidentity.IdentityStatusReplayable),
		RequestIdentityMarker: "",
		UserMessageID:         input.UserMessageID,
		UserID:                input.UserID,
		AgentID:               input.AgentID,
		ProviderID:            input.ProviderID,
		ModelID:               input.ModelID,
		ModelDisplayName:      input.ModelDisplayName,
		InputSnapshot:         input.InputSnapshot,
		Status:                enum.AIRunStatusRunning,
		StartedAt:             &startedAt,
	}
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
			return ProjectTerminalDashboardFacts(ctx, tx, runID)
		}
		changed = true
		var maxSeq uint
		if err := tx.Model(&RunEvent{}).Where("run_id = ?", runID).Select("COALESCE(MAX(seq), 0)").Scan(&maxSeq).Error; err != nil {
			return err
		}
		if err := tx.Create(&RunEvent{RunID: runID, Seq: maxSeq + 1, EventType: eventType, Message: truncateRecorderRunMessage(message)}).Error; err != nil {
			return err
		}
		return ProjectTerminalDashboardFacts(ctx, tx, runID)
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
