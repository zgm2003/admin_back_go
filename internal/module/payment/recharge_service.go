package payment

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	walletmodule "admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/money"
)

func (s *Service) RechargePageInit(ctx context.Context, userID int64) (*RechargePageInitResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	wallet, err := repo.GetOrCreateWallet(ctx, userID)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "查询用户钱包失败", err)
	}
	packages, err := repo.ListRechargePackages(ctx)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "查询充值套餐失败", err)
	}
	payConfig, err := firstAvailableRechargeConfig(ctx, repo)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "查询可用支付配置失败", err)
	}
	summary, err := walletSummary(wallet)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "钱包余额数据无效", err)
	}
	return &RechargePageInitResponse{
		Wallet:        summary,
		Packages:      rechargePackageItems(packages),
		PaymentMethod: RechargePaymentMethod{Provider: providerAlipay, Label: providerText(providerAlipay), Enabled: len(packages) > 0 && payConfig != nil},
		Dict:          RechargePageInitDict{StatusArr: rechargeStatusOptions()},
	}, nil
}

func (s *Service) ListRecharges(ctx context.Context, query RechargeListQuery) (*RechargeListResponse, *apperror.Error) {
	if query.UserID <= 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.Status = strings.TrimSpace(query.Status)
	rows, total, err := repo.ListRecharges(ctx, query)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "查询充值记录失败", err)
	}
	_, size, _ := normalizePage(query.CurrentPage, query.PageSize)
	return &RechargeListResponse{List: rechargeListItems(rows), Page: Page{PageSize: size, CurrentPage: currentPage(query.CurrentPage), TotalPage: totalPage(total, size), Total: total}}, nil
}

func (s *Service) GetRecharge(ctx context.Context, userID int64, id int64) (*RechargeDetail, *apperror.Error) {
	row, appErr := s.rechargeByID(ctx, userID, id)
	if appErr != nil {
		return nil, appErr
	}
	detail := rechargeDetail(*row)
	return &detail, nil
}

func (s *Service) CreateRecharge(ctx context.Context, input RechargeCreateInput) (*RechargePayResponse, *apperror.Error) {
	if input.UserID <= 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	method := strings.TrimSpace(input.PayMethod)
	if !enum.IsPaymentMethod(method) {
		return nil, apperror.BadRequest("无效的支付方式")
	}
	returnURL := strings.TrimSpace(input.ReturnURL)
	if !isHTTPURL(returnURL) {
		return nil, apperror.BadRequest("支付返回地址必须是 http 或 https URL")
	}
	pkg, err := repo.GetRechargePackageByCode(ctx, strings.TrimSpace(input.PackageCode))
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "查询充值套餐失败", err)
	}
	if pkg == nil {
		return nil, apperror.NotFound("充值套餐不存在或已停用")
	}
	cfg, err := repo.FirstEnabledConfigForPay(ctx, providerAlipay, method)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "查询可用支付配置失败", err)
	}
	if cfg == nil {
		return nil, apperror.BadRequest("暂无可用支付宝支付配置")
	}
	if _, err := repo.GetOrCreateWallet(ctx, input.UserID); err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "初始化用户钱包失败", err)
	}
	now := s.now()
	order := Order{
		OrderNo:       newPaymentOrderNo(now),
		ConfigID:      cfg.ID,
		ConfigCode:    cfg.Code,
		Provider:      cfg.Provider,
		PayMethod:     method,
		Subject:       "余额充值 " + pkg.Name,
		AmountCents:   pkg.AmountCents,
		Status:        orderStatusPending,
		ExpiredAt:     now.Add(defaultOrderExpireMinutes * time.Minute),
		IsDel:         enum.CommonNo,
		FailureReason: "",
	}
	recharge := Recharge{
		RechargeNo:  newPaymentRechargeNo(now),
		UserID:      input.UserID,
		PackageCode: pkg.Code,
		PackageName: pkg.Name,
		AmountCents: pkg.AmountCents,
		Status:      rechargeStatusPending,
		IsDel:       enum.CommonNo,
	}
	order.ReturnURL = rechargeReturnURL(returnURL, recharge.RechargeNo)
	row, err := repo.CreateRechargeWithOrder(ctx, recharge, order)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "创建充值单失败", err)
	}
	return s.payRechargeRow(ctx, row)
}

