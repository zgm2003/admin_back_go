package airun

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/clock"
	"admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/enum"
	sharedmoney "admin_back_go/internal/shared/money"
)

const timeLayout = "2006-01-02 15:04:05"

const (
	latencyStatsWindowDays     = 30
	latencyStatsMaxSamples     = 10000
	latencyStatsMinimumSamples = 20
)

var emptyJSONObject = json.RawMessage("{}")

var knowledgeRetrievalStatusLabels = map[string]string{
	"success": "检索成功",
	"failed":  "检索失败",
	"skipped": "未检索",
}

type Service struct {
	repository         Repository
	feedbackRepository FeedbackRepository
	clock              clock.Clock
	logger             *slog.Logger
}

type Option func(*Service)

func WithFeedbackRepository(repository FeedbackRepository) Option {
	return func(service *Service) {
		service.feedbackRepository = repository
	}
}

func WithClock(value clock.Clock) Option {
	return func(service *Service) {
		if value != nil {
			service.clock = value
		}
	}
}

func WithLogger(value *slog.Logger) Option {
	return func(service *Service) {
		if value != nil {
			service.logger = value
		}
	}
}

func NewService(repository Repository, options ...Option) *Service {
	service := &Service{repository: repository, clock: clock.SystemClock{}, logger: slog.Default()}
	if feedbackRepository, ok := repository.(FeedbackRepository); ok {
		service.feedbackRepository = feedbackRepository
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) PageInit(ctx context.Context) (*InitResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	agents, err := repo.AgentOptions(ctx)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI智能体选项失败", err)
	}
	engines, err := repo.ProviderOptions(ctx)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商选项失败", err)
	}
	agentOptions := optionItems(agents)
	return &InitResponse{Dict: InitDict{
		StatusArr:   dict.AIRunStatusOptions(),
		PlatformArr: dict.AIRunPlatformOptions(),
		AgentArr:    agentOptions,
		ProviderArr: optionItems(engines),
	}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(query)
	if appErr := validateOptionalPlatform(query.Platform); appErr != nil {
		return nil, appErr
	}
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI运行记录失败", err)
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
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI运行详情失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI运行记录不存在")
	}
	charge, usageRows, attemptRows, err := repo.BillingDetail(ctx, id)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI运行计费详情失败", err)
	}
	billingView, err := buildBillingDetail(*row, charge, usageRows, attemptRows)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "AI运行计费详情无效", err)
	}
	events, err := repo.Events(ctx, id)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI运行事件失败", err)
	}
	toolCalls, err := repo.ToolCalls(ctx, id)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI工具调用失败", err)
	}
	knowledgeRetrievals, appErr := s.knowledgeRetrievalItems(ctx, repo, id)
	if appErr != nil {
		return nil, appErr
	}
	result := detailItem(*row, events, knowledgeRetrievals, toolCalls, billingView, buildLatencyBreakdown(*row, attemptRows), buildSafeRequestSummary(attemptRows, toolCalls))
	return &result, nil
}

func (s *Service) knowledgeRetrievalItems(ctx context.Context, repo Repository, runID int64) ([]KnowledgeRetrievalItem, *apperror.Error) {
	rows, err := repo.KnowledgeRetrievals(ctx, runID)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI知识库检索记录失败", err)
	}
	if len(rows) == 0 {
		return []KnowledgeRetrievalItem{}, nil
	}
	retrievalIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		retrievalIDs = append(retrievalIDs, row.ID)
	}
	hits, err := repo.KnowledgeRetrievalHits(ctx, retrievalIDs)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI知识库检索命中失败", err)
	}
	hitsByRetrieval := make(map[int64][]KnowledgeHitRow, len(rows))
	for _, hit := range hits {
		hitsByRetrieval[hit.RetrievalID] = append(hitsByRetrieval[hit.RetrievalID], hit)
	}
	items := make([]KnowledgeRetrievalItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, knowledgeRetrievalItem(row, hitsByRetrieval[row.ID]))
	}
	return items, nil
}

