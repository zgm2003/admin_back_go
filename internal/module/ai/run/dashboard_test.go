package airun

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/clock"
)

func TestDashboardDefaultsToSevenShanghaiCalendarDays(t *testing.T) {
	now := dashboardFixedNow(t)
	repository := &fakeRepository{}
	service := NewService(repository, WithClock(clock.Func(func() time.Time { return now })))

	response, appErr := service.Dashboard(context.Background(), DashboardFilter{})
	if appErr != nil {
		t.Fatalf("Dashboard returned error: %v", appErr)
	}
	if repository.dashboardCalls != 1 {
		t.Fatalf("dashboard calls=%d", repository.dashboardCalls)
	}
	assertDashboardTime(t, "start_at", repository.dashboardQuery.StartAt, "2026-07-23T00:00:00+08:00")
	assertDashboardTime(t, "end_exclusive", repository.dashboardQuery.EndExclusive, "2026-07-30T00:00:00+08:00")
	assertDashboardTime(t, "generated_at", repository.dashboardQuery.GeneratedAt, "2026-07-29T15:42:18+08:00")
	assertDashboardTime(t, "stale_before", repository.dashboardQuery.StaleBefore, "2026-07-29T15:27:18+08:00")
	if !repository.dashboardQuery.StaleBefore.Equal(repository.dashboardQuery.GeneratedAt.Add(-config.DefaultAIRunStaleTimeout)) {
		t.Fatalf("stale_before=%s generated_at=%s timeout=%s", repository.dashboardQuery.StaleBefore, repository.dashboardQuery.GeneratedAt, config.DefaultAIRunStaleTimeout)
	}
	if response.GeneratedAt != "2026-07-29T15:42:18+08:00" || response.Timezone != dashboardTimezone {
		t.Fatalf("response metadata=%+v", response)
	}
	if response.DateRange.StartAt != "2026-07-23T00:00:00+08:00" || response.DateRange.EndExclusive != "2026-07-30T00:00:00+08:00" {
		t.Fatalf("date range=%+v", response.DateRange)
	}
}

