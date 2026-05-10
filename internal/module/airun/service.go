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

var knowledgeRetrievalStatusLabels = map[string]string{
	"success": "检索成功",
	"failed":  "检索失败",
	"skipped": "未检索",
}

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
	engines, err := repo.ProviderOptions(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商选项失败", err)
	}
	agentOptions := optionItems(agents)
	return &InitResponse{Dict: InitDict{StatusArr: dict.AIRunStatusOptions(), AgentArr: agentOptions, ProviderArr: optionItems(engines)}}, nil
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
	toolCalls, err := repo.ToolCalls(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI工具调用失败", err)
	}
	knowledgeRetrievals, appErr := s.knowledgeRetrievalItems(ctx, repo, id)
	if appErr != nil {
		return nil, appErr
	}
	result := detailItem(*row, events, knowledgeRetrievals, toolCalls)
	return &result, nil
}

func (s *Service) knowledgeRetrievalItems(ctx context.Context, repo Repository, runID int64) ([]KnowledgeRetrievalItem, *apperror.Error) {
	rows, err := repo.KnowledgeRetrievals(ctx, runID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI知识库检索记录失败", err)
	}
	items := make([]KnowledgeRetrievalItem, 0, len(rows))
	for _, row := range rows {
		hits, err := repo.KnowledgeRetrievalHits(ctx, row.ID)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI知识库检索命中失败", err)
		}
		items = append(items, knowledgeRetrievalItem(row, hits))
	}
	return items, nil
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
		TotalCompletionTokens: row.CompletionTokens, AvgDurationMS: row.AvgDurationMS,
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
		list = append(list, StatsByAgentItem{AgentID: row.AgentID, AgentName: row.AgentName, StatsMetricItem: metricItem(row.StatsMetricRow)})
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
		ID: row.ID, RequestID: row.RequestID, UserID: row.UserID,
		AgentID: row.AgentID, AgentName: row.AgentName,
		ProviderID: row.ProviderID, ProviderName: row.ProviderName,
		ConversationID: row.ConversationID, ConversationTitle: row.ConversationTitle,
		Status: row.Status, StatusName: enum.AIRunStatusLabels[row.Status],
		ModelID: row.ModelID, ModelDisplayName: row.ModelDisplayName,
		PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens, TotalTokens: row.TotalTokens,
		DurationMS: row.DurationMS, DurationText: durationString(row.DurationMS), ErrorMessage: row.ErrorMessage,
		CreatedAt: formatTime(row.CreatedAt),
	}
}

func detailItem(row RunDetailRow, events []EventRow, knowledgeRetrievals []KnowledgeRetrievalItem, toolCalls []ToolCallRow) DetailResponse {
	items := make([]EventItem, 0, len(events))
	for _, event := range events {
		items = append(items, eventItem(event, row.StartedAt))
	}
	callItems := make([]ToolCallItem, 0, len(toolCalls))
	for _, call := range toolCalls {
		callItems = append(callItems, toolCallItem(call))
	}
	return DetailResponse{
		ID: row.ID, RequestID: row.RequestID, UserID: row.UserID, Username: row.Username,
		AgentID: row.AgentID, AgentName: row.AgentName,
		ProviderID: row.ProviderID, ProviderName: row.ProviderName,
		ConversationID: row.ConversationID, ConversationTitle: row.ConversationTitle,
		Status: row.Status, StatusName: enum.AIRunStatusLabels[row.Status],
		ModelID: row.ModelID, ModelDisplayName: row.ModelDisplayName,
		PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens, TotalTokens: row.TotalTokens,
		DurationMS: row.DurationMS, DurationText: durationString(row.DurationMS), ErrorMessage: row.ErrorMessage,
		UserMessage: row.UserMessage, AssistantMessage: row.AssistantMessage, Events: items, KnowledgeRetrievals: knowledgeRetrievals, ToolCalls: callItems,
		StartedAt: formatOptionalTime(row.StartedAt), FinishedAt: formatOptionalTime(row.FinishedAt),
		CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}
}