func (s *Service) Stats(ctx context.Context, query StatsFilter) (*StatsResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeStatsFilter(query)
	if appErr := validateOptionalPlatform(query.Platform); appErr != nil {
		return nil, appErr
	}
	row, err := repo.StatsSummary(ctx, query)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI运行统计失败", err)
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
	if appErr := validateOptionalPlatform(query.Platform); appErr != nil {
		return nil, appErr
	}
	rows, total, err := repo.StatsByDate(ctx, query)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI运行日期统计失败", err)
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
	if appErr := validateOptionalPlatform(query.Platform); appErr != nil {
		return nil, appErr
	}
	rows, total, err := repo.StatsByAgent(ctx, query)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI运行智能体统计失败", err)
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
	if appErr := validateOptionalPlatform(query.Platform); appErr != nil {
		return nil, appErr
	}
	rows, total, err := repo.StatsByUser(ctx, query)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI运行用户统计失败", err)
	}
	list := make([]StatsByUserItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, StatsByUserItem{Username: row.Username, StatsMetricItem: metricItem(row.StatsMetricRow)})
	}
	return &StatsByUserResponse{List: list, Page: page(total, query.CurrentPage, query.PageSize)}, nil
}

func (s *Service) LatencyStats(ctx context.Context) (*LatencyStatsResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	now := s.clock.Now().UTC()
	rows, err := repo.LatencySamples(ctx, now.AddDate(0, 0, -latencyStatsWindowDays), latencyStatsMaxSamples)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI延迟统计失败", err)
	}
	type groupKey struct {
		providerID int64
		modelID    string
	}
	type samples struct {
		providerName string
		ttft         []int64
		provider     []int64
	}
	groups := make(map[groupKey]*samples)
	for _, row := range rows {
		key := groupKey{providerID: row.ProviderID, modelID: strings.TrimSpace(row.ModelID)}
		group := groups[key]
		if group == nil {
			group = &samples{providerName: strings.TrimSpace(row.ProviderName)}
			groups[key] = group
		}
		if value := nonNegativeDurationMS(row.DispatchedAt, row.FirstDeltaAt); value != nil {
			group.ttft = append(group.ttft, *value)
		}
		if value := nonNegativeDurationMS(row.DispatchedAt, row.FinishedAt); value != nil {
			group.provider = append(group.provider, *value)
		}
	}
	list := make([]LatencyStatsItem, 0, len(groups))
	for key, group := range groups {
		list = append(list, LatencyStatsItem{
			ProviderID: key.providerID, ProviderName: group.providerName, ModelID: key.modelID,
			TTFT: latencyDistribution(group.ttft), ProviderTotal: latencyDistribution(group.provider),
		})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].ProviderID != list[j].ProviderID {
			return list[i].ProviderID < list[j].ProviderID
		}
		return list[i].ModelID < list[j].ModelID
	})
	return &LatencyStatsResponse{WindowDays: latencyStatsWindowDays, MaxSamples: latencyStatsMaxSamples, List: list}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI运行仓储未配置")
	}
	return s.repository, nil
}

func validateOptionalPlatform(platform string) *apperror.Error {
	platform = strings.TrimSpace(platform)
	if platform != "" && !enum.IsRegisteredPlatform(platform) {
		return apperror.BadRequestKey("airun.platform.invalid", nil, "无效的AI运行平台")
	}
	return nil
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
	query.Platform = strings.TrimSpace(query.Platform)
	query.DateStart = strings.TrimSpace(query.DateStart)
	query.DateEnd = strings.TrimSpace(query.DateEnd)
	return query
}

func normalizeStatsFilter(query StatsFilter) StatsFilter {
	query.DateStart = strings.TrimSpace(query.DateStart)
	query.DateEnd = strings.TrimSpace(query.DateEnd)
	query.Platform = strings.TrimSpace(query.Platform)
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
	query.Platform = strings.TrimSpace(query.Platform)
	return query
}

func listItem(row ListRow) ListItem {
	return ListItem{
		ID: row.ID, RequestID: row.RequestID, UserID: row.UserID,
		AgentID: row.AgentID, AgentName: row.AgentName,
		ProviderID: row.ProviderID, ProviderName: row.ProviderName,
		Platform: row.Platform, InputSnapshot: row.InputSnapshot,
		ConversationID: row.ConversationID, ConversationTitle: row.ConversationTitle,
		Status: row.Status, StatusName: enum.AIRunStatusLabels[row.Status],
		ModelID: row.ModelID, ModelDisplayName: row.ModelDisplayName,
		PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens, TotalTokens: row.TotalTokens,
		DurationMS: row.DurationMS, DurationText: durationString(row.DurationMS), ErrorMessage: row.ErrorMessage,
		CreatedAt: formatTime(row.CreatedAt),
	}
}

