package airun

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/module/ai/billing"
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
	retrievals       []KnowledgeRetrievalRow
	hits             []KnowledgeHitRow
	hitQueryIDs      []int64
	hitQueries       int
	dashboardQuery   DashboardQuery
	dashboardRows    DashboardRepositoryResult
	dashboardErr     error
	dashboardCalls   int
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
func (f *fakeRepository) KnowledgeRetrievals(ctx context.Context, runID int64) ([]KnowledgeRetrievalRow, error) {
	return f.retrievals, nil
}
func (f *fakeRepository) KnowledgeRetrievalHits(ctx context.Context, retrievalIDs []int64) ([]KnowledgeHitRow, error) {
	f.hitQueries++
	f.hitQueryIDs = append([]int64(nil), retrievalIDs...)
	return f.hits, nil
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

func TestDetailIncludesKnowledgeRetrievals(t *testing.T) {
	startedAt := time.Date(2026, 5, 10, 20, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		run: &RunDetailRow{ID: 1, RequestID: "rid", Status: enum.AIRunStatusSuccess, CreatedAt: startedAt, UpdatedAt: startedAt},
		retrievals: []KnowledgeRetrievalRow{
			{ID: 7, RunID: 1, Query: "项目架构", Status: "success", TotalHits: 2, SelectedHits: 1, DurationMS: ptrUint(8), CreatedAt: startedAt},
			{ID: 9, RunID: 1, Query: "部署", Status: "success", TotalHits: 1, SelectedHits: 1, DurationMS: ptrUint(3), CreatedAt: startedAt.Add(time.Millisecond)},
		},
		hits: []KnowledgeHitRow{
			{ID: 8, RetrievalID: 7, KnowledgeBaseID: 1, KnowledgeBaseName: "架构库", DocumentID: 2, DocumentTitle: "Go 后端架构", ChunkID: 3, ChunkIndex: 1, Score: 0.82, RankNo: 1, ContentSnapshot: "Gin modular monolith", Status: 1, CreatedAt: startedAt},
			{ID: 10, RetrievalID: 9, KnowledgeBaseID: 1, KnowledgeBaseName: "架构库", DocumentID: 4, DocumentTitle: "部署", ChunkID: 5, ChunkIndex: 1, Score: 0.75, RankNo: 1, ContentSnapshot: "Docker", Status: 1, CreatedAt: startedAt},
		},
	}
	res, appErr := NewService(repo).Detail(context.Background(), 1)
	if appErr != nil {
		t.Fatalf("Detail returned error: %v", appErr)
	}
	if repo.hitQueries != 1 || len(repo.hitQueryIDs) != 2 || repo.hitQueryIDs[0] != 7 || repo.hitQueryIDs[1] != 9 {
		t.Fatalf("knowledge hits must load in one query, calls=%d ids=%v", repo.hitQueries, repo.hitQueryIDs)
	}
	if len(res.KnowledgeRetrievals) != 2 || len(res.KnowledgeRetrievals[0].Hits) != 1 || len(res.KnowledgeRetrievals[1].Hits) != 1 {
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

func ptrUint(v uint) *uint { return &v }

func ptrInt64(v int64) *int64 { return &v }

func ptrString(v string) *string { return &v }
