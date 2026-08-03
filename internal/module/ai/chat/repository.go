package aichat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/module/ai/officialmodel"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/module/ai/requestidentity"
	airun "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

var (
	ErrRepositoryNotConfigured   = errors.New("aichat repository not configured")
	ErrInvalidRunBillingIdentity = errors.New("invalid new AI run billing identity")
)

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

func (r *GormRepository) AcceptedRunForReply(ctx context.Context, userID int64, requestID string) (*airun.Run, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	requestID = strings.TrimSpace(requestID)
	if userID <= 0 || requestID == "" {
		return nil, ErrInvalidRunBillingIdentity
	}
	var row airun.Run
	err := r.db.WithContext(ctx).Where("user_id = ? AND request_id = ?", userID, requestID).First(&row).Error
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

func (r *GormRepository) ProviderForPreparedRecovery(ctx context.Context, providerID uint64) (*AgentEngineConfig, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if providerID == 0 {
		return nil, nil
	}
	var row AgentEngineConfig
	err := r.db.WithContext(ctx).Table("ai_providers AS e").
		Select(`e.id AS provider_id,
			e.engine_type AS engine_type,
			e.api_protocol AS api_protocol,
			e.base_url AS engine_base_url,
			e.api_key_enc AS engine_api_key_enc,
			e.status AS engine_status`).
		Where("e.id = ?", providerID).
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ProviderID == 0 {
		return nil, nil
	}
	return &row, nil
}

func (r *GormRepository) MessageForReply(ctx context.Context, conversationID int64, messageID int64) (*MessageHistory, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if conversationID <= 0 || messageID <= 0 {
		return nil, nil
	}
	var row MessageHistory
	result := r.db.WithContext(ctx).Table("ai_messages").
		Select("id, role, content_type, content, meta_json, created_at").
		Where("id = ? AND conversation_id = ? AND is_del = ?", messageID, conversationID, enum.CommonNo).
		Limit(1).
		Scan(&row)
	if result.Error != nil {
		return nil, result.Error
	}
	if row.ID == 0 {
		return nil, nil
	}
	return &row, nil
}

func (r *GormRepository) CreateRun(ctx context.Context, input CreateRunRecord) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	startedAt := input.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	run, err := runFromCreateRecord(input, startedAt)
	if err != nil {
		return 0, err
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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

func runFromCreateRecord(input CreateRunRecord, startedAt time.Time) (Run, error) {
	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		return Run{}, fmt.Errorf("%w: request id", ErrInvalidRunBillingIdentity)
	}
	identityStatus := strings.TrimSpace(input.RequestIdentityStatus)
	identityMarker := strings.TrimSpace(input.RequestIdentityMarker)
	if identityStatus == "" {
		identityStatus = string(requestidentity.IdentityStatusReplayable)
	}
	if identityStatus != string(requestidentity.IdentityStatusReplayable) {
		return Run{}, fmt.Errorf("%w: request identity status", ErrInvalidRunBillingIdentity)
	}
	if identityMarker != "" {
		return Run{}, fmt.Errorf("%w: request identity marker", ErrInvalidRunBillingIdentity)
	}
	if input.RequestFingerprint == ([32]byte{}) {
		return Run{}, fmt.Errorf("%w: request fingerprint", ErrInvalidRunBillingIdentity)
	}
	pricingSnapshotJSON := strings.TrimSpace(input.PricingSnapshotJSON)
	if pricingSnapshotJSON == "" || !json.Valid([]byte(pricingSnapshotJSON)) {
		return Run{}, fmt.Errorf("%w: pricing snapshot JSON", ErrInvalidRunBillingIdentity)
	}
	var pricingFields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(pricingSnapshotJSON), &pricingFields); err != nil || pricingFields == nil {
		return Run{}, fmt.Errorf("%w: pricing snapshot object", ErrInvalidRunBillingIdentity)
	}
	if hasDisallowedPricingMarker(pricingFields) {
		return Run{}, fmt.Errorf("%w: non-billable pricing snapshot", ErrInvalidRunBillingIdentity)
	}
	return Run{
		ConversationID:        input.ConversationID,
		RequestID:             requestID,
		RequestFingerprint:    append([]byte(nil), input.RequestFingerprint[:]...),
		RequestIdentityStatus: string(requestidentity.IdentityStatusReplayable),
		RequestIdentityMarker: "",
		UserMessageID:         input.UserMessageID,
		UserID:                input.UserID,
		AgentID:               input.AgentID,
		ProviderID:            input.ProviderID,
		ModelID:               strings.TrimSpace(input.ModelID),
		ModelDisplayName:      strings.TrimSpace(input.ModelDisplayName),
		PricingSnapshotJSON:   pricingSnapshotJSON,
		Status:                enum.AIRunStatusRunning,
		BillingStatus:         "pending",
		BillingReason:         "pending",
		StartedAt:             &startedAt,
	}, nil
}

