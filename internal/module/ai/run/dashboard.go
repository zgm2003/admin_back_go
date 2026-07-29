package airun

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"admin_back_go/internal/config"
	"admin_back_go/internal/shared/apperror"
	sharedmoney "admin_back_go/internal/shared/money"
)

const dashboardTimezone = "Asia/Shanghai"
const dashboardDefaultDays = 7
const dashboardMaxDays = 90
const dashboardMinimumSamples = 20

const (
	dashboardDateLayout       = "2006-01-02"
	dashboardMaxModelIDLength = 191
)

func (s *Service) Dashboard(ctx context.Context, filter DashboardFilter) (*DashboardResponse, *apperror.Error) {
	repository, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	now := time.Now()
	if s.clock != nil {
		now = s.clock.Now()
	}
	query, appErr := normalizeDashboardFilter(filter, now)
	if appErr != nil {
		return nil, appErr
	}
	rows, err := repository.Dashboard(ctx, query)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, http.StatusInternalServerError, "airun.dashboard.query_failed", nil, "查询AI运行驾驶舱失败", err)
	}
	response, err := buildDashboardResponse(query, rows)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, http.StatusInternalServerError, "airun.dashboard.result_invalid", nil, "AI运行驾驶舱统计结果无效", err)
	}
	return response, nil
}

func normalizeDashboardFilter(filter DashboardFilter, now time.Time) (DashboardQuery, *apperror.Error) {
	location, err := time.LoadLocation(dashboardTimezone)
	if err != nil {
		return DashboardQuery{}, apperror.WrapKey(apperror.CodeInternal, http.StatusInternalServerError, "airun.dashboard.timezone_invalid", nil, "AI运行驾驶舱时区不可用", err)
	}

	generatedAt := now.In(location)
	dateStart := strings.TrimSpace(filter.DateStart)
	dateEnd := strings.TrimSpace(filter.DateEnd)
	var startAt time.Time
	var endExclusive time.Time
	switch {
	case dateStart == "" && dateEnd == "":
		today := time.Date(generatedAt.Year(), generatedAt.Month(), generatedAt.Day(), 0, 0, 0, 0, location)
		startAt = today.AddDate(0, 0, -(dashboardDefaultDays - 1))
		endExclusive = today.AddDate(0, 0, 1)
	case dateStart == "" || dateEnd == "":
		return DashboardQuery{}, apperror.BadRequestKey("airun.dashboard.date_range.partial", nil, "开始日期和结束日期必须同时提供")
	default:
		startAt, err = parseDashboardDate(dateStart, location)
		if err != nil {
			return DashboardQuery{}, apperror.BadRequestKey("airun.dashboard.date_start.invalid", nil, "无效的开始日期")
		}
		endAt, parseErr := parseDashboardDate(dateEnd, location)
		if parseErr != nil {
			return DashboardQuery{}, apperror.BadRequestKey("airun.dashboard.date_end.invalid", nil, "无效的结束日期")
		}
		if startAt.After(endAt) {
			return DashboardQuery{}, apperror.BadRequestKey("airun.dashboard.date_range.reversed", nil, "开始日期不能晚于结束日期")
		}
		endExclusive = endAt.AddDate(0, 0, 1)
		if endExclusive.After(startAt.AddDate(0, 0, dashboardMaxDays)) {
			return DashboardQuery{}, apperror.BadRequestKey("airun.dashboard.date_range.too_large", map[string]any{"max_days": dashboardMaxDays}, "日期范围不能超过90天")
		}
	}

	if appErr := validateDashboardID("agent_id", filter.AgentID); appErr != nil {
		return DashboardQuery{}, appErr
	}
	if appErr := validateDashboardID("provider_id", filter.ProviderID); appErr != nil {
		return DashboardQuery{}, appErr
	}
	if appErr := validateDashboardID("user_id", filter.UserID); appErr != nil {
		return DashboardQuery{}, appErr
	}
	platform := strings.TrimSpace(filter.Platform)
	if appErr := validateOptionalPlatform(platform); appErr != nil {
		return DashboardQuery{}, appErr
	}
	modelID := strings.TrimSpace(filter.ModelID)
	if utf8.RuneCountInString(modelID) > dashboardMaxModelIDLength {
		return DashboardQuery{}, apperror.BadRequestKey("airun.dashboard.model_id.too_long", map[string]any{"max": dashboardMaxModelIDLength}, "模型ID长度不能超过191个字符")
	}

	return DashboardQuery{
		StartAt:      startAt,
		EndExclusive: endExclusive,
		GeneratedAt:  generatedAt,
		StaleBefore:  generatedAt.Add(-config.DefaultAIRunStaleTimeout),
		Platform:     platform,
		ModelID:      modelID,
		AgentID:      filter.AgentID,
		ProviderID:   filter.ProviderID,
		UserID:       filter.UserID,
	}, nil
}

