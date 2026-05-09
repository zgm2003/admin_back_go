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

var emptyJSONObject = json.RawMessage("{}")

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
	apps, err := repo.AppOptions(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI应用选项失败", err)
	}
	engines, err := repo.ProviderOptions(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商选项失败", err)
	}
	appOptions := optionItems(apps)
	return &InitResponse{Dict: InitDict{RunStatusArr: dict.AIRunStatusOptions(), AppArr: appOptions, AgentArr: appOptions, ProviderArr: optionItems(engines)}}, nil
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
	events, err := repo.Events(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI运行事件失败", err)
	}
	result := detailItem(*row, events)
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
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI运行应用统计失败", err)
	}
	list := make([]StatsByAgentItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, StatsByAgentItem{AppID: row.AppID, AppName: row.AppName, AgentName: row.AppName, StatsMetricItem: metricItem(row.StatsMetricRow)})
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
	if query.AppID == nil && query.AgentID != nil {
		query.AppID = query.AgentID
	}
	query.RequestID = strings.TrimSpace(query.RequestID)
	query.DateStart = strings.TrimSpace(query.DateStart)
	query.DateEnd = strings.TrimSpace(query.DateEnd)
	return query
}

func normalizeStatsFilter(query StatsFilter) StatsFilter {
	if query.AppID == nil && query.AgentID != nil {
		query.AppID = query.AgentID
	}
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
	if query.AppID == nil && query.AgentID != nil {
		query.AppID = query.AgentID
	}
	query.DateStart = strings.TrimSpace(query.DateStart)
	query.DateEnd = strings.TrimSpace(query.DateEnd)
	return query
}

func listItem(row ListRow) ListItem {
	return ListItem{
		ID: row.ID, RequestID: row.RequestID, UserID: row.UserID,
		AppID: row.AppID, AppName: row.AppName, AgentID: row.AppID, AgentName: row.AppName,
		ProviderID: row.ProviderID, ProviderName: row.ProviderName, EngineType: row.EngineType,
		EngineTaskID: row.EngineTaskID, EngineRunID: row.EngineRunID,
		ConversationID: row.ConversationID, ConversationTitle: row.ConversationTitle, RunStatus: row.RunStatus,
		RunStatusName: enum.AIRunStatusLabels[row.RunStatus], ModelSnapshot: row.ModelSnapshot,
		PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens, TotalTokens: row.TotalTokens, Cost: row.Cost,
		LatencyMS: row.LatencyMS, LatencyStr: latencyString(row.LatencyMS), ErrorMsg: row.ErrorMsg,
		CreatedAt: formatTime(row.CreatedAt),
	}
}

func detailItem(row RunDetailRow, events []EventRow) DetailResponse {
	items := make([]EventItem, 0, len(events))
	for _, event := range events {
		items = append(items, eventItem(event))
	}
	return DetailResponse{
		ID: row.ID, RequestID: row.RequestID, UserID: row.UserID, Username: row.Username,
		AppID: row.AppID, AppName: row.AppName, AgentID: row.AppID, AgentName: row.AppName,
		ProviderID: row.ProviderID, ProviderName: row.ProviderName, EngineType: row.EngineType,
		EngineTaskID: row.EngineTaskID, EngineRunID: row.EngineRunID,
		ConversationID: row.ConversationID, ConversationTitle: row.ConversationTitle,
		RunStatus: row.RunStatus, RunStatusName: enum.AIRunStatusLabels[row.RunStatus], ModelSnapshot: row.ModelSnapshot,
		PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens, TotalTokens: row.TotalTokens, Cost: row.Cost,
		LatencyMS: row.LatencyMS, LatencyStr: latencyString(row.LatencyMS), ErrorMsg: row.ErrorMsg,
		MetaJSON: rawJSON(row.MetaJSON), UsageJSON: rawJSON(row.UsageJSON), OutputSnapshotJSON: rawJSON(row.OutputSnapshotJSON),
		UserMessage: row.UserMessage, AssistantMessage: row.AssistantMessage, Events: items, Steps: []StepItem{},
		CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}
}

func eventItem(row EventRow) EventItem {
	return EventItem{ID: row.ID, Seq: row.Seq, EventID: row.EventID, EventType: row.EventType, DeltaText: row.DeltaText, PayloadJSON: rawJSON(row.PayloadJSON), CreatedAt: formatTime(row.CreatedAt)}
}

func stepItem(row StepRow) StepItem {
	return StepItem{ID: row.ID, StepNo: row.StepNo, StepType: row.StepType, StepTypeName: enum.AIRunStepTypeLabels[row.StepType], AgentID: row.AgentID, AgentName: row.AgentName, ModelSnapshot: row.ModelSnapshot, Status: row.Status, StatusName: enum.AIRunStepStatusLabels[row.Status], ErrorMsg: row.ErrorMsg, LatencyMS: row.LatencyMS, LatencyStr: latencyString(row.LatencyMS), PayloadJSON: rawJSON(row.PayloadJSON), CreatedAt: formatTime(row.CreatedAt)}
}

func metricItem(row StatsMetricRow) StatsMetricItem {
	return StatsMetricItem{TotalRuns: row.TotalRuns, TotalTokens: row.TotalTokens, TotalPromptTokens: row.PromptTokens, TotalCompletionTokens: row.CompletionTokens, AvgLatencyMS: row.AvgLatencyMS}
}

func optionItems(rows []OptionRow) []dict.Option[int] {
	items := make([]dict.Option[int], 0, len(rows))
	for _, row := range rows {
		items = append(items, dict.Option[int]{Label: row.Name, Value: int(row.ID)})
	}
	return items
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

func decodeObject(raw *string) json.RawMessage {
	return rawJSON(raw)
}

func rawJSON(raw *string) json.RawMessage {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return cloneRawJSON(emptyJSONObject)
	}
	var out any
	if err := json.Unmarshal([]byte(*raw), &out); err != nil {
		return cloneRawJSON(emptyJSONObject)
	}
	return json.RawMessage(*raw)
}

func cloneRawJSON(raw json.RawMessage) json.RawMessage {
	out := make(json.RawMessage, len(raw))
	copy(out, raw)
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
