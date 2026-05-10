package airun

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/enum"
)

type fakeRepository struct {
	agents      []OptionRow
	engines     []OptionRow
	listQuery   ListQuery
	rows        []ListRow
	total       int64
	run         *RunDetailRow
	events      []EventRow
	toolCalls   []ToolCallRow
	retrievals  []KnowledgeRetrievalRow
	hits        map[int64][]KnowledgeHitRow
	summary     StatsSummaryRow
	metricQuery StatsListQuery
	byDate      []StatsByDateRow
	byAgent     []StatsByAgentRow
	byUser      []StatsByUserRow
	metricTotal int64
}

func (f *fakeRepository) AgentOptions(ctx context.Context) ([]OptionRow, error) {
	return f.agents, nil
}
func (f *fakeRepository) ProviderOptions(ctx context.Context) ([]OptionRow, error) {
	return f.engines, nil
}
func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	f.listQuery = query
	return f.rows, f.total, nil
}
func (f *fakeRepository) Detail(ctx context.Context, id int64) (*RunDetailRow, error) {
	return f.run, nil
}
func (f *fakeRepository) Events(ctx context.Context, runID int64) ([]EventRow, error) {
	return f.events, nil
}
func (f *fakeRepository) ToolCalls(ctx context.Context, runID int64) ([]ToolCallRow, error) {
	return f.toolCalls, nil
}
func (f *fakeRepository) KnowledgeRetrievals(ctx context.Context, runID int64) ([]KnowledgeRetrievalRow, error) {
	return f.retrievals, nil
}
func (f *fakeRepository) KnowledgeRetrievalHits(ctx context.Context, retrievalID int64) ([]KnowledgeHitRow, error) {
	return f.hits[retrievalID], nil
}
func (f *fakeRepository) StatsSummary(ctx context.Context, query StatsFilter) (StatsSummaryRow, error) {
	return f.summary, nil
}
func (f *fakeRepository) StatsByDate(ctx context.Context, query StatsListQuery) ([]StatsByDateRow, int64, error) {
	f.metricQuery = query
	return f.byDate, f.metricTotal, nil
}
func (f *fakeRepository) StatsByAgent(ctx context.Context, query StatsListQuery) ([]StatsByAgentRow, int64, error) {
	f.metricQuery = query
	return f.byAgent, f.metricTotal, nil
}
func (f *fakeRepository) StatsByUser(ctx context.Context, query StatsListQuery) ([]StatsByUserRow, int64, error) {
	f.metricQuery = query
	return f.byUser, f.metricTotal, nil
}

func TestInitReturnsStatusAgentAndProviderOptions(t *testing.T) {
	repo := &fakeRepository{agents: []OptionRow{{ID: 3, Name: "客服智能体"}}, engines: []OptionRow{{ID: 2, Name: "Dify"}}}
	res, appErr := NewService(repo).Init(context.Background())
	if appErr != nil {
		t.Fatalf("Init returned error: %v", appErr)
	}
	if len(res.Dict.StatusArr) == 0 || res.Dict.AgentArr[0].Value != 3 || res.Dict.ProviderArr[0].Value != 2 {
		t.Fatalf("unexpected init response: %#v", res)
	}
}

func TestListFiltersAndMapsDuration(t *testing.T) {
	created := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	status := enum.AIRunStatusSuccess
	agentID := int64(3)
	repo := &fakeRepository{rows: []ListRow{{ID: 1, RequestID: "rid", UserID: 7, AgentID: 3, AgentName: "agent", ProviderID: 2, ProviderName: "OpenAI", ConversationID: 4, ConversationTitle: "chat", Status: status, ModelID: "gpt-5.4", ModelDisplayName: "GPT-5.4", TotalTokens: 12, DurationMS: ptrUint(1530), CreatedAt: created}}, total: 1}
	res, appErr := NewService(repo).List(context.Background(), ListQuery{Status: status, RequestID: " rid ", AgentID: &agentID, CurrentPage: 0, PageSize: 0})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	if repo.listQuery.CurrentPage != 1 || repo.listQuery.PageSize != 20 || repo.listQuery.RequestID != "rid" || repo.listQuery.AgentID == nil || *repo.listQuery.AgentID != 3 {
		t.Fatalf("unexpected query: %#v", repo.listQuery)
	}
	if len(res.List) != 1 || res.List[0].DurationText != "1.53s" || res.List[0].StatusName == "" || res.List[0].ModelID != "gpt-5.4" {
		t.Fatalf("unexpected list response: %#v", res)
	}
}