func buildDashboardResponse(query DashboardQuery, rows DashboardRepositoryResult) (*DashboardResponse, error) {
	summaryDenominator, err := dashboardSumNonNegative("summary success denominator", rows.Summary.SuccessRuns, rows.Summary.FailedRuns, rows.Summary.TimeoutRuns, rows.Summary.OutcomeUnknownRuns)
	if err != nil {
		return nil, err
	}
	terminalRuns, err := dashboardSumNonNegative("summary terminal runs", rows.Summary.SuccessRuns, rows.Summary.FailedRuns, rows.Summary.CanceledRuns, rows.Summary.TimeoutRuns, rows.Summary.OutcomeUnknownRuns)
	if err != nil {
		return nil, err
	}
	if _, err := dashboardSumNonNegative("billing amount", rows.Billing.ActualUnits, rows.Billing.ReleasedUnits); err != nil {
		return nil, err
	}
	actualAmount, err := dashboardFormatUnits("billing actual amount", rows.Billing.ActualUnits)
	if err != nil {
		return nil, err
	}
	releasedAmount, err := dashboardFormatUnits("billing released amount", rows.Billing.ReleasedUnits)
	if err != nil {
		return nil, err
	}

	response := &DashboardResponse{
		GeneratedAt: query.GeneratedAt.Format(time.RFC3339),
		Timezone:    dashboardTimezone,
		DateRange: DashboardDateRange{
			StartAt:      query.StartAt.Format(time.RFC3339),
			EndExclusive: query.EndExclusive.Format(time.RFC3339),
		},
		Summary: DashboardSummary{
			TotalRuns:          rows.Summary.TotalRuns,
			TerminalRuns:       terminalRuns,
			InProgressRuns:     rows.Summary.RunningRuns,
			SuccessRuns:        rows.Summary.SuccessRuns,
			FailedRuns:         rows.Summary.FailedRuns,
			TimeoutRuns:        rows.Summary.TimeoutRuns,
			OutcomeUnknownRuns: rows.Summary.OutcomeUnknownRuns,
			CanceledRuns:       rows.Summary.CanceledRuns,
			SuccessDenominator: summaryDenominator,
			SuccessRate:        dashboardRate(rows.Summary.SuccessRuns, summaryDenominator),
			PromptTokens:       rows.Summary.PromptTokens,
			CompletionTokens:   rows.Summary.CompletionTokens,
			TotalTokens:        rows.Summary.TotalTokens,
		},
		Performance: DashboardPerformance{
			TTFT:     dashboardPercentile(rows.Performance.TTFT),
			EndToEnd: dashboardPercentile(rows.Performance.EndToEnd),
		},
		Billing: DashboardBilling{
			SettledRuns:    rows.Billing.SettledRuns,
			ActualAmount:   actualAmount,
			ReleasedRuns:   rows.Billing.ReleasedRuns,
			ReleasedAmount: releasedAmount,
			UnbilledRuns:   rows.Billing.UnbilledRuns,
		},
		Trend: make([]DashboardTrendItem, 0, len(rows.Trend)),
		Breakdowns: DashboardBreakdowns{
			Models:    make([]DashboardModelBreakdown, 0),
			Providers: make([]DashboardProviderBreakdown, 0),
			Agents:    make([]DashboardAgentBreakdown, 0),
			Users:     make([]DashboardUserBreakdown, 0),
			Errors:    make([]DashboardErrorBreakdown, 0, len(rows.Errors)),
			Tools:     make([]DashboardToolBreakdown, 0, len(rows.Tools)),
		},
	}

	response.Anomalies.RunItems, response.Anomalies.RunTotal, err = dashboardAnomalyItems("run anomalies", rows.RunAnomalies)
	if err != nil {
		return nil, err
	}
	response.Anomalies.BillingItems, response.Anomalies.BillingTotal, err = dashboardAnomalyItems("billing anomalies", rows.BillingAnomalies)
	if err != nil {
		return nil, err
	}

	var trendUnits int64
	for _, row := range rows.Trend {
		trendUnits, err = dashboardAddNonNegative("trend actual amount", trendUnits, row.ActualUnits)
		if err != nil {
			return nil, err
		}
		amount, formatErr := dashboardFormatUnits("trend actual amount", row.ActualUnits)
		if formatErr != nil {
			return nil, formatErr
		}
		denominator, sumErr := dashboardSumNonNegative("trend success denominator", row.SuccessRuns, row.FailedRuns, row.TimeoutRuns, row.OutcomeUnknownRuns)
		if sumErr != nil {
			return nil, sumErr
		}
		response.Trend = append(response.Trend, DashboardTrendItem{
			Date:               row.Date,
			TotalRuns:          row.TotalRuns,
			InProgressRuns:     row.RunningRuns,
			SuccessRuns:        row.SuccessRuns,
			FailedRuns:         row.FailedRuns,
			CanceledRuns:       row.CanceledRuns,
			TimeoutRuns:        row.TimeoutRuns,
			OutcomeUnknownRuns: row.OutcomeUnknownRuns,
			SuccessDenominator: denominator,
			SuccessRate:        dashboardRate(row.SuccessRuns, denominator),
			ActualAmount:       amount,
			TTFT:               dashboardPercentile(row.TTFT),
			EndToEnd:           dashboardPercentile(row.EndToEnd),
		})
	}

	attributionUnits := make(map[string]int64, 4)
	for _, row := range rows.Attributions {
		dimension := strings.TrimSpace(row.Dimension)
		switch dimension {
		case "model", "provider", "agent", "user":
		default:
			return nil, fmt.Errorf("unsupported dashboard attribution dimension %q", row.Dimension)
		}
		attributionUnits[dimension], err = dashboardAddNonNegative("attribution actual amount", attributionUnits[dimension], row.ActualUnits)
		if err != nil {
			return nil, err
		}
		metrics, metricsErr := dashboardAttributionMetrics(row)
		if metricsErr != nil {
			return nil, metricsErr
		}
		switch dimension {
		case "model":
			response.Breakdowns.Models = append(response.Breakdowns.Models, DashboardModelBreakdown{
				ModelID: row.Key, ModelDisplayName: row.Name, Historical: row.ID == 0, DashboardAttributionMetrics: metrics,
			})
		case "provider":
			response.Breakdowns.Providers = append(response.Breakdowns.Providers, DashboardProviderBreakdown{
				ProviderID: row.ID, ProviderName: row.Name, DashboardAttributionMetrics: metrics,
			})
		case "agent":
			response.Breakdowns.Agents = append(response.Breakdowns.Agents, DashboardAgentBreakdown{
				AgentID: row.ID, AgentName: row.Name, DashboardAttributionMetrics: metrics,
			})
		case "user":
			response.Breakdowns.Users = append(response.Breakdowns.Users, DashboardUserBreakdown{
				UserID: row.ID, Username: row.Name, DashboardAttributionMetrics: metrics,
			})
		}
	}

	for _, row := range rows.Errors {
		if row.Count < 0 {
			return nil, fmt.Errorf("dashboard error count must be non-negative")
		}
		response.Breakdowns.Errors = append(response.Breakdowns.Errors, DashboardErrorBreakdown{ErrorCode: row.ErrorCode, Count: row.Count})
	}
	for _, row := range rows.Tools {
		denominator, sumErr := dashboardSumNonNegative("tool success denominator", row.SuccessCalls, row.FailedCalls, row.TimeoutCalls)
		if sumErr != nil {
			return nil, sumErr
		}
		if row.TotalCalls < 0 {
			return nil, fmt.Errorf("dashboard tool total calls must be non-negative")
		}
		response.Breakdowns.Tools = append(response.Breakdowns.Tools, DashboardToolBreakdown{
			ToolCode: row.ToolCode, ToolName: row.ToolName, TotalCalls: row.TotalCalls,
			SuccessCalls: row.SuccessCalls, FailedCalls: row.FailedCalls, TimeoutCalls: row.TimeoutCalls,
			SuccessDenominator: denominator, SuccessRate: dashboardRate(row.SuccessCalls, denominator),
			Duration: dashboardPercentile(row.Duration),
		})
	}

	return response, nil
}

