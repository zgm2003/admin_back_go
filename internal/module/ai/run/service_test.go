package airun

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/module/ai/contextengine"
	"admin_back_go/internal/shared/clock"
	"admin_back_go/internal/shared/enum"
)

type fakeRepository struct {
	agents           []OptionRow
	engines          []OptionRow
	historicalModels []HistoricalModelRow
	historicalStart  time.Time
	historicalEnd    time.Time
	listQuery        ListQuery
	rows             []ListRow
	total            int64
	run              *RunDetailRow
	charge           *ChargeRow
	usageItems       []UsageChargeItemRow
	attempts         []ProviderAttemptRow
	billingRuns      []int64
	events           []EventRow
	toolCalls        []ToolCallRow
	dashboardQuery   DashboardQuery
	dashboardRows    DashboardRepositoryResult
	dashboardErr     error
	dashboardCalls   int
	contextPlan      *contextengine.ContextPlan
	contextPlanRuns  []int64
	contextPlanErr   error
}

func (f *fakeRepository) AgentOptions(ctx context.Context) ([]OptionRow, error) {
	return f.agents, nil
}
func (f *fakeRepository) ProviderOptions(ctx context.Context) ([]OptionRow, error) {
	return f.engines, nil
}
func (f *fakeRepository) HistoricalModelOptions(_ context.Context, startAt, endExclusive time.Time) ([]HistoricalModelRow, error) {
	f.historicalStart = startAt
	f.historicalEnd = endExclusive
	return f.historicalModels, nil
}
func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	f.listQuery = query
	return f.rows, f.total, nil
}
func (f *fakeRepository) Detail(ctx context.Context, id int64) (*RunDetailRow, error) {
	return f.run, nil
}
func (f *fakeRepository) BillingDetail(ctx context.Context, runID int64) (*ChargeRow, []UsageChargeItemRow, []ProviderAttemptRow, error) {
	f.billingRuns = append(f.billingRuns, runID)
	return f.charge, f.usageItems, f.attempts, nil
}
func (f *fakeRepository) Events(ctx context.Context, runID int64) ([]EventRow, error) {
	return f.events, nil
}
func (f *fakeRepository) ToolCalls(ctx context.Context, runID int64) ([]ToolCallRow, error) {
	return f.toolCalls, nil
}
func (f *fakeRepository) ContextPlan(_ context.Context, runID int64) (*contextengine.ContextPlan, error) {
	f.contextPlanRuns = append(f.contextPlanRuns, runID)
	return f.contextPlan, f.contextPlanErr
}
func (f *fakeRepository) Dashboard(_ context.Context, query DashboardQuery) (DashboardRepositoryResult, error) {
	f.dashboardCalls++
	f.dashboardQuery = query
	return f.dashboardRows, f.dashboardErr
}

func TestInitReturnsStatusAgentAndProviderOptions(t *testing.T) {
	repo := &fakeRepository{agents: []OptionRow{{ID: 3, Name: "客服智能体"}}, engines: []OptionRow{{ID: 2, Name: "OpenAI"}}}
	res, appErr := NewService(repo).PageInit(context.Background(), PageInitFilter{})
	if appErr != nil {
		t.Fatalf("PageInit returned error: %v", appErr)
	}
	if len(res.Dict.StatusArr) == 0 || res.Dict.AgentArr[0].Value != 3 || res.Dict.ProviderArr[0].Value != 2 {
		t.Fatalf("unexpected init response: %#v", res)
	}
	if len(res.Dict.PlatformArr) != 1 || res.Dict.PlatformArr[0].Value != enum.PlatformAdmin {
		t.Fatalf("AI run platform options must expose registered adapters only: %#v", res.Dict.PlatformArr)
	}
	encoded, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal page init: %v", err)
	}
	for _, key := range []string{"modality_arr", "source_type_arr", "usage_status_arr"} {
		if strings.Contains(string(encoded), key) {
			t.Fatalf("AI run page init leaked retired source field %s: %s", key, string(encoded))
		}
	}
}

func TestPageInitMergesOfficialCatalogAndHistoricalRunModels(t *testing.T) {
	repo := &fakeRepository{historicalModels: []HistoricalModelRow{
		{ModelID: "gpt-5.5", ModelDisplayName: "不得覆盖官方模型"},
		{ModelID: "retired-local-model", ModelDisplayName: "历史模型快照"},
	}}
	service := NewService(repo, WithClock(clock.Func(func() time.Time { return dashboardFixedNow(t) })))

	response, appErr := service.PageInit(context.Background(), PageInitFilter{})
	if appErr != nil {
		t.Fatalf("PageInit returned error: %v", appErr)
	}
	official, officialCount := findModelOptions(response.Dict.ModelArr, "gpt-5.5")
	if officialCount != 1 || official.Historical || official.Label != "gpt-5.5" {
		t.Fatalf("official option=%+v count=%d", official, officialCount)
	}
	historical, historicalCount := findModelOptions(response.Dict.ModelArr, "retired-local-model")
	if historicalCount != 1 || !historical.Historical || historical.Label != "历史模型快照" {
		t.Fatalf("historical option=%+v count=%d", historical, historicalCount)
	}
	if len(response.Dict.BillingStatusArr) != 5 || len(response.Dict.BillingReasonArr) != 10 {
		t.Fatalf("billing options are incomplete: statuses=%+v reasons=%+v", response.Dict.BillingStatusArr, response.Dict.BillingReasonArr)
	}
	assertDashboardTime(t, "page init default start", repo.historicalStart, "2026-07-23T00:00:00+08:00")
	assertDashboardTime(t, "page init default end", repo.historicalEnd, "2026-07-30T00:00:00+08:00")
}