type billingDetailView struct {
	status   string
	reason   string
	held     string
	actual   string
	pricing  *PricingDetail
	usage    []UsageItemDetail
	attempts []ProviderAttemptDetail
}

func buildBillingDetail(row RunDetailRow, charge *ChargeRow, usageRows []UsageChargeItemRow, attemptRows []ProviderAttemptRow) (billingDetailView, error) {
	legacy := row.BillingStatus == string(billing.BillingStatusUnbilled) && row.BillingReason == string(billing.BillingReasonLegacyUnpriced)
	legacy = legacy || (charge == nil && strings.TrimSpace(row.BillingStatus) == "" && strings.TrimSpace(row.BillingReason) == "")
	if legacy {
		if charge != nil || len(usageRows) != 0 || len(attemptRows) != 0 {
			return billingDetailView{}, fmt.Errorf("legacy unpriced run contains paid billing evidence")
		}
		return billingDetailView{
			status: string(billing.BillingStatusUnbilled), reason: string(billing.BillingReasonLegacyUnpriced),
			held: "0", actual: "0", usage: []UsageItemDetail{}, attempts: []ProviderAttemptDetail{},
		}, nil
	}
	if charge == nil {
		return billingDetailView{}, fmt.Errorf("paid run has no usage charge")
	}
	expectedChargeStatus, ok := chargeStatusForBillingStatus(row.BillingStatus)
	if !ok || charge.Status != expectedChargeStatus {
		return billingDetailView{}, fmt.Errorf("charge status %q does not match billing status %q", charge.Status, row.BillingStatus)
	}

	snapshot, err := aigateway.ParsePricingSnapshot(row.PricingSnapshotJSON)
	if err != nil {
		return billingDetailView{}, fmt.Errorf("parse paid pricing snapshot: %w", err)
	}
	pricingDetail, err := pricingDetailFromSnapshot(snapshot)
	if err != nil {
		return billingDetailView{}, err
	}
	held, err := sharedmoney.FormatRMBUnits(charge.HeldUnits)
	if err != nil {
		return billingDetailView{}, fmt.Errorf("format held amount: %w", err)
	}
	actual, err := sharedmoney.FormatRMBUnits(charge.ActualUnits)
	if err != nil {
		return billingDetailView{}, fmt.Errorf("format actual amount: %w", err)
	}

	attemptByID := make(map[int64]ProviderAttemptRow, len(attemptRows))
	for _, attempt := range attemptRows {
		if !validProviderAttemptRow(attempt) {
			return billingDetailView{}, fmt.Errorf("provider attempt %d is invalid", attempt.ID)
		}
		if _, exists := attemptByID[attempt.ID]; exists {
			return billingDetailView{}, fmt.Errorf("provider attempt %d is duplicated", attempt.ID)
		}
		attemptByID[attempt.ID] = attempt
	}

	usage := make([]UsageItemDetail, 0, len(usageRows))
	seenItems := make(map[string]struct{}, len(usageRows))
	var billableSum int64
	for _, item := range usageRows {
		if item.AttemptID <= 0 || item.AttemptNo == 0 || item.UnitPriceUnits < 0 || item.UnitScale <= 0 || item.AmountUnits < 0 {
			return billingDetailView{}, fmt.Errorf("persisted usage item has invalid identity or money fields")
		}
		if err := (billing.UsageItem{Category: billing.UsageCategory(item.Category), TierKey: item.TierKey, Quantity: item.Quantity, Unit: item.Unit}).Validate(); err != nil {
			return billingDetailView{}, fmt.Errorf("persisted usage item is invalid: %w", err)
		}
		attempt, exists := attemptByID[item.AttemptID]
		if !exists || attempt.AttemptNo != item.AttemptNo || attempt.State != item.AttemptState {
			return billingDetailView{}, fmt.Errorf("usage item attempt %d does not belong to this run", item.AttemptID)
		}
		unitPrice, formatErr := sharedmoney.FormatRMBUnits(item.UnitPriceUnits)
		if formatErr != nil {
			return billingDetailView{}, fmt.Errorf("format usage unit price: %w", formatErr)
		}
		amount, formatErr := sharedmoney.FormatRMBUnits(item.AmountUnits)
		if formatErr != nil {
			return billingDetailView{}, fmt.Errorf("format usage amount: %w", formatErr)
		}
		billable := item.AttemptState == string(billing.AttemptStateSucceeded)
		if billable {
			if item.AmountUnits > math.MaxInt64-billableSum {
				return billingDetailView{}, fmt.Errorf("billable usage amount overflow")
			}
			billableSum += item.AmountUnits
		}
		itemKey := usageItemKey(item.AttemptID, item.Category, item.TierKey, item.Unit)
		if _, exists := seenItems[itemKey]; exists {
			return billingDetailView{}, fmt.Errorf("usage item %q is duplicated", itemKey)
		}
		seenItems[itemKey] = struct{}{}
		usage = append(usage, UsageItemDetail{
			AttemptNo: item.AttemptNo, Category: item.Category, TierKey: item.TierKey,
			Quantity: item.Quantity, Unit: item.Unit, UnitPrice: unitPrice,
			UnitScale: item.UnitScale, Amount: amount, Billable: billable,
		})
	}
	if row.BillingStatus == string(billing.BillingStatusSettled) && billableSum != charge.ActualUnits {
		return billingDetailView{}, fmt.Errorf("settled usage item sum %d does not equal actual units %d", billableSum, charge.ActualUnits)
	}

	attempts := make([]ProviderAttemptDetail, 0, len(attemptRows))
	auditLines := make([]pricing.QuoteLine, 0)
	auditItems := make(map[string]UsageItemDetail)
	for _, attempt := range attemptRows {
		attempts = append(attempts, ProviderAttemptDetail{
			AttemptNo: attempt.AttemptNo, State: attempt.State,
			ProviderRequestID: optionalNonBlank(attempt.ProviderRequestID), UsageStatus: attempt.UsageStatus,
		})
		if attempt.State != string(billing.AttemptStateFailed) || attempt.UsageStatus != string(billing.UsageStatusComplete) {
			continue
		}
		var snapshotUsage infraai.UsageSnapshot
		if err := json.Unmarshal([]byte(attempt.UsageJSON), &snapshotUsage); err != nil || !snapshotUsage.Complete() {
			return billingDetailView{}, fmt.Errorf("failed attempt %d has invalid complete usage", attempt.AttemptNo)
		}
		for index, raw := range snapshotUsage.Items {
			key := usageItemKey(attempt.ID, raw.Category, raw.TierKey, raw.Unit)
			if _, exists := seenItems[key]; exists {
				continue
			}
			lineKey := strconv.FormatUint(uint64(attempt.AttemptNo), 10) + ":" + strconv.Itoa(index)
			auditLines = append(auditLines, pricing.QuoteLine{
				Key: lineKey, AttemptID: strconv.FormatInt(attempt.ID, 10),
				Item: billing.UsageItem{Category: billing.UsageCategory(raw.Category), TierKey: raw.TierKey, Quantity: raw.Quantity, Unit: raw.Unit},
			})
			auditItems[lineKey] = UsageItemDetail{AttemptNo: attempt.AttemptNo, Category: raw.Category, TierKey: raw.TierKey, Quantity: raw.Quantity, Unit: raw.Unit, Billable: false}
		}
	}
	if len(auditLines) > 0 {
		quoted, quoteErr := pricing.Quote(pricing.PriceBook{
			ModelID: snapshot.CanonicalModelID, ContextTierThresholdTokens: snapshot.ContextTierThresholdTokens,
			Rates: snapshot.Rates,
		}, auditLines, snapshot.MultiplierPPM)
		if quoteErr != nil {
			return billingDetailView{}, fmt.Errorf("price failed attempt audit usage: %w", quoteErr)
		}
		for _, line := range quoted.Lines {
			item := auditItems[line.Key]
			item.UnitPrice, err = sharedmoney.FormatRMBUnits(line.Rate.PriceUnits)
			if err != nil {
				return billingDetailView{}, fmt.Errorf("format failed attempt unit price: %w", err)
			}
			item.UnitScale = line.Rate.UnitScale
			item.Amount, err = sharedmoney.FormatRMBUnits(line.AmountUnits)
			if err != nil {
				return billingDetailView{}, fmt.Errorf("format failed attempt amount: %w", err)
			}
			usage = append(usage, item)
		}
	}
	sort.SliceStable(usage, func(i, j int) bool {
		if usage[i].AttemptNo != usage[j].AttemptNo {
			return usage[i].AttemptNo < usage[j].AttemptNo
		}
		if usage[i].Category != usage[j].Category {
			return usage[i].Category < usage[j].Category
		}
		if usage[i].TierKey != usage[j].TierKey {
			return usage[i].TierKey < usage[j].TierKey
		}
		return usage[i].Unit < usage[j].Unit
	})
	return billingDetailView{
		status: row.BillingStatus, reason: row.BillingReason, held: held, actual: actual,
		pricing: pricingDetail, usage: usage, attempts: attempts,
	}, nil
}