func knowledgeRetrievalItem(row KnowledgeRetrievalRow, hits []KnowledgeHitRow) KnowledgeRetrievalItem {
	items := make([]KnowledgeHitItem, 0, len(hits))
	for _, hit := range hits {
		items = append(items, knowledgeHitItem(hit))
	}
	return KnowledgeRetrievalItem{
		ID: row.ID, RunID: row.RunID, Query: row.Query,
		Status: row.Status, StatusName: knowledgeRetrievalStatusName(row.Status),
		TotalHits: row.TotalHits, SelectedHits: row.SelectedHits,
		DurationMS: row.DurationMS, DurationText: durationString(row.DurationMS),
		ErrorMessage: row.ErrorMessage, CreatedAt: formatTime(row.CreatedAt),
		Hits: items,
	}
}

func knowledgeHitItem(row KnowledgeHitRow) KnowledgeHitItem {
	return KnowledgeHitItem{
		ID: row.ID, KnowledgeBaseID: row.KnowledgeBaseID, KnowledgeBaseName: row.KnowledgeBaseName,
		DocumentID: row.DocumentID, DocumentTitle: row.DocumentTitle,
		ChunkID: row.ChunkID, ChunkIndex: row.ChunkIndex,
		Score: row.Score, RankNo: row.RankNo, ContentSnapshot: row.ContentSnapshot,
		Status: row.Status, StatusName: knowledgeHitStatusName(row.Status),
		SkipReason: row.SkipReason, CreatedAt: formatTime(row.CreatedAt),
	}
}

func knowledgeRetrievalStatusName(status string) string {
	if label, ok := knowledgeRetrievalStatusLabels[status]; ok {
		return label
	}
	return status
}

func knowledgeHitStatusName(status uint) string {
	switch status {
	case 1:
		return "进入上下文"
	case 2:
		return "已跳过"
	default:
		return ""
	}
}

func toolCallItem(row ToolCallRow) ToolCallItem {
	return ToolCallItem{
		ID:            row.ID,
		ToolID:        row.ToolID,
		ToolCode:      row.ToolCode,
		ToolName:      row.ToolName,
		CallID:        row.CallID,
		Status:        row.Status,
		ArgumentsJSON: rawJSONString(row.ArgumentsJSON),
		ResultJSON:    rawJSON(row.ResultJSON),
		ErrorMessage:  row.ErrorMessage,
		DurationMS:    row.DurationMS,
		StartedAt:     formatTime(row.StartedAt),
		FinishedAt:    formatOptionalTime(row.FinishedAt),
	}
}

func eventItem(row EventRow, startedAt *time.Time) EventItem {
	elapsedMS := eventElapsedMS(row.CreatedAt, startedAt)
	return EventItem{
		ID: row.ID, Seq: row.Seq,
		EventType: row.EventType, EventTypeName: enum.AIRunEventLabels[row.EventType],
		Message:   row.Message,
		ElapsedMS: elapsedMS, ElapsedText: durationString(elapsedMS),
		CreatedAt: formatTime(row.CreatedAt),
	}
}

func metricItem(row StatsMetricRow) StatsMetricItem {
	return StatsMetricItem{TotalRuns: row.TotalRuns, TotalTokens: row.TotalTokens, TotalPromptTokens: row.PromptTokens, TotalCompletionTokens: row.CompletionTokens, AvgDurationMS: row.AvgDurationMS}
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

func durationString(value *uint) string {
	if value == nil {
		return "-"
	}
	if *value < 1000 {
		return fmt.Sprintf("%dms", *value)
	}
	return fmt.Sprintf("%.2fs", float64(*value)/1000)
}

func eventElapsedMS(createdAt time.Time, startedAt *time.Time) *uint {
	if startedAt == nil || startedAt.IsZero() || createdAt.IsZero() || createdAt.Before(*startedAt) {
		return nil
	}
	value := uint(createdAt.Sub(*startedAt).Milliseconds())
	return &value
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

func rawJSONString(raw string) json.RawMessage {
	return rawJSON(&raw)
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

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTime(*value)
}