func TestRunListAcceptsOutcomeUnknownAndDashboardDrilldownFilters(t *testing.T) {
	agentID, providerID, userID := int64(2), int64(3), int64(4)
	repository := &fakeRepository{}
	service := NewService(repository, WithClock(clock.Func(func() time.Time { return dashboardFixedNow(t) })))

	response, appErr := service.List(context.Background(), ListQuery{
		Status: "outcome_unknown", Platform: "admin", ModelID: " gpt-5.5 ", AgentID: &agentID,
		ProviderID: &providerID, UserID: &userID, BillingStatus: " settled ",
		BillingReason: " settled_complete_usage ", ErrorCode: " provider_timeout ", ToolCode: " lookup ",
		RunAnomaly: " stale_running ", BillingAnomaly: " state_inconsistent ",
		AnomalyAsOf: "2026-07-29T15:42:18+08:00", DateStart: "2026-07-28", DateEnd: "2026-07-29",
	})
	if appErr != nil || response == nil {
		t.Fatalf("List response=%#v error=%v", response, appErr)
	}
	query := repository.listQuery
	if query.Status != "outcome_unknown" || query.ModelID != "gpt-5.5" || query.BillingStatus != "settled" ||
		query.BillingReason != "settled_complete_usage" || query.ErrorCode != "provider_timeout" || query.ToolCode != "lookup" ||
		query.RunAnomaly != "stale_running" || query.BillingAnomaly != "state_inconsistent" {
		t.Fatalf("normalized drilldown query=%+v", query)
	}
	assertDashboardTime(t, "list start", query.StartAt, "2026-07-28T00:00:00+08:00")
	assertDashboardTime(t, "list end", query.EndExclusive, "2026-07-30T00:00:00+08:00")
	assertDashboardTime(t, "list anomaly as of", query.GeneratedAt, "2026-07-29T15:42:18+08:00")
	assertDashboardTime(t, "list stale before", query.StaleBefore, "2026-07-29T15:27:18+08:00")
}

func TestRunListReturnsBillingFactsAndFinalAttemptErrorCode(t *testing.T) {
	repository := &fakeRepository{rows: []ListRow{{
		ID: 9, Status: "failed", BillingStatus: "released", BillingReason: "released_provider_failed",
		ErrorCode: "upstream_unavailable",
	}}}

	response, appErr := NewService(repository).List(context.Background(), ListQuery{})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	if len(response.List) != 1 || response.List[0].BillingStatus != "released" ||
		response.List[0].BillingReason != "released_provider_failed" || response.List[0].ErrorCode != "upstream_unavailable" {
		t.Fatalf("list billing/error facts=%+v", response.List)
	}
}

func TestRunListNormalizesUserFeedbackAndReturnsLikedFacts(t *testing.T) {
	likedAt := time.Date(2026, 7, 30, 10, 11, 12, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	repository := &fakeRepository{rows: []ListRow{{ID: 9, LikedAt: &likedAt}}}

	response, appErr := NewService(repository).List(context.Background(), ListQuery{UserFeedback: " liked "})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	if repository.listQuery.UserFeedback != "liked" {
		t.Fatalf("normalized user feedback=%q", repository.listQuery.UserFeedback)
	}
	if len(response.List) != 1 || !response.List[0].Liked || response.List[0].LikedAt == nil || *response.List[0].LikedAt != "2026-07-30 10:11:12" {
		t.Fatalf("list feedback facts=%+v", response.List)
	}
}

func TestRunListRejectsUnknownUserFeedback(t *testing.T) {
	repository := &fakeRepository{}
	response, appErr := NewService(repository).List(context.Background(), ListQuery{UserFeedback: "thumbs_up"})
	if response != nil || appErr == nil || appErr.HTTPStatus != 400 {
		t.Fatalf("response=%#v error=%#v", response, appErr)
	}
	if repository.listQuery.UserFeedback != "" {
		t.Fatalf("invalid user feedback reached repository: %+v", repository.listQuery)
	}
}

func TestRunListUsesDashboardHalfOpenDateRangeForExactDrilldown(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository, WithClock(clock.Func(func() time.Time { return dashboardFixedNow(t) })))

	_, appErr := service.List(context.Background(), ListQuery{DateStart: "2026-07-28", DateEnd: "2026-07-29"})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	assertDashboardTime(t, "list half-open start", repository.listQuery.StartAt, "2026-07-28T00:00:00+08:00")
	assertDashboardTime(t, "list half-open end", repository.listQuery.EndExclusive, "2026-07-30T00:00:00+08:00")
	sql := renderRunListQuerySQL(t, repository.listQuery)
	assertDashboardSQLContains(t, sql, "r.created_at >= ?", "r.created_at < ?")
	if strings.Contains(sql, "r.created_at <= ?") {
		t.Fatalf("list end date must be exclusive, sql=%s", sql)
	}

	for _, filter := range []ListQuery{
		{DateStart: "2026-07-29"},
		{DateStart: "2026-07-30", DateEnd: "2026-07-29"},
		{DateStart: "2026-04-30", DateEnd: "2026-07-29"},
	} {
		if response, listErr := service.List(context.Background(), filter); response != nil || listErr == nil || listErr.HTTPStatus != 400 {
			t.Fatalf("invalid range response=%#v error=%#v filter=%+v", response, listErr, filter)
		}
	}
}

func findModelOptions(options []ModelOption, modelID string) (ModelOption, int) {
	var found ModelOption
	count := 0
	for _, option := range options {
		if option.Value == modelID {
			found = option
			count++
		}
	}
	return found, count
}

func TestListFiltersAndMapsDuration(t *testing.T) {
	created := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	status := enum.AIRunStatusSuccess
	agentID := int64(3)
	repo := &fakeRepository{rows: []ListRow{{ID: 1, RequestID: "rid", UserID: 7, AgentID: 3, AgentName: "agent", ProviderID: 2, ProviderName: "OpenAI", Platform: enum.PlatformAdmin, InputSnapshot: "hi", ConversationID: ptrInt64(4), ConversationTitle: "chat", Status: status, ModelID: "gpt-5.4", ModelDisplayName: "GPT-5.4", TotalTokens: 12, DurationMS: ptrUint(1530), CreatedAt: created}}, total: 1}
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

func TestListRedactsAttachmentStorageFactsFromInputSnapshot(t *testing.T) {
	snapshot := `{"content":"summarize","attachments":[{"type":"file","object_key":"ai_chat_attachments/private/report.pdf","mime_type":"application/pdf","url":"https://cos.example/private/report.pdf","name":"report.pdf","size":4096,"etag":"\"secret-v1\"","file_data":"data:application/pdf;base64,AAAA"}],"runtime_params":{"temperature":0.3}}`
	repo := &fakeRepository{rows: []ListRow{{ID: 1, InputSnapshot: snapshot}}}

	result, appErr := NewService(repo).List(context.Background(), ListQuery{})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"ai_chat_attachments/", "https://cos.example/", "secret-v1", "file_data", ";base64,"} {
		if strings.Contains(strings.ToLower(string(encoded)), strings.ToLower(forbidden)) {
			t.Fatalf("safe list leaked %q: %s", forbidden, encoded)
		}
	}
	for _, allowed := range []string{"summarize", "report.pdf", "application/pdf", "temperature"} {
		if !strings.Contains(string(encoded), allowed) {
			t.Fatalf("safe list dropped %q: %s", allowed, encoded)
		}
	}
}