func (s *Service) PayRecharge(ctx context.Context, userID int64, id int64) (*RechargePayResponse, *apperror.Error) {
	row, appErr := s.rechargeByID(ctx, userID, id)
	if appErr != nil {
		return nil, appErr
	}
	return s.payRechargeRow(ctx, *row)
}

func (s *Service) SyncRecharge(ctx context.Context, userID int64, id int64) (*RechargeStatusResponse, *apperror.Error) {
	row, appErr := s.rechargeByID(ctx, userID, id)
	if appErr != nil {
		return nil, appErr
	}
	return s.syncRechargeRow(ctx, *row)
}

func (s *Service) CloseRecharge(ctx context.Context, userID int64, id int64) (*RechargeStatusResponse, *apperror.Error) {
	row, appErr := s.rechargeByID(ctx, userID, id)
	if appErr != nil {
		return nil, appErr
	}
	if row.Status == rechargeStatusCredited || row.Status == rechargeStatusPaid {
		return nil, apperror.BadRequest("已支付充值单不能关闭")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	if row.Status != rechargeStatusClosed {
		if _, appErr := s.CloseOrder(ctx, row.PaymentOrderID); appErr != nil {
			return nil, appErr
		}
		if err := repo.UpdateRechargeClosed(ctx, row.ID); err != nil {
			return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "关闭充值单失败", err)
		}
	}
	latest, appErr := s.rechargeByID(ctx, userID, id)
	if appErr != nil {
		return nil, appErr
	}
	wallet, walletErr := repo.GetOrCreateWallet(ctx, userID)
	if walletErr != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "查询用户钱包失败", walletErr)
	}
	response, err := rechargeStatusResponse(*latest, wallet)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "钱包余额数据无效", err)
	}
	return response, nil
}

func (s *Service) payRechargeRow(ctx context.Context, row RechargeWithOrder) (*RechargePayResponse, *apperror.Error) {
	if row.Status == rechargeStatusCredited || row.Status == rechargeStatusPaid || row.Status == rechargeStatusClosed {
		return nil, apperror.BadRequest("当前充值单状态不能拉起支付")
	}
	if row.Status == rechargeStatusPaying && strings.TrimSpace(row.PayURL) != "" {
		return rechargePayResponse(row), nil
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	result, appErr := s.PayOrder(ctx, row.PaymentOrderID)
	if appErr != nil {
		if err := repo.UpdateRechargeFailed(ctx, row.ID, appErr.Message); err != nil {
			if errors.Is(err, ErrPaymentStateChanged) {
				return s.rechargePayChangedResponse(ctx, row.UserID, row.ID)
			}
			return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "更新充值失败状态失败", err)
		}
		return nil, appErr
	}
	if result.Status == orderStatusClosed {
		_ = repo.UpdateRechargeClosed(ctx, row.ID)
		row.Status = rechargeStatusClosed
		row.OrderStatus = orderStatusClosed
		return rechargePayResponse(row), nil
	}
	if err := repo.UpdateRechargePaying(ctx, row.ID); err != nil {
		if errors.Is(err, ErrPaymentStateChanged) {
			return s.rechargePayChangedResponse(ctx, row.UserID, row.ID)
		}
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "更新充值支付状态失败", err)
	}
	row.Status = rechargeStatusPaying
	row.OrderStatus = result.Status
	row.PayURL = result.PayURL
	return rechargePayResponse(row), nil
}

func (s *Service) rechargePayChangedResponse(ctx context.Context, userID, id int64) (*RechargePayResponse, *apperror.Error) {
	latest, appErr := s.rechargeByID(ctx, userID, id)
	if appErr != nil {
		return nil, appErr
	}
	return rechargePayResponse(*latest), nil
}

