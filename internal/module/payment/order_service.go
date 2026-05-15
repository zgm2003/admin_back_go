package payment

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
	gateway "admin_back_go/internal/platform/payment"
)

const defaultOrderExpireMinutes = 30

func (s *Service) OrderInit(ctx context.Context) (*OrderInitResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	rows, err := repo.ListEnabledOrderConfigOptions(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询支付配置选项失败", err)
	}
	options := make([]OrderConfigOption, 0, len(rows))
	for _, row := range rows {
		methods, appErr := decodeEnabledMethods(row.EnabledMethodsJSON)
		if appErr != nil {
			return nil, appErr
		}
		options = append(options, OrderConfigOption{
			Label:          fmt.Sprintf("%s(%s)", row.Name, row.Code),
			Value:          row.Code,
			Provider:       row.Provider,
			EnabledMethods: methods,
		})
	}
	return &OrderInitResponse{
		Dict: OrderInitDict{
			ProviderArr:    paymentProviderOptions(),
			PayMethodArr:   dict.PaymentMethodOptions(),
			OrderStatusArr: orderStatusOptions(),
		},
		ConfigOptions: options,
	}, nil
}

func (s *Service) ListOrders(ctx context.Context, query OrderListQuery) (*OrderListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.ConfigCode = strings.TrimSpace(query.ConfigCode)
	query.Provider = strings.TrimSpace(query.Provider)
	query.PayMethod = strings.TrimSpace(query.PayMethod)
	query.Status = strings.TrimSpace(query.Status)
	rows, total, err := repo.ListOrders(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询支付订单失败", err)
	}
	list := make([]OrderListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, orderListItem(row))
	}
	_, size, _ := normalizePage(query.CurrentPage, query.PageSize)
	return &OrderListResponse{List: list, Page: Page{PageSize: size, CurrentPage: currentPage(query.CurrentPage), TotalPage: totalPage(total, size), Total: total}}, nil
}

func (s *Service) GetOrder(ctx context.Context, id int64) (*OrderDetail, *apperror.Error) {
	row, appErr := s.orderByID(ctx, id)
	if appErr != nil {
		return nil, appErr
	}
	detail := orderDetail(*row)
	return &detail, nil
}

func (s *Service) CreateOrder(ctx context.Context, input OrderCreateInput) (*OrderCreateResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	cfg, appErr := s.enabledConfigByCode(ctx, input.ConfigCode)
	if appErr != nil {
		return nil, appErr
	}
	method := strings.TrimSpace(input.PayMethod)
	if !methodEnabled(cfg.EnabledMethodsJSON, method) {
		return nil, apperror.BadRequest("支付配置未启用该支付方式")
	}
	subject := strings.TrimSpace(input.Subject)
	if subject == "" {
		return nil, apperror.BadRequest("支付订单标题不能为空")
	}
	if input.AmountCents <= 0 {
		return nil, apperror.BadRequest("支付订单金额必须大于0")
	}
	returnURL := strings.TrimSpace(input.ReturnURL)
	if returnURL != "" && !isHTTPURL(returnURL) {
		return nil, apperror.BadRequest("支付返回地址必须是 http 或 https URL")
	}
	expireMinutes := input.ExpireMinutes
	if expireMinutes <= 0 {
		expireMinutes = defaultOrderExpireMinutes
	}
	now := s.now()
	row := Order{
		OrderNo:       newPaymentOrderNo(now),
		ConfigID:      cfg.ID,
		ConfigCode:    cfg.Code,
		Provider:      cfg.Provider,
		PayMethod:     method,
		Subject:       subject,
		AmountCents:   input.AmountCents,
		Status:        orderStatusPending,
		ReturnURL:     returnURL,
		ExpiredAt:     now.Add(time.Duration(expireMinutes) * time.Minute),
		IsDel:         enum.CommonNo,
		FailureReason: "",
	}
	id, err := repo.CreateOrder(ctx, row)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "新增支付订单失败", err)
	}
	return &OrderCreateResponse{ID: id, OrderNo: row.OrderNo, Status: row.Status}, nil
}