func TestDashboardRejectsPartialInvalidReversedAndOverNinetyDayRanges(t *testing.T) {
	zero, negative := int64(0), int64(-1)
	tests := []struct {
		name   string
		filter DashboardFilter
	}{
		{name: "start only", filter: DashboardFilter{DateStart: "2026-07-01"}},
		{name: "end only", filter: DashboardFilter{DateEnd: "2026-07-29"}},
		{name: "invalid start", filter: DashboardFilter{DateStart: "2026/07/01", DateEnd: "2026-07-29"}},
		{name: "non canonical date", filter: DashboardFilter{DateStart: "2026-7-01", DateEnd: "2026-07-29"}},
		{name: "start with surrounding spaces", filter: DashboardFilter{DateStart: " 2026-07-01 ", DateEnd: "2026-07-29"}},
		{name: "end with trailing space", filter: DashboardFilter{DateStart: "2026-07-01", DateEnd: "2026-07-29 "}},
		{name: "invalid calendar date", filter: DashboardFilter{DateStart: "2026-02-30", DateEnd: "2026-03-01"}},
		{name: "reversed", filter: DashboardFilter{DateStart: "2026-07-29", DateEnd: "2026-07-28"}},
		{name: "over ninety inclusive days", filter: DashboardFilter{DateStart: "2026-04-30", DateEnd: "2026-07-29"}},
		{name: "invalid platform", filter: DashboardFilter{Platform: "partner_portal"}},
		{name: "zero agent", filter: DashboardFilter{AgentID: &zero}},
		{name: "negative provider", filter: DashboardFilter{ProviderID: &negative}},
		{name: "zero user", filter: DashboardFilter{UserID: &zero}},
		{name: "model too long", filter: DashboardFilter{ModelID: strings.Repeat("m", 192)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{}
			service := NewService(repository, WithClock(clock.Func(func() time.Time { return dashboardFixedNow(t) })))

			response, appErr := service.Dashboard(context.Background(), test.filter)
			if response != nil || appErr == nil || appErr.HTTPStatus != http.StatusBadRequest || appErr.LegacyCode != apperror.CodeBadRequest {
				t.Fatalf("response=%#v appErr=%#v", response, appErr)
			}
			if repository.dashboardCalls != 0 {
				t.Fatalf("invalid filter reached repository: %+v", repository.dashboardQuery)
			}
		})
	}

	agentID, providerID, userID := int64(3), int64(4), int64(5)
	repository := &fakeRepository{}
	service := NewService(repository, WithClock(clock.Func(func() time.Time { return dashboardFixedNow(t) })))
	_, appErr := service.Dashboard(context.Background(), DashboardFilter{
		DateStart: "2026-05-01", DateEnd: "2026-07-29", Platform: " admin ", ModelID: " gpt-test ",
		AgentID: &agentID, ProviderID: &providerID, UserID: &userID,
	})
	if appErr != nil {
		t.Fatalf("exactly ninety days returned error: %v", appErr)
	}
	if repository.dashboardQuery.Platform != "admin" || repository.dashboardQuery.ModelID != "gpt-test" {
		t.Fatalf("normalized filters=%+v", repository.dashboardQuery)
	}
	assertDashboardTime(t, "custom start_at", repository.dashboardQuery.StartAt, "2026-05-01T00:00:00+08:00")
	assertDashboardTime(t, "custom end_exclusive", repository.dashboardQuery.EndExclusive, "2026-07-30T00:00:00+08:00")
	if repository.dashboardQuery.AgentID == nil || *repository.dashboardQuery.AgentID != agentID ||
		repository.dashboardQuery.ProviderID == nil || *repository.dashboardQuery.ProviderID != providerID ||
		repository.dashboardQuery.UserID == nil || *repository.dashboardQuery.UserID != userID {
		t.Fatalf("normalized IDs=%+v", repository.dashboardQuery)
	}
}

func TestDashboardSuccessRateExcludesRunningAndCanceled(t *testing.T) {
	statuses := DashboardSummaryRow{
		TotalRuns: 18, RunningRuns: 4, SuccessRuns: 7, FailedRuns: 2, CanceledRuns: 3,
		TimeoutRuns: 1, OutcomeUnknownRuns: 1, PromptTokens: 100, CompletionTokens: 40, TotalTokens: 140,
	}
	repository := &fakeRepository{dashboardRows: DashboardRepositoryResult{
		Summary:      statuses,
		Trend:        []DashboardTrendRow{{Date: "2026-07-29", TotalRuns: 18, RunningRuns: 4, SuccessRuns: 7, FailedRuns: 2, CanceledRuns: 3, TimeoutRuns: 1, OutcomeUnknownRuns: 1}},
		Attributions: []DashboardAttributionRow{{Dimension: "model", Key: "gpt-test", Name: "GPT Test", TotalRuns: 18, SuccessRuns: 7, FailedRuns: 2, TimeoutRuns: 1, OutcomeUnknownRuns: 1}},
		Tools:        []DashboardToolRow{{ToolCode: "lookup", ToolName: "Lookup", TotalCalls: 14, SuccessCalls: 7, FailedCalls: 2, TimeoutCalls: 1}},
	}}

	response, appErr := NewService(repository, WithClock(clock.Func(func() time.Time { return dashboardFixedNow(t) }))).Dashboard(context.Background(), DashboardFilter{})
	if appErr != nil {
		t.Fatalf("Dashboard returned error: %v", appErr)
	}
	wantSummary := DashboardSummary{
		TotalRuns: 18, TerminalRuns: 14, InProgressRuns: 4, SuccessRuns: 7, FailedRuns: 2,
		TimeoutRuns: 1, OutcomeUnknownRuns: 1, CanceledRuns: 3, SuccessDenominator: 11,
		SuccessRate: 63.64, PromptTokens: 100, CompletionTokens: 40, TotalTokens: 140,
	}
	if response.Summary != wantSummary {
		t.Fatalf("summary=%+v want=%+v", response.Summary, wantSummary)
	}
	if got := response.Trend[0]; got.SuccessDenominator != 11 || got.SuccessRate != 63.64 || got.InProgressRuns != 4 {
		t.Fatalf("trend=%+v", got)
	}
	if got := response.Breakdowns.Models[0].DashboardAttributionMetrics; got.SuccessDenominator != 11 || got.SuccessRate != 63.64 {
		t.Fatalf("model metrics=%+v", got)
	}
	if got := response.Breakdowns.Tools[0]; got.SuccessDenominator != 10 || got.SuccessRate != 70 {
		t.Fatalf("tool=%+v", got)
	}
}