func (s *Service) syncRechargeRow(ctx context.Context, row RechargeWithOrder) (*RechargeStatusResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	wallet, err := repo.GetOrCreateWallet(ctx, row.UserID)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "查询用户钱包失败", err)
	}
	if row.Status == rechargeStatusCredited {
		return rechargeStatusReply(row, wallet)
	}
	if row.Status == rechargeStatusClosed {
		return rechargeStatusReply(row, wallet)
	}
	if row.OrderStatus == orderStatusPaying {
		if _, appErr := s.SyncOrder(ctx, row.PaymentOrderID); appErr != nil {
			return nil, appErr
		}
		latest, appErr := s.rechargeByID(ctx, row.UserID, row.ID)
		if appErr != nil {
			return nil, appErr
		}
		row = *latest
	}
	if row.OrderStatus == orderStatusClosed {
		if err := repo.UpdateRechargeClosed(ctx, row.ID); err != nil {
			return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "关闭充值单失败", err)
		}
		row.Status = rechargeStatusClosed
		return rechargeStatusReply(row, wallet)
	}
	if row.OrderStatus != orderStatusPaid {
		return rechargeStatusReply(row, wallet)
	}
	paidAt := s.now()
	if row.OrderPaidAt != nil {
		paidAt = *row.OrderPaidAt
	}
	if _, appErr := s.FinalizeOrderPaid(ctx, row.PaymentOrderID, row.AlipayTradeNo, paidAt, finalizeSourceSync); appErr != nil {
		return nil, appErr
	}
	latest, appErr := s.rechargeByID(ctx, row.UserID, row.ID)
	if appErr != nil {
		return nil, appErr
	}
	row = *latest
	wallet, err = repo.GetOrCreateWallet(ctx, row.UserID)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "查询用户钱包失败", err)
	}
	return rechargeStatusReply(row, wallet)
}

func (s *Service) rechargeByID(ctx context.Context, userID int64, id int64) (*RechargeWithOrder, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	if id <= 0 {
		return nil, apperror.BadRequest("无效的充值单ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.GetRecharge(ctx, userID, id)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "查询充值单失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("充值单不存在")
	}
	return row, nil
}

func firstAvailableRechargeConfig(ctx context.Context, repo Repository) (*Config, error) {
	for _, method := range []string{enum.PaymentMethodWeb, enum.PaymentMethodH5} {
		cfg, err := repo.FirstEnabledConfigForPay(ctx, providerAlipay, method)
		if err != nil || cfg != nil {
			return cfg, err
		}
	}
	return nil, nil
}

func rechargePackageItems(rows []RechargePackage) []RechargePackageItem {
	items := make([]RechargePackageItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, RechargePackageItem{
			Code:        row.Code,
			Name:        row.Name,
			AmountCents: row.AmountCents,
			AmountText:  amountText(row.AmountCents),
			Badge:       row.Badge,
		})
	}
	return items
}

func walletSummary(wallet *Wallet) (WalletSummary, error) {
	if wallet == nil {
		return WalletSummary{}, nil
	}
	balance, err := formatWalletUnits(wallet.BalanceUnits)
	if err != nil {
		return WalletSummary{}, err
	}
	available, err := formatWalletAvailable(wallet.BalanceUnits, wallet.HeldUnits)
	if err != nil {
		return WalletSummary{}, err
	}
	held, err := formatWalletUnits(wallet.HeldUnits)
	if err != nil {
		return WalletSummary{}, err
	}
	totalRecharge, err := formatWalletUnits(wallet.TotalRechargeUnits)
	if err != nil {
		return WalletSummary{}, err
	}
	totalConsume, err := formatWalletUnits(wallet.TotalConsumeUnits)
	if err != nil {
		return WalletSummary{}, err
	}
	return WalletSummary{Balance: balance, AvailableBalance: available, HeldAmount: held, TotalRecharge: totalRecharge, TotalConsume: totalConsume}, nil
}