func (s *Service) PayOrder(ctx context.Context, id int64) (*OrderPayResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, appErr := s.orderByID(ctx, id)
	if appErr != nil {
		return nil, appErr
	}
	if row.Status == orderStatusPaying && strings.TrimSpace(row.PayURL) != "" {
		return &OrderPayResponse{ID: row.ID, OrderNo: row.OrderNo, Status: row.Status, PayURL: row.PayURL}, nil
	}
	if row.Status != orderStatusPending && row.Status != orderStatusFailed {
		return nil, apperror.BadRequest("当前支付订单状态不能拉起支付")
	}
	now := s.now()
	if !row.ExpiredAt.IsZero() && !now.Before(row.ExpiredAt) {
		if err := repo.UpdateOrderClosed(ctx, row.ID, now); err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "关闭过期支付订单失败", err)
		}
		return &OrderPayResponse{ID: row.ID, OrderNo: row.OrderNo, Status: orderStatusClosed, PayURL: row.PayURL}, nil
	}
	cfg, appErr := s.configByOrder(ctx, row)
	if appErr != nil {
		return nil, appErr
	}
	platformCfg, appErr := s.gatewayConfigFromConfig(*cfg)
	if appErr != nil {
		return nil, appErr
	}
	gw, appErr := s.requireGateway()
	if appErr != nil {
		return nil, appErr
	}
	result, err := gw.Pay(ctx, platformCfg, gateway.PayInput{
		OutTradeNo:  row.OrderNo,
		Method:      row.PayMethod,
		Subject:     row.Subject,
		AmountCents: row.AmountCents,
		ReturnURL:   row.ReturnURL,
		ExpiredAt:   row.ExpiredAt,
	})
	if err != nil {
		_ = repo.UpdateOrderFailed(ctx, row.ID, err.Error())
		return nil, apperror.Wrap(apperror.CodeBadRequest, http.StatusBadRequest, "拉起支付宝支付失败", err)
	}
	payURL := ""
	if result != nil {
		payURL = strings.TrimSpace(result.PayURL)
	}
	if payURL == "" {
		err := fmt.Errorf("alipay: empty pay url")
		_ = repo.UpdateOrderFailed(ctx, row.ID, err.Error())
		return nil, apperror.Wrap(apperror.CodeBadRequest, http.StatusBadRequest, "拉起支付宝支付失败", err)
	}
	if err := repo.UpdateOrderPaying(ctx, row.ID, payURL); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "保存支付链接失败", err)
	}
	return &OrderPayResponse{ID: row.ID, OrderNo: row.OrderNo, Status: orderStatusPaying, PayURL: payURL}, nil
}

func (s *Service) SyncOrder(ctx context.Context, id int64) (*OrderStatusResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, appErr := s.orderByID(ctx, id)
	if appErr != nil {
		return nil, appErr
	}
	if row.Status != orderStatusPaying {
		return nil, apperror.BadRequest("当前支付订单状态不能同步")
	}
	cfg, appErr := s.configByOrder(ctx, row)
	if appErr != nil {
		return nil, appErr
	}
	platformCfg, appErr := s.gatewayConfigFromConfig(*cfg)
	if appErr != nil {
		return nil, appErr
	}
	gw, appErr := s.requireGateway()
	if appErr != nil {
		return nil, appErr
	}
	result, err := gw.Query(ctx, platformCfg, row.OrderNo)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeBadRequest, http.StatusBadRequest, "同步支付宝订单状态失败", err)
	}
	switch strings.TrimSpace(resultStatus(result)) {
	case "TRADE_SUCCESS", "TRADE_FINISHED":
		paidAt := s.now()
		if result != nil && result.PaidAt != nil {
			paidAt = *result.PaidAt
		}
		if err := repo.UpdateOrderPaid(ctx, row.ID, resultTradeNo(result), paidAt); err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "保存支付订单成功状态失败", err)
		}
	case "TRADE_CLOSED":
		if err := repo.UpdateOrderClosed(ctx, row.ID, s.now()); err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "保存支付订单关闭状态失败", err)
		}
	case "WAIT_BUYER_PAY":
	default:
		return nil, apperror.BadRequest("未知的支付宝订单状态")
	}
	latest, appErr := s.orderByID(ctx, id)
	if appErr != nil {
		return nil, appErr
	}
	resp := orderStatusResponse(*latest)
	return &resp, nil
}