func TestDashboardFormatsOnlySettledActualUnits(t *testing.T) {
	repository := &fakeRepository{dashboardRows: DashboardRepositoryResult{
		Billing: DashboardBillingRow{SettledRuns: 2, ActualUnits: 150_000_000, ReleasedRuns: 3, ReleasedUnits: 575_000_000, UnbilledRuns: 4},
		Trend:   []DashboardTrendRow{{Date: "2026-07-29", ActualUnits: 25_000_000}},
		Attributions: []DashboardAttributionRow{
			{Dimension: "model", Key: "gpt-test", Name: "GPT Test", ActualUnits: 10_000_000},
			{Dimension: "provider", ID: 9, Name: "OpenAI", ActualUnits: 20_000_000},
			{Dimension: "agent", ID: 8, Name: "Agent", ActualUnits: 30_000_000},
			{Dimension: "user", ID: 7, Name: "admin", ActualUnits: 40_000_000},
		},
	}}

	response, appErr := NewService(repository, WithClock(clock.Func(func() time.Time { return dashboardFixedNow(t) }))).Dashboard(context.Background(), DashboardFilter{})
	if appErr != nil {
		t.Fatalf("Dashboard returned error: %v", appErr)
	}
	if response.Billing.ActualAmount != "1.5" || response.Billing.ReleasedAmount != "5.75" {
		t.Fatalf("billing=%+v", response.Billing)
	}
	if response.Trend[0].ActualAmount != "0.25" || response.Breakdowns.Models[0].ActualAmount != "0.1" || response.Breakdowns.Users[0].ActualAmount != "0.4" {
		t.Fatalf("trend=%+v breakdowns=%+v", response.Trend, response.Breakdowns)
	}
}

func TestDashboardFormatsIndependentBillingAmountsWithoutCombiningThem(t *testing.T) {
	repository := &fakeRepository{dashboardRows: DashboardRepositoryResult{
		Billing: DashboardBillingRow{ActualUnits: math.MaxInt64, ReleasedUnits: 1},
	}}

	response, appErr := NewService(repository, WithClock(clock.Func(func() time.Time { return dashboardFixedNow(t) }))).Dashboard(context.Background(), DashboardFilter{})
	if appErr != nil {
		t.Fatalf("Dashboard returned error: %v", appErr)
	}
	if response.Billing.ActualAmount != "92233720368.54775807" || response.Billing.ReleasedAmount != "0.00000001" {
		t.Fatalf("billing=%+v", response.Billing)
	}
}

