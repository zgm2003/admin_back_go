package aibilling

import (
	"context"
	"math"
	"strings"
	"time"

	walletmodule "admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/enum"
)

const (
	timeLayout      = "2006-01-02 15:04:05"
	defaultPageSize = 20
)

var sceneOptions = []dict.Option[string]{
	{Label: "Admin 图片生成", Value: SceneAdminImageGenerate},
	{Label: "无限画布-文本生成", Value: SceneCanvasTextGenerate},
	{Label: "无限画布-图片生成", Value: SceneCanvasImageGenerate},
	{Label: "无限画布-视频生成", Value: SceneCanvasVideoGenerate},
}

var unitOptions = []dict.Option[string]{
	{Label: "请求", Value: UnitRequest},
	{Label: "图片", Value: UnitImage},
	{Label: "秒", Value: UnitSecond},
}

type Service struct {
	repository Repository
	wallet     WalletService
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return NewServiceWithWallet(repository, nil, time.Now)
}

func NewServiceWithWallet(repository Repository, wallet WalletService, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repository: repository, wallet: wallet, now: now}
}

func (s *Service) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	_ = ctx
	return &PageInitResponse{Dict: PageInitDict{SceneArr: sceneOptions, UnitArr: unitOptions, CommonStatusArr: dict.CommonStatusOptions()}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(query)
	if query.Scene != "" && !validScene(query.Scene) {
		return nil, apperror.BadRequestKey("aibilling.rule.scene.invalid", nil, "AI计费场景无效")
	}
	if query.Unit != "" && !validUnit(query.Unit) {
		return nil, apperror.BadRequestKey("aibilling.rule.unit.invalid", nil, "AI计费单位无效")
	}
	if query.Status != nil && !validStatus(*query.Status) {
		return nil, apperror.BadRequestKey("aibilling.rule.status.invalid", nil, "AI计费规则状态无效")
	}
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, wrapInternal("aibilling.rule.query_failed", "查询AI计费规则失败", err)
	}
	list := make([]RuleDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, ruleDTO(row))
	}
	return &ListResponse{List: list, Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}

func (s *Service) CreateRule(ctx context.Context, input CreateRuleInput) (uint64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	row, appErr := normalizeCreateInput(input)
	if appErr != nil {
		return 0, appErr
	}
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, wrapInternal("aibilling.rule.create_failed", "新增AI计费规则失败", err)
	}
	return id, nil
}