func dashboardRate(numerator, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	return math.Round(float64(numerator)*10000/float64(denominator)) / 100
}

func parseDashboardDate(value string, location *time.Location) (time.Time, error) {
	if len(value) != len(dashboardDateLayout) {
		return time.Time{}, fmt.Errorf("dashboard date must use %s", dashboardDateLayout)
	}
	parsed, err := time.ParseInLocation(dashboardDateLayout, value, location)
	if err != nil || parsed.Format(dashboardDateLayout) != value {
		return time.Time{}, fmt.Errorf("invalid dashboard date %q", value)
	}
	return parsed, nil
}

func validateDashboardID(field string, value *int64) *apperror.Error {
	if value != nil && *value <= 0 {
		return apperror.BadRequestKey("airun.dashboard.id.invalid", map[string]any{"field": field}, "筛选ID必须为正整数")
	}
	return nil
}

func dashboardPercentile(row DashboardDistributionRow) DashboardPercentile {
	if row.SampleCount <= 0 {
		return DashboardPercentile{InsufficientSample: true}
	}
	return DashboardPercentile{
		SampleCount:        row.SampleCount,
		InsufficientSample: row.SampleCount < dashboardMinimumSamples,
		P50MS:              row.P50MS,
		P95MS:              row.P95MS,
	}
}