func TestListRejectsUnregisteredPlatformFilter(t *testing.T) {
	repo := &fakeRepository{}
	_, appErr := NewService(repo).List(context.Background(), ListQuery{Platform: "partner_portal"})
	if appErr == nil || appErr.MessageID != "airun.platform.invalid" {
		t.Fatalf("expected unregistered platform filter to fail, got %#v", appErr)
	}
	if repo.listQuery.Platform != "" {
		t.Fatalf("unregistered platform filter reached repository: %#v", repo.listQuery)
	}
}

func TestDetailReturnsMessagesAndPersistedEvents(t *testing.T) {
	startedAt := time.Date(2026, 5, 10, 11, 18, 14, 0, time.UTC)
	repo := &fakeRepository{
		run:       &RunDetailRow{ID: 1, RequestID: "rid", UserID: 7, Username: "admin", AgentID: 3, AgentName: "agent", ProviderID: 2, ProviderName: "OpenAI", Platform: enum.PlatformAdmin, InputSnapshot: "hi", ConversationID: ptrInt64(4), ConversationTitle: "chat", Status: enum.AIRunStatusSuccess, ModelID: "gpt-5.4", StartedAt: &startedAt, UserMessage: &MessageSummary{ID: 10, Content: "hi"}, AssistantMessage: &MessageSummary{ID: 11, Content: "ok"}},
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

func TestDetailResponsePublishesContractRequiredErrorCode(t *testing.T) {
	row := &RunDetailRow{ID: 1, ErrorCode: "provider_timeout"}
	response, appErr := NewService(&fakeRepository{run: row}).Detail(context.Background(), 1)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal detail response: %v", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if got := string(payload["error_code"]); got != `"provider_timeout"` {
		t.Fatalf("detail response error_code=%s, want provider_timeout; payload=%s", got, encoded)
	}
}

func TestDetailResponsePublishesDiagnosticCodesAsArray(t *testing.T) {
	response, appErr := NewService(&fakeRepository{run: &RunDetailRow{ID: 1}}).Detail(context.Background(), 1)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal detail response: %v", err)
	}
	if !bytes.Contains(encoded, []byte(`"diagnostic_codes":[]`)) {
		t.Fatalf("detail response must publish diagnostic_codes as an array: %s", encoded)
	}
}

func TestRunDetailBuildsLatencyBreakdownFromDurableTimeline(t *testing.T) {
	received := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	accepted := received.Add(20 * time.Millisecond)
	claimed := accepted.Add(80 * time.Millisecond)
	prepare := claimed.Add(10 * time.Millisecond)
	dispatched := prepare.Add(40 * time.Millisecond)
	firstDelta := dispatched.Add(350 * time.Millisecond)
	providerFinished := dispatched.Add(900 * time.Millisecond)
	settled := providerFinished.Add(30 * time.Millisecond)
	repo := &fakeRepository{
		run: &RunDetailRow{
			ID: 81, BillingStatus: string(billing.BillingStatusHeld), BillingReason: string(billing.BillingReasonHeld), PricingSnapshotJSON: paidPricingSnapshotJSON(),
			RequestReceivedAt: &received, AcceptedAt: &accepted, ClaimedAt: &claimed, ClaimSource: "wake", SettledAt: &settled,
		},
		charge:   &ChargeRow{ID: 14, HeldUnits: 1, Status: string(billing.ChargeStatusOpen)},
		attempts: []ProviderAttemptRow{{ID: 200, AttemptNo: 1, State: string(billing.AttemptStateSucceeded), UsageStatus: string(billing.UsageStatusUnavailable), PrepareStartedAt: &prepare, DispatchedAt: &dispatched, FirstDeltaAt: &firstDelta, FinishedAt: &providerFinished}},
	}

	result, appErr := NewService(repo).Detail(context.Background(), 81)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	want := LatencyBreakdown{AcceptMS: int64Pointer(20), QueueMS: int64Pointer(80), PrepareMS: int64Pointer(40), TTFTMS: int64Pointer(350), ProviderTotalMS: int64Pointer(900), SettlementMS: int64Pointer(30), EndToEndMS: int64Pointer(1080), ClaimSource: "wake"}
	if !reflect.DeepEqual(result.Latency, want) {
		t.Fatalf("latency=%+v want=%+v", result.Latency, want)
	}
}

func TestRunDetailAggregatesDurableFileLatencyAndFiltersInternalEvents(t *testing.T) {
	received := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	settled := received.Add(time.Second)
	repo := &fakeRepository{
		run: &RunDetailRow{
			ID: 83, BillingStatus: string(billing.BillingStatusHeld), BillingReason: string(billing.BillingReasonHeld), PricingSnapshotJSON: paidPricingSnapshotJSON(),
			RequestReceivedAt: &received, SettledAt: &settled,
		},
		charge: &ChargeRow{ID: 16, HeldUnits: 1, Status: string(billing.ChargeStatusOpen)},
		events: []EventRow{
			{ID: 21, Seq: 1, EventType: enum.AIRunEventFileMaterialized, Message: `{"cos_head_ms":12,"cos_stream_ms":34,"materialized_request_bytes":4096}`, CreatedAt: received.Add(300 * time.Millisecond)},
			{ID: 22, Seq: 2, EventType: enum.AIRunEventFileMaterialized, Message: `{"cos_head_ms":5,"cos_stream_ms":7,"materialized_request_bytes":2048}`, CreatedAt: received.Add(600 * time.Millisecond)},
			{ID: 23, Seq: 3, EventType: enum.AIRunEventCompleted, Message: "生成完成", CreatedAt: settled},
		},
	}

	result, appErr := NewService(repo).Detail(context.Background(), 83)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	if result.Latency.COSHeadMS == nil || *result.Latency.COSHeadMS != 17 || result.Latency.COSStreamMS == nil || *result.Latency.COSStreamMS != 41 {
		t.Fatalf("file latency=%+v", result.Latency)
	}
	if len(result.Events) != 1 || result.Events[0].EventType != enum.AIRunEventCompleted {
		t.Fatalf("internal file metrics events leaked: %#v", result.Events)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "materialized_request_bytes\\\":4096") || strings.Contains(string(encoded), enum.AIRunEventFileMaterialized) {
		t.Fatalf("internal metrics payload leaked: %s", encoded)
	}
}