func TestDetailReturnsMessagesAndPersistedEvents(t *testing.T) {
	startedAt := time.Date(2026, 5, 10, 11, 18, 14, 0, time.UTC)
	repo := &fakeRepository{
		run:       &RunDetailRow{ID: 1, RequestID: "rid", UserID: 7, Username: "admin", AgentID: 3, AgentName: "agent", ProviderID: 2, ProviderName: "OpenAI", ConversationID: 4, ConversationTitle: "chat", Status: enum.AIRunStatusSuccess, ModelID: "gpt-5.4", StartedAt: &startedAt, UserMessage: &MessageSummary{ID: 10, Content: "hi"}, AssistantMessage: &MessageSummary{ID: 11, Content: "ok"}},
		events:    []EventRow{{ID: 2, Seq: 1, EventType: enum.AIRunEventCompleted, Message: "生成完成", CreatedAt: startedAt.Add(1530 * time.Millisecond)}},
		toolCalls: []ToolCallRow{{ID: 8, ToolID: 1, ToolCode: "admin_user_count", ToolName: "查询当前用户量", CallID: ptrString("call-1"), Status: "success", ArgumentsJSON: `{"scope":"all"}`, ResultJSON: ptrString(`{"total_users":1015}`), DurationMS: ptrUint(12), StartedAt: startedAt, FinishedAt: &startedAt}},
	}
	res, appErr := NewService(repo).Detail(context.Background(), 1)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	if res.UserMessage == nil || res.AssistantMessage == nil || len(res.Events) != 1 || res.Events[0].EventType != enum.AIRunEventCompleted || res.Events[0].EventTypeName != "生成完成" || res.Events[0].Message != "生成完成" || res.Events[0].ElapsedMS == nil || *res.Events[0].ElapsedMS != 1530 || res.Events[0].ElapsedText != "1.53s" || res.AgentName != "agent" || res.ModelID != "gpt-5.4" {
		t.Fatalf("unexpected detail: %#v", res)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].ToolCode != "admin_user_count" || string(res.ToolCalls[0].ArgumentsJSON) != `{"scope":"all"}` || string(res.ToolCalls[0].ResultJSON) != `{"total_users":1015}` || res.ToolCalls[0].DurationMS == nil || *res.ToolCalls[0].DurationMS != 12 {
		t.Fatalf("unexpected tool calls: %#v", res.ToolCalls)
	}
}

func TestDetailIncludesKnowledgeRetrievals(t *testing.T) {
	startedAt := time.Date(2026, 5, 10, 20, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		run:        &RunDetailRow{ID: 1, RequestID: "rid", Status: enum.AIRunStatusSuccess, CreatedAt: startedAt, UpdatedAt: startedAt},
		retrievals: []KnowledgeRetrievalRow{{ID: 7, RunID: 1, Query: "项目架构", Status: "success", TotalHits: 2, SelectedHits: 1, DurationMS: ptrUint(8), CreatedAt: startedAt}},
		hits: map[int64][]KnowledgeHitRow{
			7: {{ID: 8, RetrievalID: 7, KnowledgeBaseID: 1, KnowledgeBaseName: "架构库", DocumentID: 2, DocumentTitle: "Go 后端架构", ChunkID: 3, ChunkIndex: 1, Score: 0.82, RankNo: 1, ContentSnapshot: "Gin modular monolith", Status: 1, CreatedAt: startedAt}},
		},
	}
	res, appErr := NewService(repo).Detail(context.Background(), 1)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	if len(res.KnowledgeRetrievals) != 1 || len(res.KnowledgeRetrievals[0].Hits) != 1 {
		t.Fatalf("missing knowledge retrievals: %#v", res.KnowledgeRetrievals)
	}
	retrieval := res.KnowledgeRetrievals[0]
	if retrieval.Query != "项目架构" || retrieval.StatusName != "检索成功" || retrieval.DurationText != "8ms" || retrieval.SelectedHits != 1 || retrieval.TotalHits != 2 {
		t.Fatalf("unexpected retrieval: %#v", retrieval)
	}
	hit := retrieval.Hits[0]
	if hit.KnowledgeBaseName != "架构库" || hit.DocumentTitle != "Go 后端架构" || hit.StatusName != "进入上下文" || hit.ContentSnapshot != "Gin modular monolith" {
		t.Fatalf("unexpected retrieval hit: %#v", hit)
	}
}

func TestStatsSummaryComputesRatesAndTotals(t *testing.T) {
	repo := &fakeRepository{summary: StatsSummaryRow{TotalRuns: 10, SuccessRuns: 7, FailRuns: 2, TotalTokens: 100, PromptTokens: 40, CompletionTokens: 60, AvgDurationMS: 1234}}
	res, appErr := NewService(repo).Stats(context.Background(), StatsFilter{})
	if appErr != nil {
		t.Fatalf("Stats returned error: %v", appErr)
	}
	if res.Summary.SuccessRate != 70 || res.Summary.TotalTokens != 100 || res.Summary.AvgDurationMS != 1234 {
		t.Fatalf("unexpected stats: %#v", res)
	}
}

func TestStatsListsArePaginatedAndNormalized(t *testing.T) {
	agentID := int64(5)
	repo := &fakeRepository{byAgent: []StatsByAgentRow{{AgentID: 5, AgentName: "agent", StatsMetricRow: StatsMetricRow{TotalRuns: 2}}}, metricTotal: 1}
	res, appErr := NewService(repo).StatsByAgent(context.Background(), StatsListQuery{CurrentPage: 0, PageSize: 0, AgentID: &agentID})
	if appErr != nil {
		t.Fatalf("StatsByAgent returned error: %v", appErr)
	}
	if repo.metricQuery.CurrentPage != 1 || repo.metricQuery.PageSize != 20 || repo.metricQuery.AgentID == nil || *repo.metricQuery.AgentID != 5 || len(res.List) != 1 {
		t.Fatalf("unexpected stats list: query=%#v res=%#v", repo.metricQuery, res)
	}
}

func ptrUint(v uint) *uint { return &v }

func ptrString(v string) *string { return &v }
