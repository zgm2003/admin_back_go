package airun

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
)

const timeLayout = "2006-01-02 15:04:05"

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	agents, err := repo.AgentOptions(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI智能体选项失败", err)
	}
	agentOptions := make([]dict.Option[int], 0, len(agents))
	for _, row := range agents {
		agentOptions = append(agentOptions, dict.Option[int]{Label: row.Name, Value: int(row.ID)})
	}
	return &InitResponse{Dict: InitDict{RunStatusArr: dict.AIRunStatusOptions(), AgentArr: agentOptions}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(query)
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI运行记录失败", err)
	}
	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, listItem(row))
	}
	return &ListResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}

func (s *Service) Detail(ctx context.Context, id int64) (*DetailResponse, *apperror.Error) {
	if id <= 0 {
		return nil, apperror.BadRequest("无效的AI运行ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Detail(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI运行详情失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI运行记录不存在")
	}
	steps, err := repo.Steps(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI运行步骤失败", err)
	}
	result := detailItem(*row, steps)
	return &result, nil
}

func (s *Service) Stats(ctx context.Context, query StatsFilter) (*StatsResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeStatsFilter(query)
	row, err := repo.StatsSummary(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI运行统计失败", err)
	}
	rate := float64(0)
	if row.TotalRuns > 0 {
		rate = math.Round(float64(row.SuccessRuns)*10000/float64(row.TotalRuns)) / 100
	}
	return &StatsResponse{DateRange: DateRange{Start: optionalString(query.DateStart), End: optionalString(query.DateEnd)}, Summary: StatsSummary{
		TotalRuns: row.TotalRuns, SuccessRate: rate, FailRuns: row.FailRuns,
		TotalTokens: row.TotalTokens, TotalPromptTokens: row.PromptTokens,
		TotalCompletionTokens: row.CompletionTokens, AvgLatencyMS: row.AvgLatencyMS,
	}}, nil
}

func (s *Service) StatsByDate(ctx context.Context, query StatsListQuery) (*StatsByDateResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeStatsListQuery(query)
	rows, total, err := repo.StatsByDate(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI运行日期统计失败", err)
	}
	list := make([]StatsByDateItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, StatsByDateItem{Date: row.Date, StatsMetricItem: metricItem(row.StatsMetricRow)})
	}
	return &StatsByDateResponse{List: list, Page: page(total, query.CurrentPage, query.PageSize)}, nil
}

func (s *Service) StatsByAgent(ctx context.Context, query StatsListQuery) (*StatsByAgentResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeStatsListQuery(query)
	rows, total, err := repo.StatsByAgent(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI运行智能体统计失败", err)
	}
	list := make([]StatsByAgentItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, StatsByAgentItem{AgentName: row.AgentName, StatsMetricItem: metricItem(row.StatsMetricRow)})
	}
	return &StatsByAgentResponse{List: list, Page: page(total, query.CurrentPage, query.PageSize)}, nil
}

func (s *Service) StatsByUser(ctx context.Context, query StatsListQuery) (*StatsByUserResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeStatsListQuery(query)
	rows, total, err := repo.StatsByUser(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI运行用户统计失败", err)
	}
	list := make([]StatsByUserItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, StatsByUserItem{Username: row.Username, StatsMetricItem: metricItem(row.StatsMetricRow)})
	}
	return &StatsByUserResponse{List: list, Page: page(total, query.CurrentPage, query.PageSize)}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI运行仓储未配置")
	}
	return s.repository, nil
}

func normalizeListQuery(query ListQuery) ListQuery {
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	query.RequestID = strings.TrimSpace(query.RequestID)
	query.DateStart = strings.TrimSpace(query.DateStart)
	query.DateEnd = strings.TrimSpace(query.DateEnd)
	return query
}

func normalizeStatsFilter(query StatsFilter) StatsFilter {
	query.DateStart = strings.TrimSpace(query.DateStart)
	query.DateEnd = strings.TrimSpace(query.DateEnd)
	return query
}

func normalizeStatsListQuery(query StatsListQuery) StatsListQuery {
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	query.DateStart = strings.TrimSpace(query.DateStart)
	query.DateEnd = strings.TrimSpace(query.DateEnd)
	return query
}

func listItem(row ListRow) ListItem {
	return ListItem{
		ID: row.ID, RequestID: row.RequestID, UserID: row.UserID, AgentID: row.AgentID, AgentName: row.AgentName,
		ConversationID: row.ConversationID, ConversationTitle: row.ConversationTitle, RunStatus: row.RunStatus,
		RunStatusName: enum.AIRunStatusLabels[row.RunStatus], ModelSnapshot: row.ModelSnapshot,
		PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens, TotalTokens: row.TotalTokens,
		LatencyMS: row.LatencyMS, LatencyStr: latencyString(row.LatencyMS), ErrorMsg: row.ErrorMsg,
		CreatedAt: formatTime(row.CreatedAt),
	}
}

func detailItem(row RunDetailRow, steps []StepRow) DetailResponse {
	items := make([]StepItem, 0, len(steps))
	for _, step := range steps {
		items = append(items, stepItem(step))
	}
	return DetailResponse{
		ID: row.ID, RequestID: row.RequestID, UserID: row.UserID, Username: row.Username, AgentID: row.AgentID,
		AgentName: row.AgentName, ConversationID: row.ConversationID, ConversationTitle: row.ConversationTitle,
		RunStatus: row.RunStatus, RunStatusName: enum.AIRunStatusLabels[row.RunStatus], ModelSnapshot: row.ModelSnapshot,
		PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens, TotalTokens: row.TotalTokens,
		LatencyMS: row.LatencyMS, LatencyStr: latencyString(row.LatencyMS), ErrorMsg: row.ErrorMsg,
		MetaJSON: decodeObject(row.MetaJSON), UserMessage: row.UserMessage, AssistantMessage: row.AssistantMessage,
		CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt), Steps: items,
	}
}

func stepItem(row StepRow) StepItem {
	return StepItem{
		ID: row.ID, StepNo: row.StepNo, StepType: row.StepType, StepTypeName: enum.AIRunStepTypeLabels[row.StepType],
		AgentID: row.AgentID, AgentName: row.AgentName, ModelSnapshot: row.ModelSnapshot, Status: row.Status,
		StatusName: enum.AIRunStepStatusLabels[row.Status], ErrorMsg: row.ErrorMsg, LatencyMS: row.LatencyMS,
		LatencyStr: latencyString(row.LatencyMS), PayloadJSON: decodeObject(row.PayloadJSON), CreatedAt: formatTime(row.CreatedAt),
	}
}

func metricItem(row StatsMetricRow) StatsMetricItem {
	return StatsMetricItem{
		TotalRuns: row.TotalRuns, TotalTokens: row.TotalTokens,
		TotalPromptTokens: row.PromptTokens, TotalCompletionTokens: row.CompletionTokens,
		AvgLatencyMS: row.AvgLatencyMS,
	}
}

func page(total int64, currentPage int, pageSize int) Page {
	return Page{CurrentPage: currentPage, PageSize: pageSize, Total: total, TotalPage: totalPage(total, pageSize)}
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

func latencyString(value *int) string {
	if value == nil {
		return "-"
	}
	if *value < 1000 {
		return fmt.Sprintf("%dms", *value)
	}
	return fmt.Sprintf("%.2fs", float64(*value)/1000)
}

func decodeObject(raw *string) JSONObject {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return JSONObject{}
	}
	var out JSONObject
	if err := json.Unmarshal([]byte(*raw), &out); err != nil {
		return JSONObject{}
	}
	return out
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}