func chargeStatusForBillingStatus(status string) (string, bool) {
	switch billing.BillingStatus(status) {
	case billing.BillingStatusPending, billing.BillingStatusHeld:
		return string(billing.ChargeStatusOpen), true
	case billing.BillingStatusSettled:
		return string(billing.ChargeStatusSettled), true
	case billing.BillingStatusReleased:
		return string(billing.ChargeStatusReleased), true
	case billing.BillingStatusUnbilled:
		return string(billing.ChargeStatusUnbilled), true
	default:
		return "", false
	}
}

func validProviderAttemptRow(attempt ProviderAttemptRow) bool {
	if attempt.ID <= 0 || attempt.AttemptNo == 0 {
		return false
	}
	switch billing.AttemptState(attempt.State) {
	case billing.AttemptStatePrepared, billing.AttemptStateDispatched, billing.AttemptStateSucceeded,
		billing.AttemptStateFailed, billing.AttemptStateCanceled, billing.AttemptStateOutcomeUnknown:
	default:
		return false
	}
	return attempt.UsageStatus == string(billing.UsageStatusComplete) || attempt.UsageStatus == string(billing.UsageStatusUnavailable)
}

func pricingDetailFromSnapshot(snapshot aigateway.PricingSnapshot) (*PricingDetail, error) {
	rates := make([]PricingRateDetail, 0, len(snapshot.Rates))
	for _, rate := range snapshot.Rates {
		price, err := sharedmoney.FormatRMBUnits(rate.PriceUnits)
		if err != nil {
			return nil, fmt.Errorf("format pricing rate: %w", err)
		}
		rates = append(rates, PricingRateDetail{Category: string(rate.Category), TierKey: rate.TierKey, Unit: rate.Unit, Price: price, UnitScale: rate.UnitScale})
	}
	resolvedAlias := ""
	if snapshot.RequestedModelID != snapshot.CanonicalModelID {
		resolvedAlias = snapshot.RequestedModelID
	}
	return &PricingDetail{
		Version: snapshot.Version, CatalogVendor: snapshot.CatalogVendor, TransportEngine: snapshot.TransportEngine,
		ModelID: snapshot.CanonicalModelID, ResolvedAlias: resolvedAlias,
		BillingMultiplier: formatMultiplierPPM(snapshot.MultiplierPPM), MaxOutputTokens: snapshot.EffectiveMaxOutputTokens,
		Rates: rates,
	}, nil
}