func TestRunDetailIgnoresInvalidDurableFileLatencyAndLogsStructureError(t *testing.T) {
	received := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	settled := received.Add(time.Second)
	var logs bytes.Buffer
	repo := &fakeRepository{
		run: &RunDetailRow{
			ID: 84, BillingStatus: string(billing.BillingStatusHeld), BillingReason: string(billing.BillingReasonHeld), PricingSnapshotJSON: paidPricingSnapshotJSON(),
			RequestReceivedAt: &received, SettledAt: &settled,
		},
		charge: &ChargeRow{ID: 17, HeldUnits: 1, Status: string(billing.ChargeStatusOpen)},
		events: []EventRow{
			{ID: 31, Seq: 1, EventType: enum.AIRunEventFileMaterialized, Message: `{"cos_head_ms":1,"cos_stream_ms":2}`, CreatedAt: received.Add(100 * time.Millisecond)},
			{ID: 32, Seq: 2, EventType: enum.AIRunEventFileMaterialized, Message: `{"cos_head_ms":-1,"cos_stream_ms":2,"materialized_request_bytes":10}`, CreatedAt: received.Add(200 * time.Millisecond)},
			{ID: 33, Seq: 3, EventType: enum.AIRunEventFileMaterialized, Message: `{"cos_head_ms":600,"cos_stream_ms":500,"materialized_request_bytes":10}`, CreatedAt: received.Add(300 * time.Millisecond)},
			{ID: 34, Seq: 4, EventType: enum.AIRunEventFileMaterialized, Message: `{"cos_head_ms":1,"cos_stream_ms":2,"materialized_request_bytes":10,"object_key":"ai_chat_attachments/private.pdf"}`, CreatedAt: received.Add(400 * time.Millisecond)},
		},
	}
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	result, appErr := NewService(repo, WithLogger(logger)).Detail(context.Background(), 84)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	if result.Latency.COSHeadMS != nil || result.Latency.COSStreamMS != nil {
		t.Fatalf("invalid file latency must not be returned: %+v", result.Latency)
	}
	if len(result.Events) != 0 {
		t.Fatalf("invalid internal events leaked: %#v", result.Events)
	}
	if got := logs.String(); strings.Count(got, "invalid durable AI file materialization metrics") != 4 || strings.Contains(got, "ai_chat_attachments/private.pdf") || strings.Contains(got, "object_key") {
		t.Fatalf("unexpected safe structure logs: %s", got)
	}
}

func TestRunDetailIgnoresOverflowingFileLatencyWithoutDiscardingPriorMetrics(t *testing.T) {
	var logs bytes.Buffer
	repo := &fakeRepository{
		run:    &RunDetailRow{ID: 88, BillingStatus: string(billing.BillingStatusHeld), BillingReason: string(billing.BillingReasonHeld), PricingSnapshotJSON: paidPricingSnapshotJSON()},
		charge: &ChargeRow{ID: 21, HeldUnits: 1, Status: string(billing.ChargeStatusOpen)},
		events: []EventRow{
			{ID: 41, Seq: 1, EventType: enum.AIRunEventFileMaterialized, Message: `{"cos_head_ms":9223372036854775807,"cos_stream_ms":0,"materialized_request_bytes":1}`},
			{ID: 42, Seq: 2, EventType: enum.AIRunEventFileMaterialized, Message: `{"cos_head_ms":1,"cos_stream_ms":0,"materialized_request_bytes":1}`},
		},
	}
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	result, appErr := NewService(repo, WithLogger(logger)).Detail(context.Background(), 88)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	if result.Latency.COSHeadMS == nil || *result.Latency.COSHeadMS != int64(math.MaxInt64) || result.Latency.COSStreamMS == nil || *result.Latency.COSStreamMS != 0 {
		t.Fatalf("valid prior metrics were discarded: %+v", result.Latency)
	}
	if strings.Count(logs.String(), "invalid durable AI file materialization metrics") != 1 {
		t.Fatalf("overflow event was not logged once: %s", logs.String())
	}
}

func TestRunDetailReturnsSafePreparedRequestSummaryOnly(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 1, 0, 0, time.UTC)
	prepared := `{"model":"gpt-test","messages":[{"role":"user","content":"private prompt"}],"authorization":"secret"}`
	repo := &fakeRepository{
		run:       &RunDetailRow{ID: 82, Status: enum.AIRunStatusRunning, BillingStatus: string(billing.BillingStatusHeld), BillingReason: string(billing.BillingReasonHeld), PricingSnapshotJSON: paidPricingSnapshotJSON(), CreatedAt: now, UpdatedAt: now},
		charge:    &ChargeRow{ID: 15, HeldUnits: 1, Status: string(billing.ChargeStatusOpen)},
		attempts:  []ProviderAttemptRow{{ID: 201, AttemptNo: 1, State: string(billing.AttemptStatePrepared), UsageStatus: string(billing.UsageStatusUnavailable), UsageJSON: `{"status":"unavailable"}`, PreparedRequestJSON: prepared}},
		toolCalls: []ToolCallRow{{ID: 1}},
	}

	result, appErr := NewService(repo).Detail(context.Background(), 82)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	if result.RequestSummary.ProviderAttemptCount != 1 || result.RequestSummary.ToolCallCount != 1 || result.RequestSummary.PreparedRequestBytes != len(prepared) || result.RequestSummary.MessageCount == nil || *result.RequestSummary.MessageCount != 1 {
		t.Fatalf("request summary=%+v", result.RequestSummary)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private prompt", "authorization", `"prepared_request_json":`, "secret"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("safe detail leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestRunDetailCountsPersistedInlineImageAttachments(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 1, 0, 0, time.UTC)
	prepared := `{"model":"gpt-test","messages":[{"role":"system","content":"rules"},{"role":"user","content":[{"type":"text","text":"look"},{"type":"image_url","image_url":{"url":"https://images.example/a.png"}},{"type":"image_url","image_url":{"url":"https://images.example/b.png"}}]}]}`
	repo := &fakeRepository{
		run:      &RunDetailRow{ID: 86, Status: enum.AIRunStatusRunning, BillingStatus: string(billing.BillingStatusHeld), BillingReason: string(billing.BillingReasonHeld), PricingSnapshotJSON: paidPricingSnapshotJSON(), CreatedAt: now, UpdatedAt: now},
		charge:   &ChargeRow{ID: 19, HeldUnits: 1, Status: string(billing.ChargeStatusOpen)},
		attempts: []ProviderAttemptRow{{ID: 203, AttemptNo: 1, State: string(billing.AttemptStatePrepared), UsageStatus: string(billing.UsageStatusUnavailable), UsageJSON: `{"status":"unavailable"}`, PreparedRequestJSON: prepared}},
	}

	result, appErr := NewService(repo).Detail(context.Background(), 86)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	if result.RequestSummary.MessageCount == nil || *result.RequestSummary.MessageCount != 2 || result.RequestSummary.AttachmentCount != 2 || result.RequestSummary.NativeFileCount != 0 || result.RequestSummary.NativeFileBytes != 0 || result.RequestSummary.PreparedManifestBytes != 0 || result.RequestSummary.MaterializedRequestBytes != 0 || result.RequestSummary.APIProtocol != infraai.APIProtocolChatCompletions {
		t.Fatalf("inline request summary=%+v", result.RequestSummary)
	}
}