func TestDashboardDoesNotInferHistoricalModelFromAttributionID(t *testing.T) {
	repository := &fakeRepository{dashboardRows: DashboardRepositoryResult{
		Attributions: []DashboardAttributionRow{
			{Dimension: "model", Key: "historical-candidate", ID: 0},
			{Dimension: "model", Key: "official-candidate", ID: 99},
		},
	}}

	response, appErr := NewService(repository, WithClock(clock.Func(func() time.Time { return dashboardFixedNow(t) }))).Dashboard(context.Background(), DashboardFilter{})
	if appErr != nil {
		t.Fatalf("Dashboard returned error: %v", appErr)
	}
	if len(response.Breakdowns.Models) != 2 {
		t.Fatalf("models=%+v", response.Breakdowns.Models)
	}
	for _, model := range response.Breakdowns.Models {
		if model.Historical {
			t.Fatalf("Task 1 must not infer historical from attribution ID: %+v", model)
		}
	}
}

func TestDashboardReturnsCompleteZeroObjectsAndEmptyArrays(t *testing.T) {
	response, appErr := NewService(&fakeRepository{}, WithClock(clock.Func(func() time.Time { return dashboardFixedNow(t) }))).Dashboard(context.Background(), DashboardFilter{})
	if appErr != nil {
		t.Fatalf("Dashboard returned error: %v", appErr)
	}
	if response.Billing.ActualAmount != "0" || response.Billing.ReleasedAmount != "0" {
		t.Fatalf("zero billing=%+v", response.Billing)
	}
	if !response.Performance.TTFT.InsufficientSample || !response.Performance.EndToEnd.InsufficientSample {
		t.Fatalf("zero performance=%+v", response.Performance)
	}
	if response.Trend == nil || response.Anomalies.RunItems == nil || response.Anomalies.BillingItems == nil ||
		response.Breakdowns.Models == nil || response.Breakdowns.Providers == nil || response.Breakdowns.Agents == nil ||
		response.Breakdowns.Users == nil || response.Breakdowns.Errors == nil || response.Breakdowns.Tools == nil {
		t.Fatalf("dashboard contains nil arrays: %+v", response)
	}

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal dashboard: %v", err)
	}
	for _, key := range []string{`"trend":[]`, `"run_items":[]`, `"billing_items":[]`, `"models":[]`, `"providers":[]`, `"agents":[]`, `"users":[]`, `"errors":[]`, `"tools":[]`} {
		if !strings.Contains(string(encoded), key) {
			t.Fatalf("dashboard JSON missing %s: %s", key, encoded)
		}
	}

	withMetrics, appErr := NewService(&fakeRepository{dashboardRows: DashboardRepositoryResult{
		Attributions: []DashboardAttributionRow{{
			Dimension: "model", Key: "gpt-test", Name: "GPT Test", TotalRuns: 1,
			SuccessRuns: 1, TotalTokens: 12, ActualUnits: 25_000_000,
		}},
	}}, WithClock(clock.Func(func() time.Time { return dashboardFixedNow(t) }))).Dashboard(context.Background(), DashboardFilter{})
	if appErr != nil {
		t.Fatalf("Dashboard with attribution returned error: %v", appErr)
	}
	encoded, err = json.Marshal(withMetrics)
	if err != nil {
		t.Fatalf("marshal dashboard with attribution: %v", err)
	}
	encodedText := string(encoded)
	if !strings.Contains(encodedText, `"models":[{"model_id":"gpt-test"`) ||
		!strings.Contains(encodedText, `"success_rate":100`) || strings.Contains(encodedText, `"metrics"`) {
		t.Fatalf("attribution metrics must be flattened: %s", encodedText)
	}
}

