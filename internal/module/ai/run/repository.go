package airun

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("airun repository not configured")

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) AgentOptions(ctx context.Context) ([]OptionRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []OptionRow
	err := r.db.WithContext(ctx).Table("ai_agents").Select("id, name").Where("is_del = ?", enum.CommonNo).Where("status = ?", enum.CommonYes).Order("id DESC").Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) ProviderOptions(ctx context.Context) ([]OptionRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []OptionRow
	err := r.db.WithContext(ctx).Table("ai_providers").Select("id, name").Where("is_del = ?", enum.CommonNo).Where("status = ?", enum.CommonYes).Order("id DESC").Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) HistoricalModelOptions(ctx context.Context, startAt, endExclusive time.Time) ([]HistoricalModelRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if startAt.IsZero() || endExclusive.IsZero() || !startAt.Before(endExclusive) {
		return nil, errors.New("historical model date range is invalid")
	}
	rows := make([]HistoricalModelRow, 0)
	err := r.db.WithContext(ctx).Raw(`
SELECT model_id, model_display_name
FROM (
  SELECT
    model_id,
    model_display_name,
    ROW_NUMBER() OVER (PARTITION BY model_id ORDER BY created_at DESC, id DESC) AS row_no
  FROM ai_runs
  WHERE created_at >= ? AND created_at < ? AND model_id <> ''
) historical_models
WHERE row_no = 1
ORDER BY model_id ASC`, startAt, endExclusive).Scan(&rows).Error
	return rows, err
}

const runListFinalAttemptJoinSQL = `LEFT JOIN ai_provider_attempts final_attempt
ON final_attempt.run_id = r.id
AND final_attempt.state IN ('succeeded', 'failed', 'canceled', 'outcome_unknown')
AND NOT EXISTS (
  SELECT 1
  FROM ai_provider_attempts newer_attempt
  WHERE newer_attempt.run_id = final_attempt.run_id
    AND newer_attempt.state IN ('succeeded', 'failed', 'canceled', 'outcome_unknown')
    AND (
      newer_attempt.attempt_no > final_attempt.attempt_no
      OR (newer_attempt.attempt_no = final_attempt.attempt_no AND newer_attempt.id > final_attempt.id)
    )
)`

func runListSelectSQL() string {
	return `r.id, r.request_id, r.user_id,
		r.agent_id, COALESCE(a.name, '') as agent_name,
		r.provider_id, COALESCE(p.name, '') as provider_name,
		r.platform, r.input_snapshot,
		r.conversation_id, COALESCE(c.title, '') as conversation_title,
		r.status, r.model_id, r.model_display_name,
		r.billing_status, r.billing_reason, COALESCE(final_attempt.error_code, '') AS error_code,
		r.prompt_tokens, r.completion_tokens, r.total_tokens, r.duration_ms, r.error_message, r.created_at`
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	countDB := applyListFilters(r.listBase(ctx, query, false), query)
	var total int64
	if err := countDB.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ListRow
	err := applyListFilters(r.listBase(ctx, query, true), query).Select(runListSelectSQL()).
		Order("r.id DESC").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	return rows, total, err
}

func (r *GormRepository) Detail(ctx context.Context, id int64) (*RunDetailRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row RunDetailRow
	err := r.runsBase(ctx).
		Joins("LEFT JOIN ai_reply_commands rc ON rc.user_id = r.user_id AND rc.request_id = r.request_id").
		Joins(runListFinalAttemptJoinSQL).
		Select(`r.id, r.request_id, r.user_id, COALESCE(u.username, '') as username,
			r.agent_id, COALESCE(a.name, '') as agent_name,
			r.provider_id, COALESCE(p.name, '') as provider_name,
			r.platform, r.input_snapshot,
			r.conversation_id, COALESCE(c.title, '') as conversation_title,
			r.status, r.model_id, r.model_display_name, COALESCE(final_attempt.error_code, '') AS error_code,
			r.prompt_tokens, r.completion_tokens, r.total_tokens, r.duration_ms, r.error_message,
			r.pricing_snapshot_json, r.billing_status, r.billing_reason,
			r.started_at, r.finished_at, r.settled_at, r.liked_at, r.created_at, r.updated_at,
			rc.request_received_at, rc.accepted_at, rc.claimed_at, COALESCE(rc.claim_source, '') AS claim_source`).
		Where("r.id = ?", id).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, nil
	}
	row.UserMessage = r.messageSummary(ctx, id, "user_message_id")
	row.AssistantMessage = r.messageSummary(ctx, id, "assistant_message_id")
	return &row, nil
}

