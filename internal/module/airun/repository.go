package airun

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

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

func (r *GormRepository) AppOptions(ctx context.Context) ([]OptionRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []OptionRow
	err := r.db.WithContext(ctx).Table("ai_apps").Select("id, name").Where("is_del = ?", enum.CommonNo).Where("status = ?", enum.CommonYes).Order("id DESC").Scan(&rows).Error
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

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := applyListFilters(r.runsBase(ctx), query)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ListRow
	err := db.Select(`r.id, r.request_id, r.user_id,
		r.app_id, COALESCE(a.name, '') as app_name,
		r.provider_id, COALESCE(e.name, '') as provider_name, COALESCE(e.engine_type, '') as engine_type,
		r.engine_task_id, r.engine_run_id,
		r.conversation_id, COALESCE(c.title, '') as conversation_title,
		r.run_status, COALESCE(r.model_snapshot, '') as model_snapshot,
		r.prompt_tokens, r.completion_tokens, r.total_tokens, r.cost, r.latency_ms, r.error_msg, r.created_at`).
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
		Select(`r.id, r.request_id, r.user_id, COALESCE(u.username, '') as username,
			r.app_id, COALESCE(a.name, '') as app_name,
			r.provider_id, COALESCE(e.name, '') as provider_name, COALESCE(e.engine_type, '') as engine_type,
			r.engine_task_id, r.engine_run_id,
			r.conversation_id, COALESCE(c.title, '') as conversation_title,
			r.run_status, COALESCE(r.model_snapshot, '') as model_snapshot,
			r.prompt_tokens, r.completion_tokens, r.total_tokens, r.cost, r.latency_ms, r.error_msg,
			r.meta_json, r.usage_json, r.output_snapshot_json, r.created_at, r.updated_at`).
		Where("r.id = ?", id).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.UserMessage = r.messageSummary(ctx, id, "user_message_id")
	row.AssistantMessage = r.messageSummary(ctx, id, "assistant_message_id")
	return &row, nil
}

func (r *GormRepository) Events(ctx context.Context, runID int64) ([]EventRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []EventRow
	err := r.db.WithContext(ctx).Table("ai_run_events").
		Select("id, seq, event_id, event_type, delta_text, payload_json, created_at").
		Where("run_id = ?", runID).
		Order("seq ASC").
		Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) StatsSummary(ctx context.Context, query StatsFilter) (StatsSummaryRow, error) {
	if r == nil || r.db == nil {
		return StatsSummaryRow{}, ErrRepositoryNotConfigured
	}
	var row StatsSummaryRow
	db := applyStatsFilters(r.db.WithContext(ctx).Table("ai_runs r").Where("r.is_del = ?", enum.CommonNo), query)
	err := db.Select(statsSummarySelectSQL(), enum.AIRunStatusSuccess, enum.AIRunStatusFail, enum.AIRunStatusCanceled).Scan(&row).Error
	return row, err
}

func (r *GormRepository) StatsByDate(ctx context.Context, query StatsListQuery) ([]StatsByDateRow, int64, error) {
	db := applyStatsListFilters(r.db.WithContext(ctx).Table("ai_runs r").Where("r.is_del = ?", enum.CommonNo), query)
	return scanGrouped[StatsByDateRow](db, "DATE(r.created_at) as date", "date DESC", query)
}

func (r *GormRepository) StatsByAgent(ctx context.Context, query StatsListQuery) ([]StatsByAgentRow, int64, error) {
	db := applyStatsListFilters(r.db.WithContext(ctx).Table("ai_runs r").Joins("LEFT JOIN ai_apps a ON a.id = r.app_id").Where("r.is_del = ?", enum.CommonNo), query)
	return scanGrouped[StatsByAgentRow](db, "r.app_id as app_id, COALESCE(a.name, '') as app_name", "total_runs DESC", query)
}

func (r *GormRepository) StatsByUser(ctx context.Context, query StatsListQuery) ([]StatsByUserRow, int64, error) {
	db := applyStatsListFilters(r.db.WithContext(ctx).Table("ai_runs r").Joins("LEFT JOIN users u ON u.id = r.user_id").Where("r.is_del = ?", enum.CommonNo), query)
	return scanGrouped[StatsByUserRow](db, "COALESCE(u.username, '') as username", "total_runs DESC", query)
}