func formatWalletUnits(units int64) (string, error) { return money.FormatRMBUnits(units) }
func formatWalletAvailable(balance, held int64) (string, error) {
	if balance < held {
		return "", walletmodule.ErrWalletInvariant
	}
	return formatWalletUnits(balance - held)
}

func rechargeListItems(rows []RechargeWithOrder) []RechargeListItem {
	items := make([]RechargeListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, rechargeListItem(row))
	}
	return items
}

func rechargeListItem(row RechargeWithOrder) RechargeListItem {
	return RechargeListItem{
		ID:             row.ID,
		RechargeNo:     row.RechargeNo,
		PaymentOrderNo: row.PaymentOrderNo,
		PackageCode:    row.PackageCode,
		PackageName:    row.PackageName,
		AmountCents:    row.AmountCents,
		AmountText:     amountText(row.AmountCents),
		Status:         row.Status,
		StatusText:     rechargeStatusText(row.Status),
		PayURL:         row.PayURL,
		PaidAt:         formatPtrTime(row.PaidAt),
		CreditedAt:     formatPtrTime(row.CreditedAt),
		CreatedAt:      formatTime(row.CreatedAt),
		UpdatedAt:      formatTime(row.UpdatedAt),
	}
}

func rechargeDetail(row RechargeWithOrder) RechargeDetail {
	return RechargeDetail{
		RechargeListItem: rechargeListItem(row),
		FailureReason:    row.FailureReason,
		AlipayTradeNo:    row.AlipayTradeNo,
	}
}

func rechargePayResponse(row RechargeWithOrder) *RechargePayResponse {
	return &RechargePayResponse{
		ID:             row.ID,
		RechargeNo:     row.RechargeNo,
		PaymentOrderNo: row.PaymentOrderNo,
		Status:         row.Status,
		PayURL:         row.PayURL,
	}
}

func rechargeStatusResponse(row RechargeWithOrder, wallet *Wallet) (*RechargeStatusResponse, error) {
	summary, err := walletSummary(wallet)
	if err != nil {
		return nil, err
	}
	return &RechargeStatusResponse{
		ID:            row.ID,
		RechargeNo:    row.RechargeNo,
		Status:        row.Status,
		StatusText:    rechargeStatusText(row.Status),
		Wallet:        summary,
		PaidAt:        formatPtrTime(row.PaidAt),
		CreditedAt:    formatPtrTime(row.CreditedAt),
		FailureReason: row.FailureReason,
	}, nil
}

func rechargeStatusReply(row RechargeWithOrder, wallet *Wallet) (*RechargeStatusResponse, *apperror.Error) {
	response, err := rechargeStatusResponse(row, wallet)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "钱包余额数据无效", err)
	}
	return response, nil
}

func rechargeStatusOptions() []dict.Option[string] {
	return []dict.Option[string]{
		{Label: "待支付", Value: rechargeStatusPending},
		{Label: "支付中", Value: rechargeStatusPaying},
		{Label: "已支付", Value: rechargeStatusPaid},
		{Label: "已入账", Value: rechargeStatusCredited},
		{Label: "已关闭", Value: rechargeStatusClosed},
		{Label: "支付失败", Value: rechargeStatusFailed},
	}
}

func rechargeStatusText(status string) string {
	for _, option := range rechargeStatusOptions() {
		if option.Value == status {
			return option.Label
		}
	}
	return status
}

func newPaymentRechargeNo(now time.Time) string {
	return newPaymentSerialNo("RCG", now)
}

func rechargeReturnURL(base string, rechargeNo string) string {
	base = strings.TrimSpace(base)
	if rechargeNo == "" {
		return base
	}
	separator := "?"
	if strings.Contains(base, "?") {
		separator = "&"
	}
	return base + separator + "tab=records&recharge_no=" + rechargeNo
}
