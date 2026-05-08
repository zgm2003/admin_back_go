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

func (r *GormRepository) AgentOptions(ctx context.Context) ([]AgentOptionRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []AgentOptionRow
	err := r.db.WithContext(ctx).Table("ai_agents").Select("id, name").Where("is_del = ?", enum.CommonNo).Order("id DESC").Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.runsBase(ctx)
	db = applyListFilters(db, query)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ListRow
	err := db.Select("r.id, r.request_id, r.user_id, r.agent_id, a.name as agent_name, r.conversation_id, c.title as conversation_title, r.run_status, COALESCE(r.model_snapshot, '') as model_snapshot, r.prompt_tokens, r.completion_tokens, r.total_tokens, r.latency_ms, r.error_msg, r.created_at").
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
		Select("r.id, r.request_id, r.user_id, COALESCE(u.username, '') as username, r.agent_id, a.name as agent_name, r.conversation_id, c.title as conversation_title, r.run_status, COALESCE(r.model_snapshot, '') as model_snapshot, r.prompt_tokens, r.completion_tokens, r.total_tokens, r.latency_ms, r.error_msg, r.meta_json, r.created_at, r.updated_at").
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

func (r *GormRepository) Steps(ctx context.Context, runID int64) ([]StepRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []StepRow
	err := r.db.WithContext(ctx).Table("ai_run_steps s").
		Select("s.id, s.step_no, s.step_type, s.agent_id, COALESCE(a.name, '') as agent_name, s.model_snapshot, s.status, s.error_msg, s.latency_ms, s.payload_json, s.created_at").
		Joins("LEFT JOIN ai_agents a ON a.id = s.agent_id").
		Where("s.run_id = ?", runID).
		Where("s.is_del = ?", enum.CommonNo).
		Order("s.step_no ASC").
		Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) StatsSummary(ctx context.Context, query StatsFilter) (StatsSummaryRow, error) {
	if r == nil || r.db == nil {
		return StatsSummaryRow{}, ErrRepositoryNotConfigured
	}
	var row StatsSummaryRow
	db := applyStatsFilters(r.db.WithContext(ctx).Table("ai_runs r").Where("r.is_del = ?", enum.CommonNo), query)
	err := db.Select(statsSummarySelectSQL(), enum.AIRunStatusSuccess, enum.AIRunStatusFail).Scan(&row).Error
	return row, err
}

func (r *GormRepository) StatsByDate(ctx context.Context, query StatsListQuery) ([]StatsByDateRow, int64, error) {
	db := applyStatsListFilters(r.db.WithContext(ctx).Table("ai_runs r").Where("r.is_del = ?", enum.CommonNo), query)
	return scanGrouped[StatsByDateRow](db, "DATE(r.created_at) as date", "date DESC", query)
}

func (r *GormRepository) StatsByAgent(ctx context.Context, query StatsListQuery) ([]StatsByAgentRow, int64, error) {
	db := applyStatsListFilters(r.db.WithContext(ctx).Table("ai_runs r").Joins("LEFT JOIN ai_agents a ON a.id = r.agent_id").Where("r.is_del = ?", enum.CommonNo), query)
	return scanGrouped[StatsByAgentRow](db, "COALESCE(a.name, '') as agent_name", "total_runs DESC", query)
}

func (r *GormRepository) StatsByUser(ctx context.Context, query StatsListQuery) ([]StatsByUserRow, int64, error) {
	db := applyStatsListFilters(r.db.WithContext(ctx).Table("ai_runs r").Joins("LEFT JOIN users u ON u.id = r.user_id").Where("r.is_del = ?", enum.CommonNo), query)
	return scanGrouped[StatsByUserRow](db, "COALESCE(u.username, '') as username", "total_runs DESC", query)
}

func (r *GormRepository) runsBase(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Table("ai_runs r").
		Joins("LEFT JOIN ai_agents a ON a.id = r.agent_id").
		Joins("LEFT JOIN ai_conversations c ON c.id = r.conversation_id").
		Joins("LEFT JOIN users u ON u.id = r.user_id").
		Where("r.is_del = ?", enum.CommonNo)
}

func (r *GormRepository) messageSummary(ctx context.Context, runID int64, column string) *MessageSummary {
	var row MessageSummary
	err := r.db.WithContext(ctx).Table("ai_runs r").
		Select("m.id, m.content, m.meta_json, m.created_at").
		Joins("JOIN ai_messages m ON m.id = r."+column).
		Where("r.id = ?", runID).
		Where("m.is_del = ?", enum.CommonNo).
		Scan(&row).Error
	if err != nil || row.ID == 0 {
		return nil
	}
	return &row
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
	if query.AgentID != nil {
		db = db.Where("r.agent_id = ?", *query.AgentID)
	}
	return applyDateRange(db, query.DateStart, query.DateEnd)
}

func applyStatsFilters(db *gorm.DB, query StatsFilter) *gorm.DB {
	if query.AgentID != nil {
		db = db.Where("r.agent_id = ?", *query.AgentID)
	}
	if query.UserID != nil {
		db = db.Where("r.user_id = ?", *query.UserID)
	}
	return applyDateRange(db, query.DateStart, query.DateEnd)
}

func applyStatsListFilters(db *gorm.DB, query StatsListQuery) *gorm.DB {
	return applyStatsFilters(db, StatsFilter{DateStart: query.DateStart, DateEnd: query.DateEnd, AgentID: query.AgentID, UserID: query.UserID})
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
	groupExpr := strings.Split(groupSelect, " as ")[0]
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

func statsSummarySelectSQL() string {
	return "COUNT(*) as total_runs, SUM(CASE WHEN r.run_status = ? THEN 1 ELSE 0 END) as success_runs, SUM(CASE WHEN r.run_status = ? THEN 1 ELSE 0 END) as fail_runs, COALESCE(SUM(r.total_tokens), 0) as total_tokens, COALESCE(SUM(r.prompt_tokens), 0) as prompt_tokens, COALESCE(SUM(r.completion_tokens), 0) as completion_tokens, COALESCE(CAST(ROUND(AVG(r.latency_ms)) AS SIGNED), 0) as avg_latency_ms"
}

func statsGroupedSelectSQL(groupSelect string) string {
	return groupSelect + ", COUNT(*) as total_runs, COALESCE(SUM(r.total_tokens), 0) as total_tokens, COALESCE(SUM(r.prompt_tokens), 0) as prompt_tokens, COALESCE(SUM(r.completion_tokens), 0) as completion_tokens, COALESCE(CAST(ROUND(AVG(r.latency_ms)) AS SIGNED), 0) as avg_latency_ms"
}