func (s *Service) CloseOrder(ctx context.Context, id int64) (*OrderStatusResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, appErr := s.orderByID(ctx, id)
	if appErr != nil {
		return nil, appErr
	}
	now := s.now()
	switch row.Status {
	case orderStatusClosed:
		resp := orderStatusResponse(*row)
		return &resp, nil
	case orderStatusPaid:
		return nil, apperror.BadRequest("已支付订单不能关闭")
	case orderStatusPending, orderStatusFailed:
		if err := repo.UpdateOrderClosed(ctx, row.ID, now); err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "关闭支付订单失败", err)
		}
	case orderStatusPaying:
		cfg, appErr := s.configByOrder(ctx, row)
		if appErr != nil {
			return nil, appErr
		}
		platformCfg, appErr := s.gatewayConfigFromConfig(*cfg)
		if appErr != nil {
			return nil, appErr
		}
		gw, appErr := s.requireGateway()
		if appErr != nil {
			return nil, appErr
		}
		if err := gw.Close(ctx, platformCfg, row.OrderNo); err != nil {
			return nil, apperror.Wrap(apperror.CodeBadRequest, http.StatusBadRequest, "关闭支付宝订单失败", err)
		}
		if err := repo.UpdateOrderClosed(ctx, row.ID, now); err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "关闭支付订单失败", err)
		}
	default:
		return nil, apperror.BadRequest("当前支付订单状态不能关闭")
	}
	latest, appErr := s.orderByID(ctx, id)
	if appErr != nil {
		return nil, appErr
	}
	resp := orderStatusResponse(*latest)
	return &resp, nil
}

func (s *Service) orderByID(ctx context.Context, id int64) (*Order, *apperror.Error) {
	if id <= 0 {
		return nil, apperror.BadRequest("无效的支付订单ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.GetOrder(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询支付订单失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("支付订单不存在")
	}
	return row, nil
}

func (s *Service) enabledConfigByCode(ctx context.Context, code string) (*Config, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	cfg, err := repo.GetConfigByCode(ctx, strings.TrimSpace(code))
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询支付配置失败", err)
	}
	if cfg == nil {
		return nil, apperror.NotFound("支付配置不存在")
	}
	if cfg.Status != enum.CommonYes {
		return nil, apperror.BadRequest("支付配置未启用")
	}
	if cfg.Provider != providerAlipay {
		return nil, apperror.BadRequest("当前仅支持支付宝支付配置")
	}
	return cfg, nil
}

func (s *Service) configByOrder(ctx context.Context, row *Order) (*Config, *apperror.Error) {
	if row == nil {
		return nil, apperror.NotFound("支付订单不存在")
	}
	cfg, appErr := s.enabledConfigByCode(ctx, row.ConfigCode)
	if appErr != nil {
		return nil, appErr
	}
	if cfg.ID != row.ConfigID {
		return nil, apperror.BadRequest("支付订单绑定配置不一致")
	}
	return cfg, nil
}

func (s *Service) gatewayConfigFromConfig(cfg Config) (gateway.ChannelConfig, *apperror.Error) {
	if strings.TrimSpace(cfg.Provider) != providerAlipay {
		return gateway.ChannelConfig{}, apperror.BadRequest("当前仅支持支付宝支付配置")
	}
	if !isPaymentEnvironment(strings.TrimSpace(cfg.Environment)) {
		return gateway.ChannelConfig{}, apperror.BadRequest("无效的支付宝环境")
	}
	if _, appErr := decodeEnabledMethods(cfg.EnabledMethodsJSON); appErr != nil {
		return gateway.ChannelConfig{}, appErr
	}
	privateKey, err := s.secretbox.Decrypt(cfg.PrivateKeyEnc)
	if err != nil {
		return gateway.ChannelConfig{}, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "解密支付宝应用私钥失败", err)
	}
	if strings.TrimSpace(privateKey) == "" {
		return gateway.ChannelConfig{}, apperror.BadRequest("支付宝应用私钥未配置")
	}
	appCertPath, err := s.certResolver.Resolve(cfg.AppCertPath)
	if err != nil {
		return gateway.ChannelConfig{}, apperror.Wrap(apperror.CodeBadRequest, http.StatusBadRequest, "应用公钥证书不可用", err)
	}
	alipayCertPath, err := s.certResolver.Resolve(cfg.PlatformCertPath)
	if err != nil {
		return gateway.ChannelConfig{}, apperror.Wrap(apperror.CodeBadRequest, http.StatusBadRequest, "支付宝公钥证书不可用", err)
	}
	rootCertPath, err := s.certResolver.Resolve(cfg.RootCertPath)
	if err != nil {
		return gateway.ChannelConfig{}, apperror.Wrap(apperror.CodeBadRequest, http.StatusBadRequest, "支付宝根证书不可用", err)
	}
	return gateway.ChannelConfig{
		Provider:         cfg.Provider,
		AppID:            cfg.AppID,
		PrivateKey:       privateKey,
		AppCertPath:      appCertPath,
		PlatformCertPath: alipayCertPath,
		RootCertPath:     rootCertPath,
		NotifyURL:        cfg.NotifyURL,
		IsSandbox:        cfg.Environment == environmentSandbox,
	}, nil
}