func TestRunDetailSummarizesResponsesInlineEnvelope(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 1, 0, 0, time.UTC)
	request := json.RawMessage(`{"model":"gpt-5.6","input":[{"role":"user","content":[{"type":"input_text","text":"private prompt"},{"type":"input_image","image_url":"https://images.example/private.png"}]}],"stream":true,"store":false}`)
	prepared, err := infraai.MarshalPreparedChatInlineEnvelope(infraai.PreparedChatInlineEnvelope{
		Schema: infraai.PreparedChatSchemaResponsesInlineV1, APIProtocol: infraai.APIProtocolResponses, Request: request,
	})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeRepository{
		run: &RunDetailRow{
			ID: 90, Status: enum.AIRunStatusRunning, BillingStatus: string(billing.BillingStatusHeld),
			BillingReason: string(billing.BillingReasonHeld), PricingSnapshotJSON: paidPricingSnapshotJSON(),
			CreatedAt: now, UpdatedAt: now,
		},
		charge:   &ChargeRow{ID: 23, HeldUnits: 1, Status: string(billing.ChargeStatusOpen)},
		attempts: []ProviderAttemptRow{{ID: 207, AttemptNo: 1, State: string(billing.AttemptStatePrepared), UsageStatus: string(billing.UsageStatusUnavailable), UsageJSON: `{"status":"unavailable"}`, PreparedRequestJSON: string(prepared)}},
	}

	result, appErr := NewService(repo).Detail(context.Background(), 90)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	if result.RequestSummary.MessageCount == nil || *result.RequestSummary.MessageCount != 1 ||
		result.RequestSummary.AttachmentCount != 1 || result.RequestSummary.APIProtocol != infraai.APIProtocolResponses {
		t.Fatalf("Responses inline summary=%+v", result.RequestSummary)
	}
}

func TestRunDetailReturnsZeroSummaryForInvalidPersistedSchemaWithoutLoggingRequest(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 1, 0, 0, time.UTC)
	prepared := `{"schema":"unknown_private_schema","object_key":"ai_chat_attachments/private.pdf"}`
	var logs bytes.Buffer
	repo := &fakeRepository{
		run:       &RunDetailRow{ID: 87, Status: enum.AIRunStatusFailed, ErrorCode: "ai.run.structure_invalid", BillingStatus: string(billing.BillingStatusHeld), BillingReason: string(billing.BillingReasonHeld), PricingSnapshotJSON: paidPricingSnapshotJSON(), CreatedAt: now, UpdatedAt: now},
		charge:    &ChargeRow{ID: 20, HeldUnits: 1, Status: string(billing.ChargeStatusOpen)},
		attempts:  []ProviderAttemptRow{{ID: 204, AttemptNo: 1, State: string(billing.AttemptStatePrepared), UsageStatus: string(billing.UsageStatusUnavailable), UsageJSON: `{"status":"unavailable"}`, PreparedRequestJSON: prepared}},
		toolCalls: []ToolCallRow{{ID: 1}},
	}
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	result, appErr := NewService(repo, WithLogger(logger)).Detail(context.Background(), 87)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	if !reflect.DeepEqual(result.RequestSummary, SafeRequestSummary{}) {
		t.Fatalf("invalid persisted request summary must be zero: %+v", result.RequestSummary)
	}
	if got := logs.String(); !strings.Contains(got, "invalid persisted AI prepared request summary") || strings.Contains(got, "ai_chat_attachments/private.pdf") || strings.Contains(got, "unknown_private_schema") {
		t.Fatalf("unexpected safe structure log: %s", got)
	}
}

func TestSafeRequestSummaryNeverContainsObjectIdentityOrManifest(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 1, 0, 0, time.UTC)
	request := json.RawMessage(`{"model":"gpt-test","messages":[{"role":"user","content":[{"type":"text","text":"private prompt"},{"type":"image_url","image_url":{"url":"https://images.example/private.png"}},{"type":"file_ref","ref":"file-1"},{"type":"file_ref","ref":"file-2"}]}],"stream":true}`)
	manifest := infraai.PreparedChatFileManifest{
		Schema:        infraai.PreparedChatSchemaFileManifestV1,
		FileInputMode: "chat_completions",
		Request:       request,
		Files: []infraai.PreparedFileRef{
			{Ref: "file-1", ObjectKey: "ai_chat_attachments/secret/report.pdf", ETag: `"report-v1"`, Size: 4, MIMEType: "application/pdf", Filename: "report.pdf"},
			{Ref: "file-2", ObjectKey: "ai_chat_attachments/secret/notes.txt", ETag: `"notes-v1"`, Size: 2, MIMEType: "text/plain", Filename: "notes.txt"},
		},
	}
	prepared, err := infraai.MarshalPreparedChatFileManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	materialized := `{"model":"gpt-test","messages":[{"role":"user","content":[{"type":"text","text":"private prompt"},{"type":"image_url","image_url":{"url":"https://images.example/private.png"}},{"type":"file","file":{"filename":"report.pdf","file_data":"data:application/pdf;base64,AAAAAAAA"}},{"type":"file","file":{"filename":"notes.txt","file_data":"data:text/plain;base64,AAAA"}}]}],"stream":true}`
	repo := &fakeRepository{
		run: &RunDetailRow{
			ID: 85, Status: enum.AIRunStatusRunning, BillingStatus: string(billing.BillingStatusHeld), BillingReason: string(billing.BillingReasonHeld), PricingSnapshotJSON: paidPricingSnapshotJSON(), CreatedAt: now, UpdatedAt: now,
			InputSnapshot: `{"content":"summarize safely","attachments":[{"type":"file","object_key":"ai_chat_attachments/private/input.pdf","mime_type":"application/pdf","url":"https://cos.example/private/input.pdf","name":"input.pdf","size":4096,"etag":"\"input-v1\"","file_data":"data:application/pdf;base64,AAAA"}],"runtime_params":{"temperature":0.3},"meta_json":"{\"attachments\":[{\"type\":\"file\",\"object_key\":\"ai_chat_attachments/private/nested.pdf\",\"mime_type\":\"application/pdf\",\"url\":\"https://cos.example/private/nested.pdf\",\"name\":\"nested.pdf\",\"size\":2048,\"etag\":\"nested-v1\"}]}"}`,
			UserMessage:   &MessageSummary{ID: 301, Content: "summarize safely", MetaJSON: json.RawMessage(`{"attachments":[{"type":"file","object_key":"ai_chat_attachments/private/message.pdf","mime_type":"application/pdf","url":"https://cos.example/private/message.pdf","name":"message.pdf","size":1024,"etag":"message-v1","file_data":"data:application/pdf;base64,BBBB"}],"runtime_params":{"temperature":0.3}}`)},
		},
		charge:    &ChargeRow{ID: 18, HeldUnits: 1, Status: string(billing.ChargeStatusOpen)},
		attempts:  []ProviderAttemptRow{{ID: 202, AttemptNo: 1, State: string(billing.AttemptStatePrepared), UsageStatus: string(billing.UsageStatusUnavailable), UsageJSON: `{"status":"unavailable"}`, PreparedRequestJSON: string(prepared)}},
		toolCalls: []ToolCallRow{{ID: 1}},
	}

	result, appErr := NewService(repo).Detail(context.Background(), 85)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	messageCount := 1
	want := SafeRequestSummary{
		ProviderAttemptCount: 1, ToolCallCount: 1, PreparedRequestBytes: len(prepared), MessageCount: &messageCount,
		AttachmentCount: 3, NativeFileCount: 2, NativeFileBytes: 6, PreparedManifestBytes: len(prepared),
		MaterializedRequestBytes: int64(len(materialized)), APIProtocol: "chat_completions",
	}
	if !reflect.DeepEqual(result.RequestSummary, want) {
		t.Fatalf("request summary=%+v want=%+v", result.RequestSummary, want)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(encoded))
	for _, forbidden := range []string{
		"ai_chat_attachments/", "https://images.example/private.png", "report-v1", "notes-v1", "report.pdf", "notes.txt",
		"https://cos.example/", "input-v1", "nested-v1", "message-v1",
		`"schema":"openai_chat_file_manifest_v1"`, "file_ref", "file_data", ";base64,", "private prompt", "object_key",
	} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("safe detail leaked %q: %s", forbidden, encoded)
		}
	}
	for _, allowed := range []string{"summarize safely", "input.pdf", "nested.pdf", "message.pdf", "application/pdf", "temperature"} {
		if !strings.Contains(string(encoded), allowed) {
			t.Fatalf("safe detail dropped %q: %s", allowed, encoded)
		}
	}
}