func (r *GormRepository) BillingDetail(ctx context.Context, runID int64) (*ChargeRow, []UsageChargeItemRow, []ProviderAttemptRow, error) {
	if r == nil || r.db == nil {
		return nil, nil, nil, ErrRepositoryNotConfigured
	}
	var charge ChargeRow
	if err := r.db.WithContext(ctx).Table("ai_usage_charges").
		Select("id, held_units, actual_units, status").
		Where("run_id = ?", runID).
		Limit(1).
		Scan(&charge).Error; err != nil {
		return nil, nil, nil, err
	}
	var chargePtr *ChargeRow
	if charge.ID != 0 {
		chargePtr = &charge
	}

	items := make([]UsageChargeItemRow, 0)
	if err := r.db.WithContext(ctx).Table("ai_usage_charge_items i").
		Select(`i.attempt_id, a.attempt_no, a.state AS attempt_state,
			i.category, i.tier_key, i.quantity, i.unit,
			i.unit_price_units, i.unit_scale, i.amount_units`).
		Joins("JOIN ai_usage_charges c ON c.id = i.charge_id").
		Joins("JOIN ai_provider_attempts a ON a.id = i.attempt_id AND a.run_id = c.run_id").
		Where("c.run_id = ?", runID).
		Order("a.attempt_no ASC, i.id ASC").
		Scan(&items).Error; err != nil {
		return nil, nil, nil, err
	}

	attempts := make([]ProviderAttemptRow, 0)
	if err := r.db.WithContext(ctx).Table("ai_provider_attempts").
		Select("id, attempt_no, state, provider_request_id, usage_status, usage_json, prepared_request_json, prepare_started_at, dispatched_at, first_delta_at, finished_at").
		Where("run_id = ?", runID).
		Order("attempt_no ASC").
		Scan(&attempts).Error; err != nil {
		return nil, nil, nil, err
	}
	return chargePtr, items, attempts, nil
}

func (r *GormRepository) Events(ctx context.Context, runID int64) ([]EventRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []EventRow
	err := r.db.WithContext(ctx).Table("ai_run_events").
		Select("id, seq, event_type, message, created_at").
		Where("run_id = ?", runID).
		Order("seq ASC").
		Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) ToolCalls(ctx context.Context, runID int64) ([]ToolCallRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []ToolCallRow
	err := r.db.WithContext(ctx).Table("ai_tool_calls").
		Select("id, tool_id, tool_code, tool_name, call_id, status, arguments_json, result_json, error_message, duration_ms, started_at, finished_at").
		Where("run_id = ?", runID).
		Order("id ASC").
		Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) KnowledgeRetrievals(ctx context.Context, runID int64) ([]KnowledgeRetrievalRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []KnowledgeRetrievalRow
	err := r.db.WithContext(ctx).Table("ai_knowledge_retrievals").
		Select("id, run_id, query, status, total_hits, selected_hits, duration_ms, error_message, created_at").
		Where("run_id = ?", runID).
		Where("is_del = ?", enum.CommonNo).
		Order("created_at ASC, id ASC").
		Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) KnowledgeRetrievalHits(ctx context.Context, retrievalIDs []int64) ([]KnowledgeHitRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if len(retrievalIDs) == 0 {
		return []KnowledgeHitRow{}, nil
	}
	var rows []KnowledgeHitRow
	err := r.db.WithContext(ctx).Table("ai_knowledge_retrieval_hits").
		Select("id, retrieval_id, knowledge_base_id, knowledge_base_name, document_id, document_title, chunk_id, chunk_index, score, rank_no, content_snapshot, status, skip_reason, created_at").
		Where("retrieval_id IN ?", retrievalIDs).
		Where("is_del = ?", enum.CommonNo).
		Order("retrieval_id ASC, rank_no ASC, id ASC").
		Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) runsBase(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Table("ai_runs r").
		Joins("LEFT JOIN ai_agents a ON a.id = r.agent_id").
		Joins("LEFT JOIN ai_providers p ON p.id = r.provider_id").
		Joins("LEFT JOIN ai_conversations c ON c.id = r.conversation_id").
		Joins("LEFT JOIN users u ON u.id = r.user_id")
}

