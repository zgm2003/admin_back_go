package aichat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	if id <= 0 {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Table("ai_agents").
		Where("id = ?", id).
		Where("status = ?", enum.CommonYes).
		Where("is_del = ?", enum.CommonNo).
		Count(&count).Error
	return count > 0, err
}

func (r *GormRepository) DefaultActiveAgent(ctx context.Context) (*AgentEngineConfig, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row AgentEngineConfig
	err := r.agentRuntimeDB(ctx).Order("a.id DESC").Limit(1).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.AgentID == 0 {
		return nil, nil
	}
	return &row, nil
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
	agentID := uint64(input.AgentID)
	record := &RunStartRecord{RequestID: input.RequestID, AgentID: input.AgentID, IsNew: input.ConversationID == 0}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		conversationID := input.ConversationID
		if conversationID == 0 {
			conversation := Conversation{
				UserID:        input.UserID,
				AgentID:       agentID,
				Title:         titleFromContent(input.Content),
				LastMessageAt: &now,
				Status:        enum.CommonYes,
				IsDel:         enum.CommonNo,
			}
			if err := tx.Create(&conversation).Error; err != nil {
				return err
			}
			conversationID = conversation.ID
		}
		message := Message{
			ConversationID: conversationID,
			UserID:         input.UserID,
			Role:           enum.AIMessageRoleUser,
			ContentType:    "text",
			Content:        input.Content,
			MetaJSON:       input.MetaJSON,
			Status:         enum.CommonYes,
			IsDel:          enum.CommonNo,
		}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		run := Run{
			RunUID:         uuid.NewString(),
			RequestID:      input.RequestID,
			UserID:         input.UserID,
			AgentID:        agentID,
			ConversationID: conversationID,
			UserMessageID:  &message.ID,
			RunStatus:      enum.AIRunStatusRunning,
			MetaJSON:       input.MetaJSON,
			StartedAt:      &now,
			IsDel:          enum.CommonNo,
		}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		if err := tx.Model(&Message{}).Where("id = ?", message.ID).Update("run_id", run.ID).Error; err != nil {
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
	err := r.db.WithContext(ctx).Where("id = ?", runID).Where("user_id = ?", userID).Where("is_del = ?", enum.CommonNo).First(&row).Error
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
	agent, err := r.AgentForRuntime(ctx, run.AgentID)
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return &RunExecutionRecord{Run: run, UserMessageContent: content}, nil
	}
	var conversation Conversation
	if err := r.db.WithContext(ctx).Where("id = ?", run.ConversationID).First(&conversation).Error; err == nil {
		agent.ConversationEngineID = conversation.EngineConversationID
	}
	return &RunExecutionRecord{Run: run, UserMessageContent: content, Agent: *agent}, nil
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

func (r *GormRepository) MarkCanceled(ctx context.Context, runID int64, message string) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	now := time.Now()
	fields := map[string]any{"run_status": enum.AIRunStatusCanceled, "canceled_at": now, "completed_at": now}
	if strings.TrimSpace(message) != "" {
		fields["error_msg"] = strings.TrimSpace(message)
	}
	return r.db.WithContext(ctx).Model(&Run{}).Where("id = ?", runID).Where("run_status = ?", enum.AIRunStatusRunning).Updates(fields).Error
}

func (r *GormRepository) MarkSuccess(ctx context.Context, input RunSuccessRecord) (*Message, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	now := time.Now()
	var message Message
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		message = Message{
			ConversationID:  input.ConversationID,
			RunID:           &input.RunID,
			Role:            enum.AIMessageRoleAssistant,
			ContentType:     "text",
			Content:         input.Content,
			EngineMessageID: input.EngineMessageID,
			TokenInput:      input.PromptTokens,
			TokenOutput:     input.CompletionTokens,
			Status:          enum.CommonYes,
			IsDel:           enum.CommonNo,
		}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		total := input.TotalTokens
		if total == 0 {
			total = input.PromptTokens + input.CompletionTokens
		}
		fields := map[string]any{
			"assistant_message_id": message.ID,
			"run_status":           enum.AIRunStatusSuccess,
			"engine_task_id":       input.EngineTaskID,
			"engine_run_id":        input.EngineRunID,
			"model_snapshot":       input.ModelSnapshot,
			"prompt_tokens":        input.PromptTokens,
			"completion_tokens":    input.CompletionTokens,
			"total_tokens":         total,
			"cost":                 input.Cost,
			"latency_ms":           input.LatencyMS,
			"completed_at":         now,
		}
		if strings.TrimSpace(input.UsageJSON) != "" {
			fields["usage_json"] = input.UsageJSON
		}
		if strings.TrimSpace(input.OutputSnapshotJSON) != "" {
			fields["output_snapshot_json"] = input.OutputSnapshotJSON
		}
		if err := tx.Model(&Run{}).Where("id = ?", input.RunID).Updates(fields).Error; err != nil {
			return err
		}
		if strings.TrimSpace(input.EngineConversationID) != "" {
			if err := tx.Model(&Conversation{}).Where("id = ?", input.ConversationID).Update("engine_conversation_id", strings.TrimSpace(input.EngineConversationID)).Error; err != nil {
				return err
			}
		}
		return r.upsertUsageDaily(tx, input.RunID, false)
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
	now := time.Now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Run{}).Where("id = ?", runID).Where("is_del = ?", enum.CommonNo).Updates(map[string]any{"run_status": enum.AIRunStatusFail, "error_msg": message, "completed_at": now}).Error; err != nil {
			return err
		}
		return r.upsertUsageDaily(tx, runID, true)
	})
}