func (s *Service) requireGateway() (gateway.Gateway, *apperror.Error) {
	if s == nil || s.gateway == nil {
		return nil, apperror.Internal("支付宝网关未配置")
	}
	return s.gateway, nil
}

func methodEnabled(raw string, method string) bool {
	methods, appErr := decodeEnabledMethods(raw)
	if appErr != nil {
		return false
	}
	for _, item := range methods {
		if item == method {
			return true
		}
	}
	return false
}

func orderListItem(row Order) OrderListItem {
	return OrderListItem{
		ID:            row.ID,
		OrderNo:       row.OrderNo,
		ConfigCode:    row.ConfigCode,
		Provider:      row.Provider,
		ProviderText:  providerText(row.Provider),
		PayMethod:     row.PayMethod,
		PayMethodText: paymentMethodText(row.PayMethod),
		Subject:       row.Subject,
		AmountCents:   row.AmountCents,
		AmountText:    amountText(row.AmountCents),
		Status:        row.Status,
		StatusText:    orderStatusText(row.Status),
		PayURL:        row.PayURL,
		ExpiredAt:     formatTime(row.ExpiredAt),
		CreatedAt:     formatTime(row.CreatedAt),
		UpdatedAt:     formatTime(row.UpdatedAt),
	}
}

func orderDetail(row Order) OrderDetail {
	return OrderDetail{
		OrderListItem: orderListItem(row),
		ReturnURL:     row.ReturnURL,
		AlipayTradeNo: row.AlipayTradeNo,
		PaidAt:        formatPtrTime(row.PaidAt),
		ClosedAt:      formatPtrTime(row.ClosedAt),
		FailureReason: row.FailureReason,
	}
}

func orderStatusResponse(row Order) OrderStatusResponse {
	return OrderStatusResponse{
		ID:            row.ID,
		OrderNo:       row.OrderNo,
		Status:        row.Status,
		StatusText:    orderStatusText(row.Status),
		AlipayTradeNo: row.AlipayTradeNo,
		PaidAt:        formatPtrTime(row.PaidAt),
		ClosedAt:      formatPtrTime(row.ClosedAt),
	}
}

func orderStatusOptions() []dict.Option[string] {
	return []dict.Option[string]{
		{Label: "待支付", Value: orderStatusPending},
		{Label: "支付中", Value: orderStatusPaying},
		{Label: "已支付", Value: orderStatusPaid},
		{Label: "已关闭", Value: orderStatusClosed},
		{Label: "支付失败", Value: orderStatusFailed},
	}
}

func orderStatusText(status string) string {
	for _, option := range orderStatusOptions() {
		if option.Value == status {
			return option.Label
		}
	}
	return status
}

func paymentMethodText(method string) string {
	if label := enum.PaymentMethodLabels[method]; label != "" {
		return label
	}
	return method
}

func formatPtrTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTime(*value)
}

func newPaymentOrderNo(now time.Time) string {
	return "PAY" + now.Format("20060102150405") + fmt.Sprintf("%06d", now.Nanosecond()%1000000)
}

func resultStatus(result *gateway.QueryResult) string {
	if result == nil {
		return ""
	}
	return result.Status
}

func resultTradeNo(result *gateway.QueryResult) string {
	if result == nil {
		return ""
	}
	return result.TradeNo
}