func TestRunDetailSummarizesResponsesFileManifest(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 1, 0, 0, time.UTC)
	request := json.RawMessage(`{"model":"gpt-5.6","input":[{"role":"user","content":[{"type":"input_text","text":"private prompt"},{"type":"input_image","image_url":"https://images.example/private.png"},{"type":"file_ref","ref":"file-1"}]}],"stream":true,"store":false}`)
	manifest := infraai.PreparedChatFileManifest{
		Schema:      infraai.PreparedChatSchemaResponsesFileManifestV1,
		APIProtocol: infraai.APIProtocolResponses,
		Request:     request,
		Files: []infraai.PreparedFileRef{{
			Ref: "file-1", ObjectKey: "ai_chat_attachments/private/report.pdf", ETag: `"report-v1"`,
			Size: 4, MIMEType: "application/pdf", Filename: "report.pdf",
		}},
	}
	prepared, err := infraai.MarshalPreparedChatFileManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	materialized := strings.Replace(
		string(request),
		`{"type":"file_ref","ref":"file-1"}`,
		`{"type":"input_file","filename":"report.pdf","file_data":"data:application/pdf;base64,AQIDBA=="}`,
		1,
	)
	repo := &fakeRepository{
		run: &RunDetailRow{
			ID: 89, Status: enum.AIRunStatusRunning, BillingStatus: string(billing.BillingStatusHeld),
			BillingReason: string(billing.BillingReasonHeld), PricingSnapshotJSON: paidPricingSnapshotJSON(),
			CreatedAt: now, UpdatedAt: now,
		},
		charge:   &ChargeRow{ID: 22, HeldUnits: 1, Status: string(billing.ChargeStatusOpen)},
		attempts: []ProviderAttemptRow{{ID: 206, AttemptNo: 1, State: string(billing.AttemptStatePrepared), UsageStatus: string(billing.UsageStatusUnavailable), UsageJSON: `{"status":"unavailable"}`, PreparedRequestJSON: string(prepared)}},
	}

	result, appErr := NewService(repo).Detail(context.Background(), 89)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	messageCount := 1
	want := SafeRequestSummary{
		ProviderAttemptCount: 1, PreparedRequestBytes: len(prepared), MessageCount: &messageCount,
		AttachmentCount: 2, NativeFileCount: 1, NativeFileBytes: 4, PreparedManifestBytes: len(prepared),
		MaterializedRequestBytes: int64(len(materialized)), APIProtocol: infraai.APIProtocolResponses,
	}
	if !reflect.DeepEqual(result.RequestSummary, want) {
		t.Fatalf("Responses request summary=%+v want=%+v", result.RequestSummary, want)
	}
}

func TestUnsafeRunProjectionRejectsAllPreparedFileManifestSchemas(t *testing.T) {
	for _, schema := range []string{
		infraai.PreparedChatSchemaFileManifestV1,
		infraai.PreparedChatSchemaResponsesFileManifestV1,
	} {
		if !containsUnsafeRunProjectionLiteral(`{"schema":"` + schema + `"}`) {
			t.Fatalf("prepared file manifest schema %q was not redacted", schema)
		}
	}
}

func TestDetailProjectsLikedFeedback(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 1, 2, 0, time.UTC)
	repository := &fakeRepository{run: &RunDetailRow{
		ID: 44, LikedAt: &now,
		BillingStatus: string(billing.BillingStatusUnbilled), BillingReason: string(billing.BillingReasonLegacyUnpriced),
	}}
	response, appErr := NewService(repository).Detail(context.Background(), 44)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	if !response.Liked || response.LikedAt == nil || *response.LikedAt != "2026-07-27 12:01:02" {
		t.Fatalf("liked detail projection missing: %#v", response)
	}
	repository.run.LikedAt = nil
	response, appErr = NewService(repository).Detail(context.Background(), 44)
	if appErr != nil {
		t.Fatalf("unliked Detail returned error: %v", appErr)
	}
	if response.Liked || response.LikedAt != nil {
		t.Fatalf("unliked detail projection must be false/null: %#v", response)
	}
}