func (r *GormRepository) runsBase(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Table("ai_runs r").
		Joins("LEFT JOIN ai_apps a ON a.id = r.app_id").
		Joins("LEFT JOIN ai_providers e ON e.id = r.provider_id").
		Joins("LEFT JOIN ai_conversations c ON c.id = r.conversation_id").
		Joins("LEFT JOIN users u ON u.id = r.user_id").
		Where("r.is_del = ?", enum.CommonNo)
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
	if query.RunStatus != nil {
		db = db.Where("r.run_status = ?", *query.RunStatus)
	}
	if query.UserID != nil {
		db = db.Where("r.user_id = ?", *query.UserID)
	}
	if strings.TrimSpace(query.RequestID) != "" {
		db = db.Where("r.request_id LIKE ?", "%"+strings.TrimSpace(query.RequestID)+"%")
	}
	if query.AppID != nil {
		db = db.Where("r.app_id = ?", *query.AppID)
	} else if query.AgentID != nil {
		db = db.Where("r.app_id = ?", *query.AgentID)
	}
	if query.ProviderID != nil {
		db = db.Where("r.provider_id = ?", *query.ProviderID)
	}
	return applyDateRange(db, query.DateStart, query.DateEnd)
}

func applyStatsFilters(db *gorm.DB, query StatsFilter) *gorm.DB {
	if query.AppID != nil {
		db = db.Where("r.app_id = ?", *query.AppID)
	} else if query.AgentID != nil {
		db = db.Where("r.app_id = ?", *query.AgentID)
	}
	if query.ProviderID != nil {
		db = db.Where("r.provider_id = ?", *query.ProviderID)
	}
	if query.UserID != nil {
		db = db.Where("r.user_id = ?", *query.UserID)
	}
	return applyDateRange(db, query.DateStart, query.DateEnd)
}

func applyStatsListFilters(db *gorm.DB, query StatsListQuery) *gorm.DB {
	return applyStatsFilters(db, StatsFilter{DateStart: query.DateStart, DateEnd: query.DateEnd, AppID: query.AppID, ProviderID: query.ProviderID, AgentID: query.AgentID, UserID: query.UserID})
}

func applyDateRange(db *gorm.DB, start string, end string) *gorm.DB {
	if strings.TrimSpace(start) != "" {
		db = db.Where("r.created_at >= ?", strings.TrimSpace(start))
	}
	if strings.TrimSpace(end) != "" {
		db = db.Where("r.created_at <= ?", strings.TrimSpace(end))
	}
	return db
}

func scanGrouped[T any](db *gorm.DB, groupSelect string, order string, query StatsListQuery) ([]T, int64, error) {
	groupExpr := groupExprFromSelect(groupSelect)
	countDB := db.Session(&gorm.Session{})
	var total int64
	if err := countDB.Select(groupExpr).Group(groupExpr).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []T
	err := db.Select(statsGroupedSelectSQL(groupSelect)).
		Group(groupExpr).
		Order(order).
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	return rows, total, err
}

func groupExprFromSelect(groupSelect string) string {
	parts := strings.Split(groupSelect, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		expr := strings.Split(strings.TrimSpace(part), " as ")[0]
		out = append(out, expr)
	}
	return strings.Join(out, ", ")
}

func statsSummarySelectSQL() string {
	return "COUNT(*) as total_runs, SUM(CASE WHEN r.run_status = ? THEN 1 ELSE 0 END) as success_runs, SUM(CASE WHEN r.run_status IN (?, ?) THEN 1 ELSE 0 END) as fail_runs, COALESCE(SUM(r.total_tokens), 0) as total_tokens, COALESCE(SUM(r.prompt_tokens), 0) as prompt_tokens, COALESCE(SUM(r.completion_tokens), 0) as completion_tokens, COALESCE(CAST(ROUND(AVG(r.latency_ms)) AS SIGNED), 0) as avg_latency_ms"
}

func statsGroupedSelectSQL(groupSelect string) string {
	return groupSelect + ", COUNT(*) as total_runs, COALESCE(SUM(r.total_tokens), 0) as total_tokens, COALESCE(SUM(r.prompt_tokens), 0) as prompt_tokens, COALESCE(SUM(r.completion_tokens), 0) as completion_tokens, COALESCE(CAST(ROUND(AVG(r.latency_ms)) AS SIGNED), 0) as avg_latency_ms"
}