func formatMultiplierPPM(ppm int64) string {
	whole := ppm / 1_000_000
	fraction := ppm % 1_000_000
	if fraction == 0 {
		return strconv.FormatInt(whole, 10)
	}
	return strconv.FormatInt(whole, 10) + "." + strings.TrimRight(fmt.Sprintf("%06d", fraction), "0")
}

func usageItemKey(attemptID int64, category, tierKey, unit string) string {
	return strconv.FormatInt(attemptID, 10) + "\x00" + strings.TrimSpace(category) + "\x00" + strings.TrimSpace(tierKey) + "\x00" + strings.TrimSpace(unit)
}

func optionalNonBlank(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func detailItem(row RunDetailRow, events []EventRow, knowledgeRetrievals []KnowledgeRetrievalItem, toolCalls []ToolCallRow, billingView billingDetailView, latency LatencyBreakdown, requestSummary SafeRequestSummary) DetailResponse {
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
		Platform: row.Platform, InputSnapshot: row.InputSnapshot,
		ConversationID: row.ConversationID, ConversationTitle: row.ConversationTitle,
		Status: row.Status, StatusName: enum.AIRunStatusLabels[row.Status],
		ModelID: row.ModelID, ModelDisplayName: row.ModelDisplayName,
		PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens, TotalTokens: row.TotalTokens,
		DurationMS: row.DurationMS, DurationText: durationString(row.DurationMS), ErrorMessage: row.ErrorMessage,
		BillingStatus: billingView.status, BillingReason: billingView.reason,
		HeldAmount: billingView.held, ActualAmount: billingView.actual,
		Pricing: billingView.pricing, UsageItems: billingView.usage, ProviderAttempts: billingView.attempts,
		Latency: latency, RequestSummary: requestSummary,
		UserMessage: row.UserMessage, AssistantMessage: row.AssistantMessage, Events: items, KnowledgeRetrievals: knowledgeRetrievals, ToolCalls: callItems,
		Liked: row.LikedAt != nil, LikedAt: formatOptionalTimePointer(row.LikedAt),
		StartedAt: formatOptionalTime(row.StartedAt), FinishedAt: formatOptionalTime(row.FinishedAt),
		CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}
}

