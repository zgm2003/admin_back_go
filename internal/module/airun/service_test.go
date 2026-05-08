package airun

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/enum"
)

type fakeRepository struct {
	agents      []AgentOptionRow
	listQuery   ListQuery
	rows        []ListRow
	total       int64
	run         *RunDetailRow
	steps       []StepRow
	summary     StatsSummaryRow
	metricQuery StatsListQuery
	byDate      []StatsByDateRow
	byAgent     []StatsByAgentRow
	byUser      []StatsByUserRow
	metricTotal int64
}

func (f *fakeRepository) AgentOptions(ctx context.Context) ([]AgentOptionRow, error) {
	return f.agents, nil
}
func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	f.listQuery = query
	return f.rows, f.total, nil
}
func (f *fakeRepository) Detail(ctx context.Context, id int64) (*RunDetailRow, error) {
	return f.run, nil
}
func (f *fakeRepository) Steps(ctx context.Context, runID int64) ([]StepRow, error) {
	return f.steps, nil
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

func TestInitReturnsRunStatusAndAgentOptions(t *testing.T) {
	repo := &fakeRepository{agents: []AgentOptionRow{{ID: 3, Name: "客服"}}}
	res, appErr := NewService(repo).Init(context.Background())
	if appErr != nil {
		t.Fatalf("Init returned error: %v", appErr)
	}
	if len(res.Dict.RunStatusArr) == 0 || res.Dict.AgentArr[0].Value != 3 {
		t.Fatalf("unexpected init response: %#v", res)
	}
}

func TestListFiltersAndMapsLatency(t *testing.T) {
	created := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	status := enum.AIRunStatusSuccess
	repo := &fakeRepository{rows: []ListRow{{ID: 1, RequestID: "rid", UserID: 7, AgentID: 3, AgentName: "agent", ConversationID: 4, ConversationTitle: "chat", RunStatus: status, ModelSnapshot: "gpt", TotalTokens: ptrInt(12), LatencyMS: ptrInt(1530), CreatedAt: created}}, total: 1}
	res, appErr := NewService(repo).List(context.Background(), ListQuery{RunStatus: &status, RequestID: " rid ", CurrentPage: 0, PageSize: 0})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	if repo.listQuery.CurrentPage != 1 || repo.listQuery.PageSize != 20 || repo.listQuery.RequestID != "rid" {
		t.Fatalf("unexpected query: %#v", repo.listQuery)
	}
	if len(res.List) != 1 || res.List[0].LatencyStr != "1.53s" || res.List[0].RunStatusName == "" {
		t.Fatalf("unexpected list response: %#v", res)
	}
}

func TestDetailReturnsMessagesAndStepsOrderedByRepository(t *testing.T) {
	repo := &fakeRepository{run: &RunDetailRow{ID: 1, RequestID: "rid", UserID: 7, Username: "admin", AgentID: 3, AgentName: "agent", ConversationID: 4, ConversationTitle: "chat", RunStatus: enum.AIRunStatusSuccess, UserMessage: &MessageSummary{ID: 10, Content: "hi"}, AssistantMessage: &MessageSummary{ID: 11, Content: "ok"}}, steps: []StepRow{{ID: 2, StepNo: 1, StepType: enum.AIRunStepTypeLLM, Status: enum.AIRunStepStatusSuccess}}}
	res, appErr := NewService(repo).Detail(context.Background(), 1)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	if res.UserMessage == nil || res.AssistantMessage == nil || len(res.Steps) != 1 || res.Steps[0].StepNo != 1 {
		t.Fatalf("unexpected detail: %#v", res)
	}
}

func TestStatsSummaryComputesRatesAndTotals(t *testing.T) {
	repo := &fakeRepository{summary: StatsSummaryRow{TotalRuns: 10, SuccessRuns: 7, FailRuns: 2, TotalTokens: 100, PromptTokens: 40, CompletionTokens: 60, AvgLatencyMS: 1234}}
	res, appErr := NewService(repo).Stats(context.Background(), StatsFilter{})
	if appErr != nil {
		t.Fatalf("Stats returned error: %v", appErr)
	}
	if res.Summary.SuccessRate != 70 || res.Summary.TotalTokens != 100 || res.Summary.AvgLatencyMS != 1234 {
		t.Fatalf("unexpected stats: %#v", res)
	}
}

func TestStatsListsArePaginatedAndNormalized(t *testing.T) {
	repo := &fakeRepository{byDate: []StatsByDateRow{{Date: "2026-05-08", StatsMetricRow: StatsMetricRow{TotalRuns: 2}}}, metricTotal: 1}
	res, appErr := NewService(repo).StatsByDate(context.Background(), StatsListQuery{CurrentPage: 0, PageSize: 0})
	if appErr != nil {
		t.Fatalf("StatsByDate returned error: %v", appErr)
	}
	if repo.metricQuery.CurrentPage != 1 || repo.metricQuery.PageSize != 20 || len(res.List) != 1 || res.Page.Total != 1 {
		t.Fatalf("unexpected stats list: query=%#v res=%#v", repo.metricQuery, res)
	}
}

func ptrInt(v int) *int { return &v }