func TestDetailPublishesPaidBillingEvidenceFromRunSnapshot(t *testing.T) {
	now := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		run: &RunDetailRow{
			ID: 44, RequestID: "paid-run", UserID: 7, Status: enum.AIRunStatusFailed,
			BillingStatus: string(billing.BillingStatusSettled), BillingReason: string(billing.BillingReasonSettledCompleteUsage),
			PricingSnapshotJSON: paidPricingSnapshotJSON(), CreatedAt: now, UpdatedAt: now,
		},
		charge: &ChargeRow{ID: 9, HeldUnits: 900000000, ActualUnits: 250000000, Status: string(billing.ChargeStatusSettled)},
		usageItems: []UsageChargeItemRow{
			{AttemptID: 101, AttemptNo: 1, AttemptState: string(billing.AttemptStateSucceeded), Category: "input", Quantity: 2, Unit: "token", UnitPriceUnits: 100000000, UnitScale: 1, AmountUnits: 250000000},
		},
		attempts: []ProviderAttemptRow{
			{ID: 101, AttemptNo: 1, State: string(billing.AttemptStateSucceeded), ProviderRequestID: "provider-ok", UsageStatus: string(billing.UsageStatusComplete), UsageJSON: `{"status":"complete","items":[{"category":"input","unit":"token","quantity":2}]}`},
			{ID: 102, AttemptNo: 2, State: string(billing.AttemptStateFailed), ProviderRequestID: "provider-failed", UsageStatus: string(billing.UsageStatusComplete), UsageJSON: `{"status":"complete","items":[{"category":"output","unit":"token","quantity":1}]}`},
		},
	}

	res, appErr := NewService(repo).Detail(context.Background(), 44)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	if len(repo.billingRuns) != 1 || repo.billingRuns[0] != 44 {
		t.Fatalf("billing evidence query calls=%v", repo.billingRuns)
	}
	if res.BillingStatus != "settled" || res.BillingReason != "settled_complete_usage" || res.HeldAmount != "9" || res.ActualAmount != "2.5" {
		t.Fatalf("unexpected billing summary: %#v", res)
	}
	if res.Pricing == nil || res.Pricing.Version != "catalog-v1" || res.Pricing.CatalogVendor != "vendor-a" || res.Pricing.TransportEngine != "openai" || res.Pricing.ModelID != "canonical-model" || res.Pricing.ResolvedAlias != "requested-alias" || res.Pricing.BillingMultiplier != "1.25" || res.Pricing.MaxOutputTokens != 4096 {
		t.Fatalf("unexpected pricing snapshot: %#v", res.Pricing)
	}
	if len(res.Pricing.Rates) != 2 || res.Pricing.Rates[0].Price != "1" || res.Pricing.Rates[1].Price != "2" {
		t.Fatalf("unexpected pricing rates: %#v", res.Pricing.Rates)
	}
	if len(res.ProviderAttempts) != 2 || res.ProviderAttempts[0].ProviderRequestID == nil || *res.ProviderAttempts[0].ProviderRequestID != "provider-ok" {
		t.Fatalf("unexpected provider attempts: %#v", res.ProviderAttempts)
	}
	if len(res.UsageItems) != 2 || !res.UsageItems[0].Billable || res.UsageItems[0].Amount != "2.5" || res.UsageItems[1].Billable || res.UsageItems[1].AttemptNo != 2 || res.UsageItems[1].Amount != "2.5" {
		t.Fatalf("unexpected usage items: %#v", res.UsageItems)
	}
	encoded, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"prepared_request":`, `"prepared_request_json":`, `"quote_json":`, `"provider_engine":`, `"api_key":`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("detail leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestDetailTreatsStoppedCanceledAttemptUsageAsBillable(t *testing.T) {
	now := time.Date(2026, 7, 30, 21, 26, 7, 0, time.Local)
	repo := &fakeRepository{
		run: &RunDetailRow{
			ID: 6, RequestID: "stopped-run", UserID: 1, Status: enum.AIRunStatusCanceled,
			BillingStatus: string(billing.BillingStatusSettled), BillingReason: string(billing.BillingReasonSettledCompleteUsage),
			PricingSnapshotJSON: paidPricingSnapshotJSON(), CreatedAt: now, UpdatedAt: now,
		},
		charge: &ChargeRow{ID: 6, HeldUnits: 581401250, ActualUnits: 4956000, Status: string(billing.ChargeStatusSettled)},
		usageItems: []UsageChargeItemRow{
			{AttemptID: 7, AttemptNo: 1, AttemptState: string(billing.AttemptStateCanceled), Category: "input", TierKey: "short_context", Quantity: 1530, Unit: "token", UnitPriceUnits: 500000000, UnitScale: 1000000, AmountUnits: 765000},
			{AttemptID: 7, AttemptNo: 1, AttemptState: string(billing.AttemptStateCanceled), Category: "output", TierKey: "short_context", Quantity: 1333, Unit: "token", UnitPriceUnits: 3000000000, UnitScale: 1000000, AmountUnits: 3999000},
			{AttemptID: 7, AttemptNo: 1, AttemptState: string(billing.AttemptStateCanceled), Category: "cache_read", TierKey: "short_context", Quantity: 3840, Unit: "token", UnitPriceUnits: 50000000, UnitScale: 1000000, AmountUnits: 192000},
		},
		attempts: []ProviderAttemptRow{
			{ID: 7, AttemptNo: 1, State: string(billing.AttemptStateCanceled), ProviderRequestID: "provider-stopped", UsageStatus: string(billing.UsageStatusComplete)},
		},
	}

	res, appErr := NewService(repo).Detail(context.Background(), 6)
	if appErr != nil {
		t.Fatalf("stopped run detail returned error: %v", appErr)
	}
	if res.ActualAmount != "0.04956" || len(res.UsageItems) != 3 {
		t.Fatalf("unexpected stopped billing detail: %#v", res)
	}
	for _, item := range res.UsageItems {
		if !item.Billable {
			t.Fatalf("settled stopped usage must be billable: %#v", res.UsageItems)
		}
	}
}

func TestDetailMapsLegacyRunWithoutChargeWithoutParsingMarker(t *testing.T) {
	repo := &fakeRepository{run: &RunDetailRow{
		ID: 45, BillingStatus: string(billing.BillingStatusUnbilled), BillingReason: string(billing.BillingReasonLegacyUnpriced),
		PricingSnapshotJSON: `{"version":"legacy_unpriced_v1","billable":false}`,
	}}
	res, appErr := NewService(repo).Detail(context.Background(), 45)
	if appErr != nil {
		t.Fatalf("legacy detail returned error: %v", appErr)
	}
	if res.BillingStatus != "unbilled" || res.BillingReason != "legacy_unpriced" || res.Pricing != nil || res.HeldAmount != "0" || res.ActualAmount != "0" || len(res.UsageItems) != 0 || len(res.ProviderAttempts) != 0 {
		t.Fatalf("legacy billing detail=%#v", res)
	}
}

func TestDetailRejectsCurrentPaidRunWithoutCharge(t *testing.T) {
	repo := &fakeRepository{
		run: &RunDetailRow{
			ID: 48, BillingStatus: string(billing.BillingStatusPending), BillingReason: string(billing.BillingReasonPending),
			PricingSnapshotJSON: paidPricingSnapshotJSON(),
		},
		attempts: []ProviderAttemptRow{{ID: 201, AttemptNo: 1, State: string(billing.AttemptStatePrepared), UsageStatus: string(billing.UsageStatusUnavailable)}},
	}
	if _, appErr := NewService(repo).Detail(context.Background(), 48); appErr == nil || appErr.HTTPStatus != 500 {
		t.Fatalf("paid Run without Charge must fail closed, got %#v", appErr)
	}
}

func TestDetailRejectsLegacyRunWithPaidEvidence(t *testing.T) {
	repo := &fakeRepository{
		run:    &RunDetailRow{ID: 50, BillingStatus: string(billing.BillingStatusUnbilled), BillingReason: string(billing.BillingReasonLegacyUnpriced)},
		charge: &ChargeRow{ID: 13, Status: string(billing.ChargeStatusUnbilled)},
	}
	if _, appErr := NewService(repo).Detail(context.Background(), 50); appErr == nil || appErr.HTTPStatus != 500 {
		t.Fatalf("legacy Run with paid evidence must fail closed, got %#v", appErr)
	}
}

func TestDetailRejectsInvalidPaidSnapshotAndMismatchedSettlement(t *testing.T) {
	for _, test := range []struct {
		name     string
		run      RunDetailRow
		charge   ChargeRow
		items    []UsageChargeItemRow
		attempts []ProviderAttemptRow
	}{
		{name: "invalid snapshot", run: RunDetailRow{ID: 46, BillingStatus: "settled", BillingReason: "settled_complete_usage", PricingSnapshotJSON: `{"version":"attempt-quote"}`}, charge: ChargeRow{ID: 10, ActualUnits: 1, Status: "settled"}},
		{name: "charge status", run: RunDetailRow{ID: 51, BillingStatus: "settled", BillingReason: "settled_complete_usage", PricingSnapshotJSON: paidPricingSnapshotJSON()}, charge: ChargeRow{ID: 14, Status: "open"}},
		{name: "item sum", run: RunDetailRow{ID: 47, BillingStatus: "settled", BillingReason: "settled_complete_usage", PricingSnapshotJSON: paidPricingSnapshotJSON()}, charge: ChargeRow{ID: 11, ActualUnits: 2, Status: "settled"}, items: []UsageChargeItemRow{{AttemptID: 101, AttemptNo: 1, AttemptState: "succeeded", Category: "input", Quantity: 1, Unit: "token", UnitScale: 1, AmountUnits: 1}}, attempts: []ProviderAttemptRow{{ID: 101, AttemptNo: 1, State: "succeeded", UsageStatus: "complete"}}},
		{name: "foreign attempt", run: RunDetailRow{ID: 52, BillingStatus: "settled", BillingReason: "settled_complete_usage", PricingSnapshotJSON: paidPricingSnapshotJSON()}, charge: ChargeRow{ID: 15, ActualUnits: 1, Status: "settled"}, items: []UsageChargeItemRow{{AttemptID: 999, AttemptNo: 1, AttemptState: "succeeded", Category: "input", Quantity: 1, Unit: "token", UnitScale: 1, AmountUnits: 1}}, attempts: []ProviderAttemptRow{{ID: 101, AttemptNo: 1, State: "succeeded", UsageStatus: "complete"}}},
		{name: "invalid item", run: RunDetailRow{ID: 49, BillingStatus: "settled", BillingReason: "settled_complete_usage", PricingSnapshotJSON: paidPricingSnapshotJSON()}, charge: ChargeRow{ID: 12, ActualUnits: 0, Status: "settled"}, items: []UsageChargeItemRow{{AttemptID: 101, AttemptNo: 1, AttemptState: "succeeded", Category: "input", Quantity: 1, Unit: "token", UnitScale: 0, AmountUnits: 0}}, attempts: []ProviderAttemptRow{{ID: 101, AttemptNo: 1, State: "succeeded", UsageStatus: "complete"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepository{run: &test.run, charge: &test.charge, usageItems: test.items, attempts: test.attempts}
			if _, appErr := NewService(repo).Detail(context.Background(), test.run.ID); appErr == nil || appErr.HTTPStatus != 500 {
				t.Fatalf("expected explicit internal error, got %#v", appErr)
			}
		})
	}
}

func paidPricingSnapshotJSON() string {
	return `{"version":"catalog-v1","billable":true,"catalog_vendor":"vendor-a","transport_engine":"openai","requested_model_id":"requested-alias","canonical_model_id":"canonical-model","catalog_max_output_tokens":8192,"effective_max_output_tokens":4096,"multiplier_ppm":1250000,"source_url":"https://example.test/pricing","retrieved_at":"2026-07-27","rates":[{"category":"input","unit":"token","tier_key":"","price_units":100000000,"unit_scale":1},{"category":"output","unit":"token","tier_key":"","price_units":200000000,"unit_scale":1}]}`
}

func TestDetailAllowsImageRunWithoutMessages(t *testing.T) {
	startedAt := time.Date(2026, 6, 7, 11, 18, 14, 0, time.UTC)
	repo := &fakeRepository{
		run: &RunDetailRow{
			ID: 9, Platform: enum.PlatformAdmin, RequestID: "ai_image_task-77",
			UserID: 7, Username: "image-user", AgentID: 8, AgentName: "image agent",
			ProviderID: 3, ProviderName: "OpenAI", Status: enum.AIRunStatusSuccess,
			ModelID: "gpt-image-1", InputSnapshot: "cat",
			TotalTokens: 11, StartedAt: &startedAt,
		},
	}
	res, appErr := NewService(repo).Detail(context.Background(), 9)
	if appErr != nil {
		t.Fatalf("detail failed: %v", appErr)
	}
	encoded, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal detail: %v", err)
	}
	for _, key := range []string{"modality", "source_type", "source_id", "usage_status"} {
		if strings.Contains(string(encoded), key) {
			t.Fatalf("AI run detail leaked retired source field %s: %s", key, string(encoded))
		}
	}
	if res.Platform != enum.PlatformAdmin || res.UserMessage != nil || res.AssistantMessage != nil {
		t.Fatalf("bad detail: %#v", res)
	}
}

func ptrUint(v uint) *uint { return &v }

func ptrInt64(v int64) *int64 { return &v }

func ptrString(v string) *string { return &v }