func dashboardAnomalyItems(label string, rows []DashboardCountRow) ([]DashboardAnomalyItem, int64, error) {
	items := make([]DashboardAnomalyItem, 0, len(rows))
	var total int64
	for _, row := range rows {
		var err error
		total, err = dashboardAddNonNegative(label, total, row.Count)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, DashboardAnomalyItem{Code: row.Code, Count: row.Count})
	}
	return items, total, nil
}

func dashboardAttributionMetrics(row DashboardAttributionRow) (DashboardAttributionMetrics, error) {
	denominator, err := dashboardSumNonNegative("attribution success denominator", row.SuccessRuns, row.FailedRuns, row.TimeoutRuns, row.OutcomeUnknownRuns)
	if err != nil {
		return DashboardAttributionMetrics{}, err
	}
	amount, err := dashboardFormatUnits("attribution actual amount", row.ActualUnits)
	if err != nil {
		return DashboardAttributionMetrics{}, err
	}
	return DashboardAttributionMetrics{
		TotalRuns: row.TotalRuns, SuccessRuns: row.SuccessRuns,
		SuccessDenominator: denominator, SuccessRate: dashboardRate(row.SuccessRuns, denominator),
		TotalTokens: row.TotalTokens, ActualAmount: amount,
		RunAnomalyCount: row.RunAnomalyCount, BillingAnomalyCount: row.BillingAnomalyCount,
	}, nil
}

func dashboardFormatUnits(label string, units int64) (string, error) {
	value, err := sharedmoney.FormatRMBUnits(units)
	if err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	return value, nil
}

func dashboardSumNonNegative(label string, values ...int64) (int64, error) {
	var total int64
	var err error
	for _, value := range values {
		total, err = dashboardAddNonNegative(label, total, value)
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}

func dashboardAddNonNegative(label string, total, value int64) (int64, error) {
	if total < 0 || value < 0 || total > math.MaxInt64-value {
		return 0, fmt.Errorf("%s must be non-negative and fit in int64", label)
	}
	return total + value, nil
}
