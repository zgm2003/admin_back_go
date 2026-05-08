package airun

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/enum"
)

type fakeRepository struct {
	apps        []OptionRow
	engines     []OptionRow
	listQuery   ListQuery
	rows        []ListRow
	total       int64
	run         *RunDetailRow
	events      []EventRow
	summary     StatsSummaryRow
	metricQuery StatsListQuery
	byDate      []StatsByDateRow
	byAgent     []StatsByAgentRow
	byUser      []StatsByUserRow
	metricTotal int64
}

func (f *fakeRepository) AppOptions(ctx context.Context) ([]OptionRow, error) {
	return f.apps, nil
}
func (f *fakeRepository) EngineOptions(ctx context.Context) ([]OptionRow, error) {
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

func TestInitReturnsRunStatusAppAndEngineOptions(t *testing.T) {
	repo := &fakeRepository{apps: []OptionRow{{ID: 3, Name: "客服应用"}}, engines: []OptionRow{{ID: 2, Name: "Dify"}}}
	res, appErr := NewService(repo).Init(context.Background())
	if appErr != nil {
		t.Fatalf("Init returned error: %v", appErr)
	}
	if len(res.Dict.RunStatusArr) == 0 || res.Dict.AppArr[0].Value != 3 || res.Dict.AgentArr[0].Value != 3 || res.Dict.EngineArr[0].Value != 2 {
		t.Fatalf("unexpected init response: %#v", res)
	}
}

func TestListFiltersAndMapsLatency(t *testing.T) {
	created := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	status := enum.AIRunStatusSuccess
	appID := int64(3)
	repo := &fakeRepository{rows: []ListRow{{ID: 1, RequestID: "rid", UserID: 7, AppID: 3, AppName: "app", EngineConnectionID: 2, EngineName: "Dify", EngineType: "dify", ConversationID: 4, ConversationTitle: "chat", RunStatus: status, ModelSnapshot: "gpt", TotalTokens: ptrInt(12), LatencyMS: ptrInt(1530), CreatedAt: created}}, total: 1}
	res, appErr := NewService(repo).List(context.Background(), ListQuery{RunStatus: &status, RequestID: " rid ", AgentID: &appID, CurrentPage: 0, PageSize: 0})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	if repo.listQuery.CurrentPage != 1 || repo.listQuery.PageSize != 20 || repo.listQuery.RequestID != "rid" || repo.listQuery.AppID == nil || *repo.listQuery.AppID != 3 {
		t.Fatalf("unexpected query: %#v", repo.listQuery)
	}
	if len(res.List) != 1 || res.List[0].LatencyStr != "1.53s" || res.List[0].RunStatusName == "" || res.List[0].AppName != "app" || res.List[0].AgentName != "app" {
		t.Fatalf("unexpected list response: %#v", res)
	}
}

func TestDetailReturnsMessagesAndPersistedEvents(t *testing.T) {
	meta := `{"tenant":"admin"}`
	payload := `{"delta":"ok"}`
	repo := &fakeRepository{
		run:    &RunDetailRow{ID: 1, RequestID: "rid", UserID: 7, Username: "admin", AppID: 3, AppName: "app", EngineConnectionID: 2, EngineName: "Dify", EngineType: "dify", ConversationID: 4, ConversationTitle: "chat", RunStatus: enum.AIRunStatusSuccess, MetaJSON: &meta, UserMessage: &MessageSummary{ID: 10, Content: "hi"}, AssistantMessage: &MessageSummary{ID: 11, Content: "ok"}},
		events: []EventRow{{ID: 2, Seq: 1, EventID: "1-0", EventType: "ai.response.delta.v1", DeltaText: "ok", PayloadJSON: &payload}},
	}
	res, appErr := NewService(repo).Detail(context.Background(), 1)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	if res.UserMessage == nil || res.AssistantMessage == nil || len(res.Events) != 1 || res.Events[0].EventType != "ai.response.delta.v1" || len(res.Steps) != 0 || res.AppName != "app" {
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
	appID := int64(5)
	repo := &fakeRepository{byAgent: []StatsByAgentRow{{AppID: 5, AppName: "app", StatsMetricRow: StatsMetricRow{TotalRuns: 2}}}, metricTotal: 1}
	res, appErr := NewService(repo).StatsByAgent(context.Background(), StatsListQuery{CurrentPage: 0, PageSize: 0, AgentID: &appID})
	if appErr != nil {
		t.Fatalf("StatsByAgent returned error: %v", appErr)
	}
	if repo.metricQuery.CurrentPage != 1 || repo.metricQuery.PageSize != 20 || repo.metricQuery.AppID == nil || *repo.metricQuery.AppID != 5 || len(res.List) != 1 || res.List[0].AppName != "app" || res.List[0].AgentName != "app" {
		t.Fatalf("unexpected stats list: query=%#v res=%#v", repo.metricQuery, res)
	}
}

func ptrInt(v int) *int { return &v }