func TestDashboardMarksNineteenSamplesInsufficientAndTwentySufficient(t *testing.T) {
	repository := &fakeRepository{dashboardRows: DashboardRepositoryResult{
		Performance: DashboardPerformanceRow{
			TTFT:     DashboardDistributionRow{SampleCount: 19, P50MS: 10, P95MS: 20},
			EndToEnd: DashboardDistributionRow{SampleCount: 20, P50MS: 30, P95MS: 40},
		},
		Trend: []DashboardTrendRow{{
			Date:     "2026-07-29",
			TTFT:     DashboardDistributionRow{SampleCount: 19, P50MS: 11, P95MS: 21},
			EndToEnd: DashboardDistributionRow{SampleCount: 20, P50MS: 31, P95MS: 41},
		}},
		Tools: []DashboardToolRow{
			{ToolCode: "few", Duration: DashboardDistributionRow{SampleCount: 19, P50MS: 12, P95MS: 22}},
			{ToolCode: "enough", Duration: DashboardDistributionRow{SampleCount: 20, P50MS: 32, P95MS: 42}},
		},
	}}

	response, appErr := NewService(repository, WithClock(clock.Func(func() time.Time { return dashboardFixedNow(t) }))).Dashboard(context.Background(), DashboardFilter{})
	if appErr != nil {
		t.Fatalf("Dashboard returned error: %v", appErr)
	}
	if !response.Performance.TTFT.InsufficientSample || response.Performance.EndToEnd.InsufficientSample {
		t.Fatalf("performance=%+v", response.Performance)
	}
	if !response.Trend[0].TTFT.InsufficientSample || response.Trend[0].EndToEnd.InsufficientSample {
		t.Fatalf("trend=%+v", response.Trend[0])
	}
	if !response.Breakdowns.Tools[0].Duration.InsufficientSample || response.Breakdowns.Tools[1].Duration.InsufficientSample {
		t.Fatalf("tools=%+v", response.Breakdowns.Tools)
	}
}

func TestDashboardFailsOnNegativeOrOverflowedMoneyFacts(t *testing.T) {
	tests := []struct {
		name string
		rows DashboardRepositoryResult
	}{
		{name: "negative billing actual", rows: DashboardRepositoryResult{Billing: DashboardBillingRow{ActualUnits: -1}}},
		{name: "negative released amount", rows: DashboardRepositoryResult{Billing: DashboardBillingRow{ReleasedUnits: -1}}},
		{name: "negative trend amount", rows: DashboardRepositoryResult{Trend: []DashboardTrendRow{{ActualUnits: -1}}}},
		{name: "negative attribution amount", rows: DashboardRepositoryResult{Attributions: []DashboardAttributionRow{{Dimension: "model", ActualUnits: -1}}}},
		{name: "overflowed daily sum", rows: DashboardRepositoryResult{Trend: []DashboardTrendRow{{Date: "2026-07-28", ActualUnits: math.MaxInt64}, {Date: "2026-07-29", ActualUnits: 1}}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{dashboardRows: test.rows}
			response, appErr := NewService(repository, WithClock(clock.Func(func() time.Time { return dashboardFixedNow(t) }))).Dashboard(context.Background(), DashboardFilter{})
			if response != nil || appErr == nil || appErr.HTTPStatus != http.StatusInternalServerError || appErr.Cause == nil {
				t.Fatalf("response=%#v appErr=%#v", response, appErr)
			}
		})
	}
}

func TestDashboardWrapsRepositoryErrors(t *testing.T) {
	repositoryErr := errors.New("dashboard query failed")
	repository := &fakeRepository{dashboardErr: repositoryErr}
	response, appErr := NewService(repository, WithClock(clock.Func(func() time.Time { return dashboardFixedNow(t) }))).Dashboard(context.Background(), DashboardFilter{})
	if response != nil || appErr == nil || appErr.MessageID != "airun.dashboard.query_failed" || !errors.Is(appErr.Cause, repositoryErr) {
		t.Fatalf("response=%#v appErr=%#v", response, appErr)
	}
}

func dashboardFixedNow(t *testing.T) time.Time {
	t.Helper()
	value, err := time.Parse(time.RFC3339, "2026-07-29T15:42:18+08:00")
	if err != nil {
		t.Fatalf("parse fixed time: %v", err)
	}
	return value
}

func assertDashboardTime(t *testing.T, name string, value time.Time, want string) {
	t.Helper()
	if got := value.Format(time.RFC3339); got != want {
		t.Fatalf("%s=%s want=%s", name, got, want)
	}
}