func hasDisallowedPricingMarker(fields map[string]json.RawMessage) bool {
	if rawVersion, ok := fields["version"]; ok {
		var version string
		if json.Unmarshal(rawVersion, &version) == nil && strings.TrimSpace(version) == "legacy_unpriced_v1" {
			return true
		}
	}
	if rawBillable, ok := fields["billable"]; ok {
		var billable bool
		if json.Unmarshal(rawBillable, &billable) == nil && !billable {
			return true
		}
	}
	return false
}

func (r *GormRepository) CompleteRun(ctx context.Context, input CompleteRunRecord) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	_, err := r.finishRun(ctx, input.RunID, enum.AIRunStatusSuccess, enum.AIRunEventCompleted, enum.AIRunEventLabels[enum.AIRunEventCompleted], input.FinishedAt, input.DurationMS, map[string]any{
		"assistant_message_id": input.AssistantMessageID,
		"prompt_tokens":        nonNegativeInt(input.PromptTokens),
		"completion_tokens":    nonNegativeInt(input.CompletionTokens),
		"total_tokens":         nonNegativeInt(input.TotalTokens),
	})
	return err
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
	_, err := r.finishRun(ctx, input.RunID, input.Status, eventType, message, input.FinishedAt, input.DurationMS, nil)
	return err
}

func (r *GormRepository) finishRun(ctx context.Context, runID int64, status string, eventType string, message string, finishedAt time.Time, durationMS uint, extra map[string]any) (bool, error) {
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
	changed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := runningRunUpdateDB(tx, runID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return airun.ProjectTerminalDashboardFacts(ctx, tx, runID)
		}
		changed = true
		var maxSeq uint
		if err := tx.Model(&RunEvent{}).Where("run_id = ?", runID).Select("COALESCE(MAX(seq), 0)").Scan(&maxSeq).Error; err != nil {
			return err
		}
		if err := tx.Create(&RunEvent{RunID: runID, Seq: maxSeq + 1, EventType: eventType, Message: truncateRunMessage(message)}).Error; err != nil {
			return err
		}
		return airun.ProjectTerminalDashboardFacts(ctx, tx, runID)
	})
	return changed, err
}
func (r *GormRepository) TimeoutRuns(ctx context.Context, limit int, staleBefore time.Time, message string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if limit <= 0 {
		limit = defaultTimeoutLimit
	}
	if staleBefore.IsZero() {
		staleBefore = time.Now().Add(-defaultRunStaleTimeout)
	}
	var runs []Run
	if err := staleRunningRunsDB(r.db.WithContext(ctx), staleBefore).
		Limit(limit).
		Find(&runs).Error; err != nil {
		return 0, err
	}
	if len(runs) == 0 {
		return 0, nil
	}
	now := time.Now()
	var changed int64
	for _, run := range runs {
		ok, err := r.finishRun(ctx, run.ID, enum.AIRunStatusTimeout, enum.AIRunEventTimeout, message, now, durationSince(run.StartedAt, now), nil)
		if err != nil {
			return changed, err
		}
		if ok {
			changed++
		}
	}
	return changed, nil
}

func runningRunUpdateDB(db *gorm.DB, runID int64) *gorm.DB {
	return db.Model(&Run{}).Where("id = ? AND status = ?", runID, enum.AIRunStatusRunning)
}

func staleRunningRunsDB(db *gorm.DB, staleBefore time.Time) *gorm.DB {
	return db.Where("status = ? AND started_at IS NOT NULL AND started_at < ?", enum.AIRunStatusRunning, staleBefore).
		Where("NOT EXISTS (SELECT 1 FROM ai_usage_charges c WHERE c.run_id = ai_runs.id AND c.status = ?)", billing.ChargeStatusOpen).
		Order("started_at ASC, id ASC")
}

func (r *GormRepository) agentRuntimeDB(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Table("ai_agents AS a").
		Select(`a.id AS agent_id,
			a.name AS agent_name,
			a.provider_id AS provider_id,
			a.model_id AS model_id,
			a.model_display_name AS model_display_name,
			a.billing_multiplier_ppm AS billing_multiplier_ppm,
			a.system_prompt AS system_prompt,
			a.scenes_json AS scenes_json,
			a.status AS agent_status,
			e.engine_type AS engine_type,
			e.api_protocol AS api_protocol,
			e.base_url AS engine_base_url,
			e.api_key_enc AS engine_api_key_enc,
			e.status AS engine_status,
			pm.status AS provider_model_status,
			pm.official_model_id AS official_model_id,
			pm.official_catalog_version AS official_catalog_version,
			pm.mapping_status AS mapping_status`).
		Joins("JOIN ai_providers e ON e.id = a.provider_id AND e.is_del = ? AND e.status = ?", enum.CommonNo, enum.CommonYes).
		Joins("JOIN ai_provider_models pm ON pm.provider_id = a.provider_id AND pm.model_id = a.model_id AND pm.model_kind = ? AND pm.status = ? AND pm.mapping_status = ?", aiprovider.ModelKindChat, enum.CommonYes, officialmodel.MappingStatusMapped).
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
