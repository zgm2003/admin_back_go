package airun

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"admin_back_go/internal/config"
	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/clock"
	"admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/enum"
	sharedmoney "admin_back_go/internal/shared/money"
)

const timeLayout = "2006-01-02 15:04:05"

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

func (s *Service) PageInit(ctx context.Context, filter PageInitFilter) (*InitResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	now := time.Now()
	if s.clock != nil {
		now = s.clock.Now()
	}
	dateQuery, appErr := normalizeDashboardFilter(DashboardFilter{DateStart: filter.DateStart, DateEnd: filter.DateEnd}, now)
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
	historicalModels, err := repo.HistoricalModelOptions(ctx, dateQuery.StartAt, dateQuery.EndExclusive)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI运行历史模型选项失败", err)
	}
	agentOptions := optionItems(agents)
	return &InitResponse{Dict: InitDict{
		StatusArr:        dict.AIRunStatusOptions(),
		PlatformArr:      dict.AIRunPlatformOptions(),
		AgentArr:         agentOptions,
		ProviderArr:      optionItems(engines),
		ModelArr:         mergeRunModelOptions(historicalModels),
		BillingStatusArr: runBillingStatusOptions(),
		BillingReasonArr: runBillingReasonOptions(),
	}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	now := time.Now()
	if s.clock != nil {
		now = s.clock.Now()
	}
	query, appErr = normalizeListQuery(query, now)
	if appErr != nil {
		return nil, appErr
	}
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
	requestSummary, summaryErr := buildSafeRequestSummary(attemptRows, toolCalls)
	if summaryErr != nil {
		s.logInvalidPreparedRequestSummary(ctx, row.ID, attemptRows)
	}
	result := detailItem(*row, events, knowledgeRetrievals, toolCalls, billingView, s.buildLatencyBreakdown(ctx, *row, attemptRows, events), requestSummary)
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

func normalizeListQuery(query ListQuery, now time.Time) (ListQuery, *apperror.Error) {
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
	query.Status = strings.TrimSpace(query.Status)
	query.ModelID = strings.TrimSpace(query.ModelID)
	query.BillingStatus = strings.TrimSpace(query.BillingStatus)
	query.BillingReason = strings.TrimSpace(query.BillingReason)
	query.ErrorCode = strings.TrimSpace(query.ErrorCode)
	query.ToolCode = strings.TrimSpace(query.ToolCode)
	query.RunAnomaly = strings.TrimSpace(query.RunAnomaly)
	query.BillingAnomaly = strings.TrimSpace(query.BillingAnomaly)
	query.UserFeedback = strings.TrimSpace(query.UserFeedback)

	if query.Status != "" && !enum.IsAIRunStatus(query.Status) {
		return ListQuery{}, apperror.BadRequest("无效的AI运行状态")
	}
	if utf8.RuneCountInString(query.ModelID) > dashboardMaxModelIDLength {
		return ListQuery{}, apperror.BadRequest("模型ID长度不能超过191个字符")
	}
	if query.BillingStatus != "" && !isRunBillingStatus(query.BillingStatus) {
		return ListQuery{}, apperror.BadRequest("无效的AI运行计费状态")
	}
	if query.BillingReason != "" && !isRunBillingReason(query.BillingReason) {
		return ListQuery{}, apperror.BadRequest("无效的AI运行计费原因")
	}
	if utf8.RuneCountInString(query.ErrorCode) > 128 {
		return ListQuery{}, apperror.BadRequest("错误码长度不能超过128个字符")
	}
	if utf8.RuneCountInString(query.ToolCode) > 128 {
		return ListQuery{}, apperror.BadRequest("工具编码长度不能超过128个字符")
	}
	if query.RunAnomaly != "" && !isDashboardRunAnomaly(query.RunAnomaly) {
		return ListQuery{}, apperror.BadRequest("无效的AI运行异常分类")
	}
	if query.BillingAnomaly != "" && !isDashboardBillingAnomaly(query.BillingAnomaly) {
		return ListQuery{}, apperror.BadRequest("无效的AI计费异常分类")
	}
	if query.UserFeedback != "" && !isRunUserFeedback(query.UserFeedback) {
		return ListQuery{}, apperror.BadRequest("无效的AI运行用户反馈")
	}
	if appErr := validateDashboardID("agent_id", query.AgentID); appErr != nil {
		return ListQuery{}, appErr
	}
	if appErr := validateDashboardID("provider_id", query.ProviderID); appErr != nil {
		return ListQuery{}, appErr
	}
	if appErr := validateDashboardID("user_id", query.UserID); appErr != nil {
		return ListQuery{}, appErr
	}
	if query.DateStart != "" || query.DateEnd != "" {
		dateQuery, appErr := normalizeDashboardFilter(DashboardFilter{DateStart: query.DateStart, DateEnd: query.DateEnd}, now)
		if appErr != nil {
			return ListQuery{}, appErr
		}
		query.StartAt = dateQuery.StartAt
		query.EndExclusive = dateQuery.EndExclusive
	}
	if query.RunAnomaly != "" || query.BillingAnomaly != "" {
		generatedAt := now
		if query.AnomalyAsOf != "" {
			parsed, err := time.Parse(time.RFC3339, query.AnomalyAsOf)
			if err != nil {
				return ListQuery{}, apperror.BadRequest("无效的异常快照时间")
			}
			generatedAt = parsed
		}
		location, err := time.LoadLocation(dashboardTimezone)
		if err != nil {
			return ListQuery{}, apperror.Internal("AI运行驾驶舱时区不可用")
		}
		query.GeneratedAt = generatedAt.In(location)
		query.StaleBefore = query.GeneratedAt.Add(-config.DefaultAIRunStaleTimeout)
	} else {
		query.AnomalyAsOf = ""
	}
	return query, nil
}

func mergeRunModelOptions(historical []HistoricalModelRow) []ModelOption {
	officialModels := officialmodel.Default.Models()
	options := make([]ModelOption, 0, len(officialModels)+len(historical))
	seen := make(map[string]struct{}, len(officialModels)+len(historical))
	for _, model := range officialModels {
		options = append(options, ModelOption{Label: model.ModelID, Value: model.ModelID})
		seen[model.ModelID] = struct{}{}
	}
	for _, model := range historical {
		modelID := strings.TrimSpace(model.ModelID)
		if modelID == "" {
			continue
		}
		if _, exists := seen[modelID]; exists {
			continue
		}
		label := strings.TrimSpace(model.ModelDisplayName)
		if label == "" {
			label = modelID
		}
		options = append(options, ModelOption{Label: label, Value: modelID, Historical: true})
		seen[modelID] = struct{}{}
	}
	return options
}

func runBillingStatusOptions() []dict.Option[string] {
	return []dict.Option[string]{
		{Label: "待处理", Value: string(billing.BillingStatusPending)},
		{Label: "已预占", Value: string(billing.BillingStatusHeld)},
		{Label: "已结算", Value: string(billing.BillingStatusSettled)},
		{Label: "已释放", Value: string(billing.BillingStatusReleased)},
		{Label: "未计费", Value: string(billing.BillingStatusUnbilled)},
	}
}

func runBillingReasonOptions() []dict.Option[string] {
	return []dict.Option[string]{
		{Label: "待处理", Value: string(billing.BillingReasonPending)},
		{Label: "已预占", Value: string(billing.BillingReasonHeld)},
		{Label: "完整用量已结算", Value: string(billing.BillingReasonSettledCompleteUsage)},
		{Label: "分发前释放", Value: string(billing.BillingReasonReleasedBeforeDispatch)},
		{Label: "余额不足释放", Value: string(billing.BillingReasonReleasedInsufficientBalance)},
		{Label: "上游失败释放", Value: string(billing.BillingReasonReleasedProviderFailed)},
		{Label: "结果未知释放", Value: string(billing.BillingReasonReleasedOutcomeUnknown)},
		{Label: "用量不完整未计费", Value: string(billing.BillingReasonUnbilledUsageIncomplete)},
		{Label: "超出预占未计费", Value: string(billing.BillingReasonUnbilledOverHold)},
		{Label: "历史无价格", Value: string(billing.BillingReasonLegacyUnpriced)},
	}
}

func isRunBillingStatus(value string) bool {
	switch billing.BillingStatus(value) {
	case billing.BillingStatusPending, billing.BillingStatusHeld, billing.BillingStatusSettled, billing.BillingStatusReleased, billing.BillingStatusUnbilled:
		return true
	default:
		return false
	}
}

func isRunBillingReason(value string) bool {
	switch billing.BillingReason(value) {
	case billing.BillingReasonPending, billing.BillingReasonHeld, billing.BillingReasonSettledCompleteUsage,
		billing.BillingReasonReleasedBeforeDispatch, billing.BillingReasonReleasedInsufficientBalance,
		billing.BillingReasonReleasedProviderFailed, billing.BillingReasonReleasedOutcomeUnknown,
		billing.BillingReasonUnbilledUsageIncomplete, billing.BillingReasonUnbilledOverHold,
		billing.BillingReasonLegacyUnpriced:
		return true
	default:
		return false
	}
}

func isDashboardRunAnomaly(value string) bool {
	switch value {
	case "failed", "timeout", "outcome_unknown", "stale_running":
		return true
	default:
		return false
	}
}

func isRunUserFeedback(value string) bool {
	return value == "liked" || value == "unliked"
}

func isDashboardBillingAnomaly(value string) bool {
	switch value {
	case "state_inconsistent", "open_overdue", "pricing_snapshot_missing", "legacy_unpriced", "unbilled_usage_incomplete", "unbilled_over_hold":
		return true
	default:
		return false
	}
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
		BillingStatus: row.BillingStatus, BillingReason: row.BillingReason, ErrorCode: row.ErrorCode,
		Liked: row.LikedAt != nil, LikedAt: formatOptionalTimePointer(row.LikedAt),
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
		// Charge items already passed the finalizer's billable-attempt allowlist.
		// Non-billable failed-attempt usage is appended separately as audit evidence below.
		if item.AmountUnits > math.MaxInt64-billableSum {
			return billingDetailView{}, fmt.Errorf("billable usage amount overflow")
		}
		billableSum += item.AmountUnits
		itemKey := usageItemKey(item.AttemptID, item.Category, item.TierKey, item.Unit)
		if _, exists := seenItems[itemKey]; exists {
			return billingDetailView{}, fmt.Errorf("usage item %q is duplicated", itemKey)
		}
		seenItems[itemKey] = struct{}{}
		usage = append(usage, UsageItemDetail{
			AttemptNo: item.AttemptNo, Category: item.Category, TierKey: item.TierKey,
			Quantity: item.Quantity, Unit: item.Unit, UnitPrice: unitPrice,
			UnitScale: item.UnitScale, Amount: amount, Billable: true,
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
		if event.EventType == enum.AIRunEventFileMaterialized {
			continue
		}
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
		DurationMS: row.DurationMS, DurationText: durationString(row.DurationMS), ErrorCode: row.ErrorCode, ErrorMessage: row.ErrorMessage,
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

func (s *Service) buildLatencyBreakdown(ctx context.Context, row RunDetailRow, attempts []ProviderAttemptRow, events []EventRow) LatencyBreakdown {
	result := LatencyBreakdown{
		AcceptMS:    nonNegativeDurationMS(row.RequestReceivedAt, row.AcceptedAt),
		QueueMS:     nonNegativeDurationMS(row.AcceptedAt, row.ClaimedAt),
		EndToEndMS:  nonNegativeDurationMS(row.RequestReceivedAt, row.SettledAt),
		ClaimSource: strings.TrimSpace(row.ClaimSource),
	}
	if len(attempts) > 0 {
		latest := attempts[len(attempts)-1]
		result.PrepareMS = nonNegativeDurationMS(latest.PrepareStartedAt, latest.DispatchedAt)
		result.TTFTMS = nonNegativeDurationMS(latest.DispatchedAt, latest.FirstDeltaAt)
		result.ProviderTotalMS = nonNegativeDurationMS(latest.DispatchedAt, latest.FinishedAt)
		result.SettlementMS = nonNegativeDurationMS(latest.FinishedAt, row.SettledAt)
	}
	s.addDurableFileLatency(ctx, row.ID, events, &result)
	return result
}

type durableFileInputMetrics struct {
	COSHeadMS                *int64 `json:"cos_head_ms"`
	COSStreamMS              *int64 `json:"cos_stream_ms"`
	MaterializedRequestBytes *int64 `json:"materialized_request_bytes"`
}

func (s *Service) addDurableFileLatency(ctx context.Context, runID int64, events []EventRow, result *LatencyBreakdown) {
	if result == nil {
		return
	}
	var headTotal int64
	var streamTotal int64
	validEvents := 0
	for _, event := range events {
		if event.EventType != enum.AIRunEventFileMaterialized {
			continue
		}
		metrics, err := decodeDurableFileInputMetrics(event.Message)
		if err == nil {
			err = validateFileInputMetricsDuration(metrics, result.EndToEndMS)
		}
		candidateHead := headTotal
		candidateStream := streamTotal
		if err == nil {
			candidateHead, err = addNonNegativeInt64(headTotal, *metrics.COSHeadMS)
		}
		if err == nil {
			candidateStream, err = addNonNegativeInt64(streamTotal, *metrics.COSStreamMS)
		}
		if err != nil {
			s.logInvalidFileInputMetrics(ctx, runID, event)
			continue
		}
		headTotal = candidateHead
		streamTotal = candidateStream
		validEvents++
	}
	if validEvents == 0 {
		return
	}
	if result.EndToEndMS != nil {
		total, err := addNonNegativeInt64(headTotal, streamTotal)
		if err != nil || total > *result.EndToEndMS {
			s.logInvalidFileInputMetrics(ctx, runID, EventRow{})
			return
		}
	}
	result.COSHeadMS = &headTotal
	result.COSStreamMS = &streamTotal
}

func decodeDurableFileInputMetrics(message string) (durableFileInputMetrics, error) {
	var metrics durableFileInputMetrics
	decoder := json.NewDecoder(strings.NewReader(message))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metrics); err != nil {
		return durableFileInputMetrics{}, fmt.Errorf("decode metrics: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return durableFileInputMetrics{}, fmt.Errorf("metrics contain multiple JSON values")
		}
		return durableFileInputMetrics{}, fmt.Errorf("decode metrics trailer: %w", err)
	}
	if metrics.COSHeadMS == nil || metrics.COSStreamMS == nil || metrics.MaterializedRequestBytes == nil {
		return durableFileInputMetrics{}, fmt.Errorf("metrics require all fields")
	}
	if *metrics.COSHeadMS < 0 || *metrics.COSStreamMS < 0 || *metrics.MaterializedRequestBytes < 0 {
		return durableFileInputMetrics{}, fmt.Errorf("metrics must be non-negative")
	}
	return metrics, nil
}

func validateFileInputMetricsDuration(metrics durableFileInputMetrics, endToEndMS *int64) error {
	if endToEndMS == nil {
		return nil
	}
	total, err := addNonNegativeInt64(*metrics.COSHeadMS, *metrics.COSStreamMS)
	if err != nil || total > *endToEndMS {
		return fmt.Errorf("COS latency exceeds end-to-end duration")
	}
	return nil
}

func addNonNegativeInt64(left, right int64) (int64, error) {
	if left < 0 || right < 0 || right > math.MaxInt64-left {
		return 0, fmt.Errorf("non-negative integer overflow")
	}
	return left + right, nil
}

func (s *Service) logInvalidFileInputMetrics(ctx context.Context, runID int64, event EventRow) {
	logger := s.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.ErrorContext(ctx, "invalid durable AI file materialization metrics",
		slog.Int64("run_id", runID),
		slog.Int64("event_id", event.ID),
		slog.Uint64("event_seq", uint64(event.Seq)),
	)
}

func buildSafeRequestSummary(attempts []ProviderAttemptRow, toolCalls []ToolCallRow) (SafeRequestSummary, error) {
	result := SafeRequestSummary{ProviderAttemptCount: len(attempts), ToolCallCount: len(toolCalls)}
	if len(attempts) == 0 {
		return result, nil
	}
	prepared := []byte(attempts[len(attempts)-1].PreparedRequestJSON)
	schema, err := infraai.DetectPreparedChatSchema(prepared)
	if err != nil {
		return SafeRequestSummary{}, err
	}
	switch schema {
	case infraai.PreparedChatSchemaInlineV1:
		messageCount, attachmentCount, _, summaryErr := summarizePreparedChatRequest(prepared)
		if summaryErr != nil {
			return SafeRequestSummary{}, summaryErr
		}
		result.PreparedRequestBytes = len(prepared)
		result.MessageCount = messageCount
		result.AttachmentCount = attachmentCount
		return result, nil
	case infraai.PreparedChatSchemaFileManifestV1:
		manifest, parseErr := infraai.ParsePreparedChatFileManifest(prepared)
		if parseErr != nil {
			return SafeRequestSummary{}, parseErr
		}
		messageCount, attachmentCount, fileRefParts, summaryErr := summarizePreparedChatRequest(manifest.Request)
		if summaryErr != nil {
			return SafeRequestSummary{}, summaryErr
		}
		if len(fileRefParts) != len(manifest.Files) {
			return SafeRequestSummary{}, fmt.Errorf("prepared request file refs do not match manifest files")
		}
		var nativeFileBytes int64
		for _, file := range manifest.Files {
			nativeFileBytes, summaryErr = addNonNegativeInt64(nativeFileBytes, file.Size)
			if summaryErr != nil {
				return SafeRequestSummary{}, summaryErr
			}
		}
		materializedBytes, summaryErr := materializedPreparedRequestBytes(manifest, fileRefParts)
		if summaryErr != nil {
			return SafeRequestSummary{}, summaryErr
		}
		result.PreparedRequestBytes = len(prepared)
		result.MessageCount = messageCount
		result.AttachmentCount = attachmentCount
		result.NativeFileCount = len(manifest.Files)
		result.NativeFileBytes = nativeFileBytes
		result.PreparedManifestBytes = len(prepared)
		result.MaterializedRequestBytes = materializedBytes
		result.FileInputMode = manifest.FileInputMode
		return result, nil
	default:
		return SafeRequestSummary{}, fmt.Errorf("unsupported prepared request schema")
	}
}

type preparedRequestMessageSummary struct {
	Content json.RawMessage `json:"content"`
}

func summarizePreparedChatRequest(request []byte) (*int, int, []json.RawMessage, error) {
	var envelope struct {
		Messages []preparedRequestMessageSummary `json:"messages"`
	}
	if err := json.Unmarshal(request, &envelope); err != nil {
		return nil, 0, nil, fmt.Errorf("decode prepared chat request summary: %w", err)
	}
	var messageCount *int
	if envelope.Messages != nil {
		count := len(envelope.Messages)
		messageCount = &count
	}
	attachmentCount := 0
	fileRefParts := make([]json.RawMessage, 0)
	for _, message := range envelope.Messages {
		content := bytes.TrimSpace(message.Content)
		if len(content) == 0 {
			return nil, 0, nil, fmt.Errorf("prepared chat message content is missing")
		}
		if content[0] == '"' || bytes.Equal(content, []byte("null")) {
			continue
		}
		if content[0] != '[' {
			return nil, 0, nil, fmt.Errorf("prepared chat message content has an invalid shape")
		}
		var parts []json.RawMessage
		if err := json.Unmarshal(content, &parts); err != nil {
			return nil, 0, nil, fmt.Errorf("decode prepared chat content parts: %w", err)
		}
		for _, rawPart := range parts {
			var part struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(rawPart, &part); err != nil {
				return nil, 0, nil, fmt.Errorf("decode prepared chat content part: %w", err)
			}
			switch part.Type {
			case "image_url":
				attachmentCount++
			case "file_ref":
				attachmentCount++
				fileRefParts = append(fileRefParts, append(json.RawMessage(nil), rawPart...))
			}
		}
	}
	return messageCount, attachmentCount, fileRefParts, nil
}

func materializedPreparedRequestBytes(manifest infraai.PreparedChatFileManifest, fileRefParts []json.RawMessage) (int64, error) {
	total := int64(len(manifest.Request))
	for index, file := range manifest.Files {
		refBytes := int64(len(fileRefParts[index]))
		if refBytes > total {
			return 0, fmt.Errorf("prepared file ref length exceeds request length")
		}
		total -= refBytes
		part := struct {
			Type string `json:"type"`
			File struct {
				Filename string `json:"filename"`
				FileData string `json:"file_data"`
			} `json:"file"`
		}{Type: "file"}
		part.File.Filename = file.Filename
		part.File.FileData = "data:" + file.MIMEType + ";base64,"
		encodedPart, err := json.Marshal(part)
		if err != nil {
			return 0, fmt.Errorf("encode materialized file part summary: %w", err)
		}
		total, err = addNonNegativeInt64(total, int64(len(encodedPart)))
		if err != nil {
			return 0, err
		}
		base64Bytes, err := base64EncodedFileLength(file.Size)
		if err != nil {
			return 0, err
		}
		total, err = addNonNegativeInt64(total, base64Bytes)
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}

func base64EncodedFileLength(size int64) (int64, error) {
	if size < 0 || size > math.MaxInt64-2 {
		return 0, fmt.Errorf("file size cannot be represented as Base64 length")
	}
	groups := (size + 2) / 3
	if groups > math.MaxInt64/4 {
		return 0, fmt.Errorf("Base64 length overflows int64")
	}
	return groups * 4, nil
}

func (s *Service) logInvalidPreparedRequestSummary(ctx context.Context, runID int64, attempts []ProviderAttemptRow) {
	logger := s.logger
	if logger == nil {
		logger = slog.Default()
	}
	var attemptID int64
	if len(attempts) > 0 {
		attemptID = attempts[len(attempts)-1].ID
	}
	logger.ErrorContext(ctx, "invalid persisted AI prepared request summary",
		slog.Int64("run_id", runID),
		slog.Int64("attempt_id", attemptID),
	)
}

func nonNegativeDurationMS(start, end *time.Time) *int64 {
	if start == nil || end == nil || start.IsZero() || end.IsZero() || end.Before(*start) {
		return nil
	}
	value := end.Sub(*start).Milliseconds()
	return &value
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