func buildLatencyBreakdown(row RunDetailRow, attempts []ProviderAttemptRow) LatencyBreakdown {
	result := LatencyBreakdown{
		AcceptMS:    nonNegativeDurationMS(row.RequestReceivedAt, row.AcceptedAt),
		QueueMS:     nonNegativeDurationMS(row.AcceptedAt, row.ClaimedAt),
		EndToEndMS:  nonNegativeDurationMS(row.RequestReceivedAt, row.SettledAt),
		ClaimSource: strings.TrimSpace(row.ClaimSource),
	}
	if len(attempts) == 0 {
		return result
	}
	latest := attempts[len(attempts)-1]
	result.PrepareMS = nonNegativeDurationMS(latest.PrepareStartedAt, latest.DispatchedAt)
	result.TTFTMS = nonNegativeDurationMS(latest.DispatchedAt, latest.FirstDeltaAt)
	result.ProviderTotalMS = nonNegativeDurationMS(latest.DispatchedAt, latest.FinishedAt)
	result.SettlementMS = nonNegativeDurationMS(latest.FinishedAt, row.SettledAt)
	return result
}

func buildSafeRequestSummary(attempts []ProviderAttemptRow, toolCalls []ToolCallRow) SafeRequestSummary {
	result := SafeRequestSummary{ProviderAttemptCount: len(attempts), ToolCallCount: len(toolCalls)}
	if len(attempts) == 0 {
		return result
	}
	prepared := attempts[len(attempts)-1].PreparedRequestJSON
	result.PreparedRequestBytes = len(prepared)
	var envelope struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if json.Unmarshal([]byte(prepared), &envelope) == nil && envelope.Messages != nil {
		count := len(envelope.Messages)
		result.MessageCount = &count
	}
	return result
}

func nonNegativeDurationMS(start, end *time.Time) *int64 {
	if start == nil || end == nil || start.IsZero() || end.IsZero() || end.Before(*start) {
		return nil
	}
	value := end.Sub(*start).Milliseconds()
	return &value
}

func latencyDistribution(values []int64) LatencyDistribution {
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	result := LatencyDistribution{SampleCount: len(sorted), InsufficientSample: len(sorted) < latencyStatsMinimumSamples}
	if len(sorted) == 0 {
		return result
	}
	result.P50MS = nearestRank(sorted, 50)
	result.P95MS = nearestRank(sorted, 95)
	result.P99MS = nearestRank(sorted, 99)
	return result
}

func nearestRank(sorted []int64, percentile int) int64 {
	if len(sorted) == 0 || percentile <= 0 || percentile > 100 {
		return 0
	}
	index := (percentile*len(sorted)+99)/100 - 1
	return sorted[index]
}

func int64Pointer(value int64) *int64 { return &value }

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

func formatOptionalTimePointer(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := formatTime(*value)
	return &formatted
}