func (s *Service) UpdateRule(ctx context.Context, id uint64, input UpdateRuleInput) *apperror.Error {
	if id == 0 {
		return apperror.BadRequestKey("aibilling.rule.id.invalid", nil, "AI计费规则ID无效")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return wrapInternal("aibilling.rule.query_failed", "查询AI计费规则失败", err)
	}
	if row == nil {
		return apperror.NotFoundKey("aibilling.rule.not_found", nil, "AI计费规则不存在")
	}
	fields, appErr := normalizeUpdateFields(input)
	if appErr != nil {
		return appErr
	}
	if err := repo.Update(ctx, id, fields); err != nil {
		return wrapInternal("aibilling.rule.update_failed", "编辑AI计费规则失败", err)
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	if id == 0 {
		return apperror.BadRequestKey("aibilling.rule.id.invalid", nil, "AI计费规则ID无效")
	}
	if !validStatus(status) {
		return apperror.BadRequestKey("aibilling.rule.status.invalid", nil, "AI计费规则状态无效")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return wrapInternal("aibilling.rule.query_failed", "查询AI计费规则失败", err)
	}
	if row == nil {
		return apperror.NotFoundKey("aibilling.rule.not_found", nil, "AI计费规则不存在")
	}
	if err := repo.ChangeStatus(ctx, id, status); err != nil {
		return wrapInternal("aibilling.rule.status_failed", "修改AI计费规则状态失败", err)
	}
	return nil
}

func (s *Service) DeleteRule(ctx context.Context, id uint64) *apperror.Error {
	if id == 0 {
		return apperror.BadRequestKey("aibilling.rule.id.invalid", nil, "AI计费规则ID无效")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return wrapInternal("aibilling.rule.query_failed", "查询AI计费规则失败", err)
	}
	if row == nil {
		return apperror.NotFoundKey("aibilling.rule.not_found", nil, "AI计费规则不存在")
	}
	if err := repo.Delete(ctx, id); err != nil {
		return wrapInternal("aibilling.rule.delete_failed", "删除AI计费规则失败", err)
	}
	return nil
}

func (s *Service) EnabledRule(ctx context.Context, scene string) (*RuleDTO, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	scene = strings.TrimSpace(scene)
	if !validScene(scene) {
		return nil, apperror.BadRequestKey("aibilling.rule.scene.invalid", nil, "AI计费场景无效")
	}
	row, err := repo.EnabledByScene(ctx, scene)
	if err != nil {
		return nil, wrapInternal("aibilling.rule.query_failed", "查询AI计费规则失败", err)
	}
	if row == nil || row.Status != RuleStatusEnabled || row.IsDel != enum.CommonNo {
		return nil, apperror.BadRequestKey("aibilling.rule.not_configured", nil, "AI计费规则未配置或已禁用")
	}
	dto := ruleDTO(*row)
	return &dto, nil
}

func (s *Service) BillingRecord(ctx context.Context, id int64) (*BillingRecord, *apperror.Error) {
	if id <= 0 {
		return nil, apperror.BadRequestKey("aibilling.record.id.invalid", nil, "AI计费记录ID无效")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	record, err := repo.GetRecord(ctx, id)
	if err != nil {
		return nil, wrapInternal("aibilling.record.query_failed", "查询AI计费记录失败", err)
	}
	if record == nil {
		return nil, apperror.NotFoundKey("aibilling.record.not_found", nil, "AI计费记录不存在")
	}
	return record, nil
}

func (s *Service) Charge(ctx context.Context, input ChargeInput) (*ChargeResult, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	walletService, appErr := s.requireWallet()
	if appErr != nil {
		return nil, appErr
	}
	input = normalizeChargeInput(input)
	if input.UserID <= 0 || input.RequestNo == "" || input.Platform == "" || input.UnitCount <= 0 {
		return nil, apperror.BadRequestKey("aibilling.charge.input.invalid", nil, "AI计费扣款参数无效")
	}
	if !validScene(input.Scene) {
		return nil, apperror.BadRequestKey("aibilling.rule.scene.invalid", nil, "AI计费场景无效")
	}
	rule, err := repo.EnabledByScene(ctx, input.Scene)
	if err != nil {
		return nil, wrapInternal("aibilling.rule.query_failed", "查询AI计费规则失败", err)
	}
	if rule == nil || rule.Status != RuleStatusEnabled || rule.IsDel != enum.CommonNo {
		return nil, apperror.BadRequestKey("aibilling.rule.not_configured", nil, "AI计费规则未配置或已禁用")
	}
	amount := int64(input.UnitCount) * rule.UnitPriceCents
	if amount <= 0 {
		return nil, apperror.BadRequestKey("aibilling.charge.amount.invalid", nil, "AI计费金额无效")
	}
	now := s.now()
	record := BillingRecord{
		RequestNo:      input.RequestNo,
		UserID:         input.UserID,
		Platform:       input.Platform,
		Scene:          input.Scene,
		AgentID:        input.AgentID,
		ProviderID:     input.ProviderID,
		ModelID:        input.ModelID,
		Unit:           rule.Unit,
		UnitCount:      input.UnitCount,
		UnitPriceCents: rule.UnitPriceCents,
		AmountCents:    amount,
		Status:         BillingStatusCharged,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	recordID, err := repo.CreateRecord(ctx, record)
	if err != nil {
		return nil, wrapInternal("aibilling.record.create_failed", "创建AI计费记录失败", err)
	}
	debit, debitErr := walletService.Debit(ctx, walletmodule.MutationInput{
		UserID:      input.UserID,
		AmountCents: amount,
		SourceType:  walletmodule.SourceAIGenerate,
		SourceID:    recordID,
		Remark:      input.Remark,
	})
	if debitErr != nil {
		_ = repo.UpdateRecord(context.Background(), recordID, map[string]any{"status": BillingStatusFailed, "error_message": debitErr.Message, "updated_at": s.now()})
		return nil, debitErr
	}
	var debitID int64
	if debit != nil {
		debitID = debit.Transaction.ID
	}
	fields := map[string]any{"debit_transaction_id": debitID, "updated_at": s.now()}
	if err := repo.UpdateRecord(ctx, recordID, fields); err != nil {
		return &ChargeResult{RecordID: recordID, AmountCents: amount, DebitTransactionID: debitID}, nil
	}
	return &ChargeResult{RecordID: recordID, AmountCents: amount, DebitTransactionID: debitID}, nil
}

func (s *Service) BindProviderTask(ctx context.Context, billingRecordID int64, providerTaskID string) *apperror.Error {
	if billingRecordID == 0 {
		return nil
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if err := repo.UpdateRecord(ctx, billingRecordID, map[string]any{"provider_task_id": strings.TrimSpace(providerTaskID), "updated_at": s.now()}); err != nil {
		return wrapInternal("aibilling.record.update_failed", "更新AI计费记录失败", err)
	}
	return nil
}

func (s *Service) MarkSuccess(ctx context.Context, billingRecordID int64) *apperror.Error {
	if billingRecordID == 0 {
		return nil
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	record, err := repo.GetRecord(ctx, billingRecordID)
	if err != nil {
		return wrapInternal("aibilling.record.query_failed", "查询AI计费记录失败", err)
	}
	if record == nil {
		return apperror.NotFoundKey("aibilling.record.not_found", nil, "AI计费记录不存在")
	}
	if record.Status == BillingStatusSuccess {
		return nil
	}
	if record.Status != BillingStatusCharged {
		return apperror.BadRequestKey("aibilling.record.status.invalid", nil, "AI计费记录状态无效")
	}
	finishedAt := s.now()
	if err := repo.UpdateRecord(ctx, billingRecordID, map[string]any{"status": BillingStatusSuccess, "finished_at": finishedAt, "updated_at": finishedAt}); err != nil {
		return wrapInternal("aibilling.record.update_failed", "更新AI计费记录失败", err)
	}
	return nil
}

func (s *Service) Refund(ctx context.Context, input RefundInput) *apperror.Error {
	if input.BillingRecordID == 0 {
		return nil
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	record, err := repo.GetRecord(ctx, input.BillingRecordID)
	if err != nil {
		return wrapInternal("aibilling.record.query_failed", "查询AI计费记录失败", err)
	}
	if record == nil {
		return nil
	}
	if record.RefundTransactionID != nil {
		return nil
	}
	if record.Status != BillingStatusCharged && record.Status != BillingStatusFailed {
		return apperror.BadRequestKey("aibilling.record.status.invalid", nil, "AI计费记录状态无效")
	}
	walletService, appErr := s.requireWallet()
	if appErr != nil {
		return appErr
	}
	credit, creditErr := walletService.Credit(ctx, walletmodule.MutationInput{
		UserID:      record.UserID,
		AmountCents: record.AmountCents,
		SourceType:  walletmodule.SourceAIRefund,
		SourceID:    record.ID,
		Remark:      strings.TrimSpace(input.Reason),
	})
	if creditErr != nil {
		_ = repo.UpdateRecord(context.Background(), record.ID, map[string]any{"status": BillingStatusFailed, "error_message": creditErr.Message, "updated_at": s.now()})
		return creditErr
	}
	var refundID int64
	if credit != nil {
		refundID = credit.Transaction.ID
	}
	finishedAt := s.now()
	if err := repo.UpdateRecord(ctx, record.ID, map[string]any{"status": BillingStatusRefunded, "refund_transaction_id": refundID, "error_message": strings.TrimSpace(input.Reason), "finished_at": finishedAt, "updated_at": finishedAt}); err != nil {
		return wrapInternal("aibilling.record.update_failed", "更新AI计费记录失败", err)
	}
	return nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("aibilling.service_missing", nil, "AI计费服务未配置")
	}
	return s.repository, nil
}

func (s *Service) requireWallet() (WalletService, *apperror.Error) {
	if s == nil || s.wallet == nil {
		return nil, apperror.InternalKey("aibilling.wallet_missing", nil, "AI计费钱包服务未配置")
	}
	return s.wallet, nil
}

func normalizeChargeInput(input ChargeInput) ChargeInput {
	input.RequestNo = strings.TrimSpace(input.RequestNo)
	input.Platform = strings.TrimSpace(input.Platform)
	input.Scene = strings.TrimSpace(input.Scene)
	input.ModelID = strings.TrimSpace(input.ModelID)
	input.Remark = strings.TrimSpace(input.Remark)
	return input
}

func normalizeCreateInput(input CreateRuleInput) (Rule, *apperror.Error) {
	scene := strings.TrimSpace(input.Scene)
	if !validScene(scene) {
		return Rule{}, apperror.BadRequestKey("aibilling.rule.scene.invalid", nil, "AI计费场景无效")
	}
	fields, appErr := normalizeMutableFields(input.Unit, input.UnitPriceCents, input.Status)
	if appErr != nil {
		return Rule{}, appErr
	}
	return Rule{Scene: scene, Unit: fields.unit, UnitPriceCents: fields.unitPriceCents, Status: fields.status, IsDel: enum.CommonNo}, nil
}

func normalizeUpdateFields(input UpdateRuleInput) (map[string]any, *apperror.Error) {
	fields, appErr := normalizeMutableFields(input.Unit, input.UnitPriceCents, input.Status)
	if appErr != nil {
		return nil, appErr
	}
	return map[string]any{"unit": fields.unit, "unit_price_cents": fields.unitPriceCents, "status": fields.status}, nil
}

type mutableFields struct {
	unit           string
	unitPriceCents int64
	status         int
}

func normalizeMutableFields(unit string, unitPriceCents int64, status int) (mutableFields, *apperror.Error) {
	unit = strings.TrimSpace(unit)
	if !validUnit(unit) {
		return mutableFields{}, apperror.BadRequestKey("aibilling.rule.unit.invalid", nil, "AI计费单位无效")
	}
	if unitPriceCents <= 0 {
		return mutableFields{}, apperror.BadRequestKey("aibilling.rule.unit_price.invalid", nil, "AI计费单价必须大于0")
	}
	if !validStatus(status) {
		return mutableFields{}, apperror.BadRequestKey("aibilling.rule.status.invalid", nil, "AI计费规则状态无效")
	}
	return mutableFields{unit: unit, unitPriceCents: unitPriceCents, status: status}, nil
}

func normalizeListQuery(query ListQuery) ListQuery {
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = defaultPageSize
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	query.Scene = strings.TrimSpace(query.Scene)
	query.Unit = strings.TrimSpace(query.Unit)
	return query
}

func validScene(value string) bool {
	for _, option := range sceneOptions {
		if option.Value == value {
			return true
		}
	}
	return false
}

func validUnit(value string) bool {
	for _, option := range unitOptions {
		if option.Value == value {
			return true
		}
	}
	return false
}

func validStatus(value int) bool {
	return value == RuleStatusEnabled || value == RuleStatusDisabled
}

func ruleDTO(row Rule) RuleDTO {
	return RuleDTO{
		ID:             row.ID,
		Scene:          row.Scene,
		SceneName:      optionLabel(sceneOptions, row.Scene),
		Unit:           row.Unit,
		UnitName:       optionLabel(unitOptions, row.Unit),
		UnitPriceCents: row.UnitPriceCents,
		Status:         row.Status,
		StatusName:     statusLabel(row.Status),
		CreatedAt:      formatTime(row.CreatedAt),
		UpdatedAt:      formatTime(row.UpdatedAt),
	}
}

func optionLabel(options []dict.Option[string], value string) string {
	for _, option := range options {
		if option.Value == value {
			return option.Label
		}
	}
	return ""
}

func statusLabel(value int) string {
	for _, option := range dict.CommonStatusOptions() {
		if option.Value == value {
			return option.Label
		}
	}
	return ""
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}

func wrapInternal(key string, fallback string, err error) *apperror.Error {
	return apperror.WrapKey(apperror.CodeInternal, 500, key, nil, fallback, err)
}