func (r *GormRepository) AppendRunEvent(ctx context.Context, input RunEventRecord) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	payload := strings.TrimSpace(string(input.PayloadJSON))
	if payload == "" {
		payload = "{}"
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	return r.db.WithContext(ctx).Create(&RunEvent{
		RunID:       input.RunID,
		Seq:         input.Seq,
		EventID:     input.EventID,
		EventType:   input.EventType,
		DeltaText:   input.DeltaText,
		PayloadJSON: payload,
		CreatedAt:   createdAt,
	}).Error
}

func (r *GormRepository) ListRunEvents(ctx context.Context, runID int64) ([]RunEventRecord, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []RunEvent
	if err := r.db.WithContext(ctx).Where("run_id = ?", runID).Order("seq ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]RunEventRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, RunEventRecord{
			RunID:       row.RunID,
			Seq:         row.Seq,
			EventID:     row.EventID,
			EventType:   row.EventType,
			DeltaText:   row.DeltaText,
			PayloadJSON: json.RawMessage(nonEmptyJSON(row.PayloadJSON)),
			CreatedAt:   row.CreatedAt,
		})
	}
	return out, nil
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
			a.agent_type AS agent_type,
			a.provider_id AS provider_id,
			a.external_agent_id AS external_agent_id,
			a.external_agent_api_key_enc AS external_agent_api_key_enc,
			a.runtime_config_json AS runtime_config_json,
			a.model_snapshot_json AS model_snapshot_json,
			a.status AS agent_status,
			e.engine_type AS engine_type,
			e.base_url AS engine_base_url,
			e.api_key_enc AS engine_api_key_enc,
			e.status AS engine_status`).
		Joins("JOIN ai_providers e ON e.id = a.provider_id AND e.is_del = ? AND e.status = ?", enum.CommonNo, enum.CommonYes).
		Where("a.is_del = ? AND a.status = ?", enum.CommonNo, enum.CommonYes)
}

func (r *GormRepository) upsertUsageDaily(tx *gorm.DB, runID int64, failed bool) error {
	var run Run
	if err := tx.Where("id = ?", runID).First(&run).Error; err != nil {
		return err
	}
	day := time.Now()
	if run.CompletedAt != nil {
		day = *run.CompletedAt
	} else if !run.CreatedAt.IsZero() {
		day = run.CreatedAt
	}
	usageDate := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	failCount := 0
	if failed || run.RunStatus == enum.AIRunStatusFail || run.RunStatus == enum.AIRunStatusCanceled {
		failCount = 1
	}
	row := map[string]any{
		"usage_date":        usageDate,
		"agent_id":          run.AgentID,
		"provider_id":       run.ProviderID,
		"user_id":           run.UserID,
		"run_count":         1,
		"fail_count":        failCount,
		"prompt_tokens":     run.PromptTokens,
		"completion_tokens": run.CompletionTokens,
		"total_tokens":      run.TotalTokens,
		"cost":              run.Cost,
		"latency_ms_total":  run.LatencyMS,
	}
	return tx.Table("ai_usage_daily").Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "usage_date"}, {Name: "agent_id"}, {Name: "provider_id"}, {Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"run_count":         gorm.Expr("run_count + VALUES(run_count)"),
			"fail_count":        gorm.Expr("fail_count + VALUES(fail_count)"),
			"prompt_tokens":     gorm.Expr("prompt_tokens + VALUES(prompt_tokens)"),
			"completion_tokens": gorm.Expr("completion_tokens + VALUES(completion_tokens)"),
			"total_tokens":      gorm.Expr("total_tokens + VALUES(total_tokens)"),
			"cost":              gorm.Expr("cost + VALUES(cost)"),
			"latency_ms_total":  gorm.Expr("latency_ms_total + VALUES(latency_ms_total)"),
			"updated_at":        gorm.Expr("NOW()"),
		}),
	}).Create(row).Error
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

func nonEmptyJSON(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "{}"
	}
	return value
}