func (r *GormRepository) listBase(ctx context.Context, query ListQuery, includeFinalAttempt bool) *gorm.DB {
	db := r.runsBase(ctx)
	if query.BillingAnomaly != "" {
		db = db.Joins("LEFT JOIN ai_usage_charges charge ON charge.run_id = r.id")
	}
	if includeFinalAttempt || query.ErrorCode != "" {
		db = db.Joins(runListFinalAttemptJoinSQL)
	}
	return db
}

func (r *GormRepository) messageSummary(ctx context.Context, runID int64, column string) *MessageSummary {
	var row struct {
		ID          int64
		Role        int
		ContentType string
		Content     string
		MetaJSON    *string
		CreatedAt   string
	}
	err := r.db.WithContext(ctx).Table("ai_runs r").
		Select("m.id, m.role, m.content_type, m.content, m.meta_json, DATE_FORMAT(m.created_at, '%Y-%m-%d %H:%i:%s') as created_at").
		Joins("JOIN ai_messages m ON m.id = r."+column).
		Where("r.id = ?", runID).
		Where("m.is_del = ?", enum.CommonNo).
		Scan(&row).Error
	if err != nil || row.ID == 0 {
		return nil
	}
	return &MessageSummary{ID: row.ID, Role: row.Role, ContentType: row.ContentType, Content: row.Content, MetaJSON: rawJSON(row.MetaJSON), CreatedAt: row.CreatedAt}
}

func applyListFilters(db *gorm.DB, query ListQuery) *gorm.DB {
	if strings.TrimSpace(query.Status) != "" {
		db = db.Where("r.status = ?", strings.TrimSpace(query.Status))
	}
	if strings.TrimSpace(query.Platform) != "" {
		db = db.Where("r.platform = ?", strings.TrimSpace(query.Platform))
	}
	if query.UserID != nil {
		db = db.Where("r.user_id = ?", *query.UserID)
	}
	if strings.TrimSpace(query.RequestID) != "" {
		db = db.Where("r.request_id LIKE ?", "%"+strings.TrimSpace(query.RequestID)+"%")
	}
	if query.AgentID != nil {
		db = db.Where("r.agent_id = ?", *query.AgentID)
	}
	if query.ProviderID != nil {
		db = db.Where("r.provider_id = ?", *query.ProviderID)
	}
	if query.ModelID != "" {
		db = db.Where("r.model_id = ?", query.ModelID)
	}
	if query.BillingStatus != "" {
		db = db.Where("r.billing_status = ?", query.BillingStatus)
	}
	if query.BillingReason != "" {
		db = db.Where("r.billing_reason = ?", query.BillingReason)
	}
	if query.ErrorCode != "" {
		db = db.Where("r.status IN ?", []string{enum.AIRunStatusFailed, enum.AIRunStatusTimeout, enum.AIRunStatusOutcomeUnknown})
		db = db.Where("COALESCE(NULLIF(TRIM(final_attempt.error_code), ''), 'unclassified') = ?", query.ErrorCode)
	}
	if query.ToolCode != "" {
		db = db.Where("EXISTS (SELECT 1 FROM ai_tool_calls tc WHERE tc.run_id = r.id AND tc.tool_code = ?)", query.ToolCode)
	}
	if query.RunAnomaly != "" {
		db = db.Where("("+dashboardRunAnomalyCaseSQL()+") = ?", query.StaleBefore, query.RunAnomaly)
	}
	if query.BillingAnomaly != "" {
		db = db.Where("("+dashboardBillingAnomalyCaseSQL()+") = ?", query.StaleBefore, query.BillingAnomaly)
	}
	if !query.StartAt.IsZero() {
		db = db.Where("r.created_at >= ?", query.StartAt)
	}
	if !query.EndExclusive.IsZero() {
		db = db.Where("r.created_at < ?", query.EndExclusive)
	}
	return db
}
