package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
	gateway "admin_back_go/internal/platform/payment"
	"admin_back_go/internal/platform/secretbox"
)

const timeLayout = "2006-01-02 15:04:05"

const (
	defaultOrderExpireDuration = 30 * time.Minute
	defaultCloseExpiredLimit   = 50
	defaultSyncPendingLimit    = 100
)

type secretDecrypter interface {
	Decrypt(ciphertext string) (string, error)
}

type secretCodec interface {
	Encrypt(plain string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

type certResolver interface {
	Resolve(path string) (string, error)
}

type noopCertResolver struct{}

func (noopCertResolver) Resolve(path string) (string, error) { return strings.TrimSpace(path), nil }

type Dependencies struct {
	Repository      Repository
	Gateway         gateway.Gateway
	Secretbox       secretCodec
	CertResolver    certResolver
	NumberGenerator NumberGenerator
	Now             func() time.Time
}

type Service struct {
	repository      Repository
	gateway         gateway.Gateway
	secretbox       secretCodec
	certResolver    certResolver
	numberGenerator NumberGenerator
	now             func() time.Time
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	box := deps.Secretbox
	if box == nil {
		box = secretbox.New("")
	}
	resolver := deps.CertResolver
	if resolver == nil {
		resolver = noopCertResolver{}
	}
	return &Service{
		repository:      deps.Repository,
		gateway:         deps.Gateway,
		secretbox:       box,
		certResolver:    resolver,
		numberGenerator: deps.NumberGenerator,
		now:             now,
	}
}

func (s *Service) ChannelInit(ctx context.Context) (*ChannelInitResponse, *apperror.Error) {
	return &ChannelInitResponse{Dict: ChannelInitDict{
		ProviderArr:     dict.PaymentProviderOptions(),
		CommonStatusArr: dict.CommonStatusOptions(),
		PayMethodArr:    dict.PaymentMethodOptions(),
		YesNoArr:        dict.CommonYesNoOptions(),
	}}, nil
}

func (s *Service) ListChannels(ctx context.Context, query ChannelListQuery) (*ChannelListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query.Name = strings.TrimSpace(query.Name)
	query.Provider = strings.TrimSpace(query.Provider)
	rows, total, err := repo.ListChannels(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询支付渠道失败", err)
	}
	list := make([]ChannelListItem, 0, len(rows))
	for _, row := range rows {
		cfg, err := repo.GetChannelConfig(ctx, row.ID)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询支付渠道配置失败", err)
		}
		list = append(list, channelListItem(row, cfg))
	}
	_, size, _ := normalizePage(query.CurrentPage, query.PageSize)
	return &ChannelListResponse{List: list, Page: Page{PageSize: size, CurrentPage: currentPage(query.CurrentPage), TotalPage: totalPage(total, size), Total: total}}, nil
}

func (s *Service) CreateChannel(ctx context.Context, input ChannelMutationInput) (int64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	channel, cfg, appErr := s.normalizeChannelMutation(input, true)
	if appErr != nil {
		return 0, appErr
	}
	id, err := repo.CreateChannel(ctx, channel, cfg)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增支付渠道失败", err)
	}
	return id, nil
}

func (s *Service) UpdateChannel(ctx context.Context, id int64, input ChannelMutationInput) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的支付渠道ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if row, err := repo.GetChannel(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询支付渠道失败", err)
	} else if row == nil {
		return apperror.NotFound("支付渠道不存在")
	}
	channel, cfg, appErr := s.normalizeChannelMutation(input, false)
	if appErr != nil {
		return appErr
	}
	fields := map[string]any{
		"code": channel.Code, "name": channel.Name, "provider": channel.Provider,
		"status": channel.Status, "supported_methods": channel.SupportedMethods, "remark": channel.Remark,
	}
	cfgFields := map[string]any{
		"app_id": cfg.AppID, "merchant_id": cfg.MerchantID, "sign_type": cfg.SignType, "is_sandbox": cfg.IsSandbox,
		"notify_url": cfg.NotifyURL, "return_url": cfg.ReturnURL, "app_cert_path": cfg.AppCertPath,
		"alipay_cert_path": cfg.AlipayCertPath, "alipay_root_cert_path": cfg.AlipayRootCertPath,
	}
	if cfg.PrivateKeyEnc != "" {
		cfgFields["private_key_enc"] = cfg.PrivateKeyEnc
		cfgFields["private_key_hint"] = cfg.PrivateKeyHint
	}
	if err := repo.UpdateChannel(ctx, id, fields, cfgFields); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "编辑支付渠道失败", err)
	}
	return nil
}

func (s *Service) ChangeChannelStatus(ctx context.Context, id int64, status int) *apperror.Error {
	if id <= 0 || !enum.IsCommonStatus(status) {
		return apperror.BadRequest("无效的支付渠道状态")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if err := repo.ChangeChannelStatus(ctx, id, status); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "切换支付渠道状态失败", err)
	}
	return nil
}

func (s *Service) DeleteChannel(ctx context.Context, id int64) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的支付渠道ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if err := repo.DeleteChannel(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除支付渠道失败", err)
	}
	return nil
}

func (s *Service) OrderInit(ctx context.Context) (*ChannelInitResponse, *apperror.Error) {
	return s.ChannelInit(ctx)
}

func (s *Service) ListOrders(ctx context.Context, query OrderListQuery) (*OrderListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	if appErr := validateOrderDateRange(query.StartDate, query.EndDate); appErr != nil {
		return nil, appErr
	}
	rows, total, err := repo.ListOrders(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询支付订单失败", err)
	}
	list := make([]OrderListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, orderListItem(row))
	}
	_, size, _ := normalizePage(query.CurrentPage, query.PageSize)
	return &OrderListResponse{List: list, Page: Page{PageSize: size, CurrentPage: currentPage(query.CurrentPage), TotalPage: totalPage(total, size), Total: total}}, nil
}

func (s *Service) GetAdminOrder(ctx context.Context, orderNo string) (*OrderDetailResponse, *apperror.Error) {
	order, appErr := s.getOrder(ctx, orderNo)
	if appErr != nil {
		return nil, appErr
	}
	return &OrderDetailResponse{Order: orderListItem(*order)}, nil
}

func (s *Service) GetOrderResult(ctx context.Context, userID int64, orderNo string) (*ResultResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("未登录")
	}
	order, appErr := s.getOrder(ctx, orderNo)
	if appErr != nil {
		return nil, appErr
	}
	if order.UserID != userID {
		return nil, apperror.Forbidden("无权查看该订单")
	}
	return resultResponse(order), nil
}

func (s *Service) CreateOrder(ctx context.Context, input CreateOrderInput) (*CreateOrderResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	input.Subject = strings.TrimSpace(input.Subject)
	input.PayMethod = strings.TrimSpace(input.PayMethod)
	if input.UserID <= 0 || input.ChannelID <= 0 || input.AmountCents <= 0 || input.Subject == "" {
		return nil, apperror.BadRequest("支付订单参数不完整")
	}
	channel, _, appErr := s.enabledChannel(ctx, repo, input.ChannelID, input.PayMethod)
	if appErr != nil {
		return nil, appErr
	}
	orderNo, appErr := s.nextNo(ctx)
	if appErr != nil {
		return nil, appErr
	}
	now := s.now()
	order := Order{
		OrderNo: orderNo, UserID: input.UserID, ChannelID: channel.ID, Provider: channel.Provider,
		PayMethod: input.PayMethod, Subject: input.Subject, AmountCents: input.AmountCents, Currency: "CNY",
		Status: enum.PaymentOrderPending, ExpiredAt: now.Add(defaultOrderExpireDuration), ClientIP: strings.TrimSpace(input.ClientIP),
		ReturnURL: strings.TrimSpace(input.ReturnURL), BusinessType: strings.TrimSpace(input.BusinessType), BusinessRef: strings.TrimSpace(input.BusinessRef),
		IsDel: enum.CommonNo, CreatedAt: now, UpdatedAt: now,
	}
	if order.BusinessType == "" {
		order.BusinessType = "manual_test"
	}
	created, err := repo.CreateOrder(ctx, order)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "创建支付订单失败", err)
	}
	return &CreateOrderResponse{OrderNo: created.OrderNo, AmountCents: created.AmountCents, ExpiredAt: formatTime(created.ExpiredAt)}, nil
}

func (s *Service) PayOrder(ctx context.Context, userID int64, orderNo string, returnURL string) (*PayOrderResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("未登录")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	order, appErr := s.getOrder(ctx, orderNo)
	if appErr != nil {
		return nil, appErr
	}
	if order.UserID != userID {
		return nil, apperror.Forbidden("无权操作该订单")
	}
	if order.Status != enum.PaymentOrderPending && order.Status != enum.PaymentOrderPaying {
		return nil, apperror.BadRequest("该订单状态不允许发起支付")
	}
	_, cfg, appErr := s.enabledChannel(ctx, repo, order.ChannelID, order.PayMethod)
	if appErr != nil {
		return nil, appErr
	}
	outTradeNo := strings.TrimSpace(ptrString(order.OutTradeNo))
	if outTradeNo == "" {
		outTradeNo = order.OrderNo
	}
	if strings.TrimSpace(returnURL) == "" {
		returnURL = order.ReturnURL
	}
	payCfg, appErr := s.gatewayConfig(cfg)
	if appErr != nil {
		return nil, appErr
	}
	result, err := s.requireGateway().CreatePagePay(ctx, payCfg, gateway.CreatePayRequest{
		OutTradeNo: outTradeNo, Subject: order.Subject, AmountCents: order.AmountCents, PayMethod: order.PayMethod, ReturnURL: strings.TrimSpace(returnURL),
	})
	if err != nil {
		if eventErr := repo.CreateEvent(ctx, Event{OrderNo: order.OrderNo, OutTradeNo: outTradeNo, EventType: enum.PaymentEventCreate, Provider: order.Provider, RequestData: mustJSON(sanitizeMap(map[string]any{"out_trade_no": outTradeNo, "return_url": returnURL})), ResponseData: mustJSON(sanitizeMap(map[string]any{"error": err.Error()})), ProcessStatus: enum.PaymentEventFailed, ErrorMessage: truncateError(err), CreatedAt: s.now()}); eventErr != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, 500, "记录支付事件失败", eventErr)
		}
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "创建支付宝支付请求失败", err)
	}
	if err := repo.WithTx(ctx, func(tx Repository) error {
		if err := tx.MarkOrderPaying(ctx, order.ID, outTradeNo, result.Content, strings.TrimSpace(returnURL), s.now()); err != nil {
			return fmt.Errorf("mark paying: %w", err)
		}
		if err := tx.CreateEvent(ctx, Event{OrderNo: order.OrderNo, OutTradeNo: outTradeNo, EventType: enum.PaymentEventCreate, Provider: order.Provider, RequestData: mustJSON(sanitizeMap(map[string]any{"out_trade_no": outTradeNo, "return_url": returnURL})), ResponseData: mustJSON(sanitizeMap(result.Raw)), ProcessStatus: enum.PaymentEventSuccess, CreatedAt: s.now()}); err != nil {
			return fmt.Errorf("create event: %w", err)
		}
		return nil
	}); err != nil {
		if strings.Contains(err.Error(), "create event:") {
			return nil, apperror.Wrap(apperror.CodeInternal, 500, "记录支付事件失败", errors.Unwrap(err))
		}
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "更新支付订单失败", errors.Unwrap(err))
	}
	return &PayOrderResponse{OrderNo: order.OrderNo, OutTradeNo: outTradeNo, PayMethod: order.PayMethod, PayURL: result.Content, PayData: map[string]any{"mode": result.Mode, "content": result.Content}}, nil
}

func (s *Service) CancelOrder(ctx context.Context, userID int64, orderNo string) *apperror.Error {
	if userID <= 0 {
		return apperror.Unauthorized("未登录")
	}
	order, appErr := s.getOrder(ctx, orderNo)
	if appErr != nil {
		return appErr
	}
	if order.UserID != userID {
		return apperror.Forbidden("无权操作该订单")
	}
	return s.closeOrder(ctx, order)
}

func (s *Service) CloseAdminOrder(ctx context.Context, orderNo string) *apperror.Error {
	order, appErr := s.getOrder(ctx, orderNo)
	if appErr != nil {
		return appErr
	}
	return s.closeOrder(ctx, order)
}

func (s *Service) ListEvents(ctx context.Context, query EventListQuery) (*EventListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	rows, total, err := repo.ListEvents(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询支付事件失败", err)
	}
	list := make([]EventListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, eventListItem(row))
	}
	_, size, _ := normalizePage(query.CurrentPage, query.PageSize)
	return &EventListResponse{List: list, Page: Page{PageSize: size, CurrentPage: currentPage(query.CurrentPage), TotalPage: totalPage(total, size), Total: total}}, nil
}

func (s *Service) GetEvent(ctx context.Context, id int64) (*EventDetailResponse, *apperror.Error) {
	if id <= 0 {
		return nil, apperror.BadRequest("无效的事件ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.GetEventByID(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询支付事件失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("支付事件不存在")
	}
	return &EventDetailResponse{Event: eventListItem(*row), RequestData: parseJSONMap(row.RequestData), ResponseData: parseJSONMap(row.ResponseData)}, nil
}

func (s *Service) HandleAlipayNotify(ctx context.Context, input NotifyInput) (string, *apperror.Error) {
	gw := s.requireGateway()
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return gw.FailureBody(), appErr
	}
	outTradeNo := strings.TrimSpace(input.Form["out_trade_no"])
	if outTradeNo == "" {
		return gw.FailureBody(), nil
	}
	order, err := repo.GetOrderByNo(ctx, outTradeNo)
	if err != nil {
		return gw.FailureBody(), apperror.Wrap(apperror.CodeInternal, 500, "查询支付订单失败", err)
	}
	if order == nil {
		return gw.FailureBody(), nil
	}
	_, cfg, appErr := s.enabledChannel(ctx, repo, order.ChannelID, order.PayMethod)
	if appErr != nil {
		return gw.FailureBody(), nil
	}
	payCfg, appErr := s.gatewayConfig(cfg)
	if appErr != nil {
		return gw.FailureBody(), appErr
	}
	result, err := gw.VerifyNotify(ctx, payCfg, input.Form)
	if err != nil {
		_ = s.writeNotifyEvent(ctx, repo, order, outTradeNo, input.Form, nil, enum.PaymentEventFailed, err)
		return gw.FailureBody(), nil
	}
	if err := validateNotify(order, cfg, result); err != nil {
		_ = s.writeNotifyEvent(ctx, repo, order, outTradeNo, input.Form, result.Raw, enum.PaymentEventFailed, err)
		return gw.FailureBody(), nil
	}
	if err := repo.WithTx(ctx, func(tx Repository) error {
		locked, err := tx.GetOrderByNoForUpdate(ctx, outTradeNo)
		if err != nil {
			return err
		}
		if locked == nil {
			return ErrOutTradeNoRequired
		}
		if locked.Status == enum.PaymentOrderSucceeded {
			return s.writeNotifyEvent(ctx, tx, locked, result.OutTradeNo, input.Form, result.Raw, enum.PaymentEventIgnored, nil)
		}
		if err := tx.MarkOrderSucceeded(ctx, locked.ID, result.TradeNo, s.now()); err != nil {
			return err
		}
		return s.writeNotifyEvent(ctx, tx, locked, result.OutTradeNo, input.Form, result.Raw, enum.PaymentEventSuccess, nil)
	}); err != nil {
		return gw.FailureBody(), apperror.Wrap(apperror.CodeInternal, 500, "处理支付宝回调失败", err)
	}
	return gw.SuccessBody(), nil
}

func (s *Service) CloseExpiredOrders(ctx context.Context, input CloseExpiredInput) (*JobResult, error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	now := input.Now
	if now.IsZero() {
		now = s.now()
	}
	limit := input.Limit
	if limit <= 0 {
		limit = defaultCloseExpiredLimit
	}
	rows, err := repo.ListExpiredOrders(ctx, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list expired payment orders: %w", err)
	}
	result := &JobResult{Scanned: len(rows)}
	for _, row := range rows {
		if row.OutTradeNo != nil {
			status, err := s.queryAndMark(ctx, repo, &row, now)
			if err != nil {
				result.Deferred++
				continue
			}
			if status == "paid" {
				result.Paid++
				continue
			}
			if status == "unpaid" {
				if err := s.closeRemote(ctx, repo, &row); err != nil {
					result.Deferred++
					continue
				}
			}
		}
		if err := repo.MarkOrderClosed(ctx, row.ID, now); err != nil {
			result.Deferred++
			continue
		}
		result.Closed++
	}
	return result, nil
}

func (s *Service) SyncPendingOrders(ctx context.Context, input SyncPendingInput) (*JobResult, error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	now := input.Now
	if now.IsZero() {
		now = s.now()
	}
	limit := input.Limit
	if limit <= 0 {
		limit = defaultSyncPendingLimit
	}
	rows, err := repo.ListPendingOrders(ctx, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending payment orders: %w", err)
	}
	result := &JobResult{Scanned: len(rows)}
	for _, row := range rows {
		status, err := s.queryAndMark(ctx, repo, &row, now)
		if err != nil {
			result.Deferred++
			continue
		}
		if status == "paid" {
			result.Paid++
		} else {
			result.Skipped++
		}
	}
	return result, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("支付仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) requireGateway() gateway.Gateway {
	if s == nil || s.gateway == nil {
		return nilGateway{}
	}
	return s.gateway
}

func (s *Service) nextNo(ctx context.Context) (string, *apperror.Error) {
	if s == nil || s.numberGenerator == nil {
		return "", apperror.Internal("支付单号生成器未配置")
	}
	no, err := s.numberGenerator.Next(ctx, "P")
	if err != nil {
		return "", apperror.Wrap(apperror.CodeInternal, 500, "生成支付单号失败", err)
	}
	return no, nil
}

func (s *Service) normalizeChannelMutation(input ChannelMutationInput, requirePrivateKey bool) (Channel, ChannelConfig, *apperror.Error) {
	if !enum.IsPaymentProvider(strings.TrimSpace(input.Provider)) || !enum.IsCommonStatus(input.Status) || !enum.IsCommonYesNo(input.IsSandbox) {
		return Channel{}, ChannelConfig{}, apperror.BadRequest("支付渠道参数无效")
	}
	methods, err := json.Marshal(input.SupportedMethods)
	if err != nil {
		return Channel{}, ChannelConfig{}, apperror.BadRequest("支付方式参数无效")
	}
	for _, method := range input.SupportedMethods {
		if !enum.IsPaymentMethod(method) {
			return Channel{}, ChannelConfig{}, apperror.BadRequest("支付方式参数无效")
		}
	}
	if requirePrivateKey && strings.TrimSpace(input.PrivateKey) == "" {
		return Channel{}, ChannelConfig{}, apperror.BadRequest("支付私钥不能为空")
	}
	cfg := ChannelConfig{
		AppID: strings.TrimSpace(input.AppID), MerchantID: strings.TrimSpace(input.MerchantID), SignType: "RSA2",
		IsSandbox: input.IsSandbox, NotifyURL: strings.TrimSpace(input.NotifyURL), ReturnURL: strings.TrimSpace(input.ReturnURL),
		AppCertPath: strings.TrimSpace(input.AppCertPath), AlipayCertPath: strings.TrimSpace(input.AlipayCertPath), AlipayRootCertPath: strings.TrimSpace(input.AlipayRootCertPath),
	}
	if privateKey := strings.TrimSpace(input.PrivateKey); privateKey != "" {
		encrypted, err := s.secretbox.Encrypt(privateKey)
		if err != nil {
			return Channel{}, ChannelConfig{}, apperror.Wrap(apperror.CodeInternal, 500, "加密支付私钥失败", err)
		}
		cfg.PrivateKeyEnc = encrypted
		cfg.PrivateKeyHint = secretbox.Hint(privateKey)
	}
	return Channel{Code: strings.TrimSpace(input.Code), Name: strings.TrimSpace(input.Name), Provider: strings.TrimSpace(input.Provider), Status: input.Status, SupportedMethods: string(methods), Remark: strings.TrimSpace(input.Remark), IsDel: enum.CommonNo}, cfg, nil
}

func (s *Service) enabledChannel(ctx context.Context, repo Repository, channelID int64, method string) (*Channel, *ChannelConfig, *apperror.Error) {
	channel, cfg, err := repo.FindEnabledChannel(ctx, channelID)
	if err != nil {
		return nil, nil, apperror.Wrap(apperror.CodeInternal, 500, "查询支付渠道失败", err)
	}
	if channel == nil || cfg == nil || channel.Status != enum.CommonYes || channel.Provider != enum.PaymentProviderAlipay {
		return nil, nil, apperror.BadRequest("支付渠道不可用")
	}
	if !methodSupported(channel.SupportedMethods, method) {
		return nil, nil, apperror.BadRequest("该渠道未配置当前支付方式")
	}
	return channel, cfg, nil
}

func (s *Service) gatewayConfig(cfg *ChannelConfig) (gateway.ChannelConfig, *apperror.Error) {
	privateKey, err := s.secretbox.Decrypt(cfg.PrivateKeyEnc)
	if err != nil {
		return gateway.ChannelConfig{}, apperror.Wrap(apperror.CodeInternal, 500, "解密支付私钥失败", err)
	}
	appCertPath, err := s.certResolver.Resolve(cfg.AppCertPath)
	if err != nil {
		return gateway.ChannelConfig{}, apperror.Wrap(apperror.CodeInternal, 500, "解析应用公钥证书路径失败", err)
	}
	alipayCertPath, err := s.certResolver.Resolve(cfg.AlipayCertPath)
	if err != nil {
		return gateway.ChannelConfig{}, apperror.Wrap(apperror.CodeInternal, 500, "解析支付宝公钥证书路径失败", err)
	}
	rootCertPath, err := s.certResolver.Resolve(cfg.AlipayRootCertPath)
	if err != nil {
		return gateway.ChannelConfig{}, apperror.Wrap(apperror.CodeInternal, 500, "解析支付宝根证书路径失败", err)
	}
	return gateway.ChannelConfig{ChannelID: cfg.ChannelID, AppID: cfg.AppID, PrivateKey: privateKey, AppCertPath: appCertPath, AlipayCertPath: alipayCertPath, RootCertPath: rootCertPath, NotifyURL: cfg.NotifyURL, IsSandbox: cfg.IsSandbox == enum.CommonYes}, nil
}

func (s *Service) getOrder(ctx context.Context, orderNo string) (*Order, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return nil, apperror.BadRequest("订单号不能为空")
	}
	order, err := repo.GetOrderByNo(ctx, orderNo)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询支付订单失败", err)
	}
	if order == nil {
		return nil, apperror.NotFound("支付订单不存在")
	}
	return order, nil
}

func (s *Service) closeOrder(ctx context.Context, order *Order) *apperror.Error {
	if order.Status != enum.PaymentOrderPending && order.Status != enum.PaymentOrderPaying {
		return apperror.BadRequest("该订单状态不允许关闭")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if ptrString(order.OutTradeNo) != "" {
		if err := s.closeRemote(ctx, repo, order); err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "关闭支付宝支付订单失败", err)
		}
	}
	if err := repo.MarkOrderClosed(ctx, order.ID, s.now()); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "关闭支付订单失败", err)
	}
	return nil
}

func (s *Service) queryAndMark(ctx context.Context, repo Repository, order *Order, now time.Time) (string, error) {
	if order == nil || order.OutTradeNo == nil {
		return "skipped", nil
	}
	_, cfg, appErr := s.enabledChannel(ctx, repo, order.ChannelID, order.PayMethod)
	if appErr != nil {
		return "skipped", appErr
	}
	payCfg, appErr := s.gatewayConfig(cfg)
	if appErr != nil {
		return "skipped", appErr
	}
	result, err := s.requireGateway().Query(ctx, payCfg, ptrString(order.OutTradeNo))
	if err != nil {
		return "deferred", err
	}
	if result != nil && result.PaidStatus() && result.AmountCents == order.AmountCents {
		if err := repo.MarkOrderSucceeded(ctx, order.ID, result.TradeNo, now); err != nil {
			return "deferred", err
		}
		return "paid", nil
	}
	return "unpaid", nil
}

func (s *Service) closeRemote(ctx context.Context, repo Repository, order *Order) error {
	_, cfg, appErr := s.enabledChannel(ctx, repo, order.ChannelID, order.PayMethod)
	if appErr != nil {
		return appErr
	}
	payCfg, appErr := s.gatewayConfig(cfg)
	if appErr != nil {
		return appErr
	}
	return s.requireGateway().Close(ctx, payCfg, ptrString(order.OutTradeNo))
}

func (s *Service) writeNotifyEvent(ctx context.Context, repo Repository, order *Order, outTradeNo string, form map[string]string, raw map[string]any, status int, cause error) error {
	message := ""
	if cause != nil {
		message = truncateError(cause)
	}
	return repo.CreateEvent(ctx, Event{OrderNo: order.OrderNo, OutTradeNo: outTradeNo, EventType: enum.PaymentEventNotify, Provider: order.Provider, RequestData: mustJSON(sanitizeStringMap(form)), ResponseData: mustJSON(sanitizeMap(raw)), ProcessStatus: status, ErrorMessage: message, CreatedAt: s.now()})
}

func validateNotify(order *Order, cfg *ChannelConfig, result *gateway.NotifyResult) error {
	if result == nil {
		return errors.New("支付宝回调结果为空")
	}
	if strings.TrimSpace(result.OutTradeNo) == "" || result.OutTradeNo != ptrString(order.OutTradeNo) {
		return errors.New("支付宝回调支付单号不匹配")
	}
	if strings.TrimSpace(result.AppID) != strings.TrimSpace(cfg.AppID) {
		return errors.New("支付宝回调应用ID不匹配")
	}
	if result.AmountCents != order.AmountCents {
		return errors.New("支付宝回调金额不匹配")
	}
	if !result.PaidStatus() {
		return errors.New("支付宝回调交易状态未成功")
	}
	return nil
}

func channelListItem(row Channel, cfg *ChannelConfig) ChannelListItem {
	methods := parseMethods(row.SupportedMethods)
	item := ChannelListItem{ID: row.ID, Code: row.Code, Name: row.Name, Provider: row.Provider, ProviderText: enum.PaymentProviderLabels[row.Provider], SupportedMethods: methods, SupportedText: methodText(methods), Status: row.Status, StatusText: commonStatusText(row.Status), Remark: row.Remark, CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
	if cfg != nil {
		item.AppID = cfg.AppID
		item.MerchantID = cfg.MerchantID
		item.NotifyURL = cfg.NotifyURL
		item.ReturnURL = cfg.ReturnURL
		item.PrivateKeyHint = cfg.PrivateKeyHint
		item.AppCertPath = cfg.AppCertPath
		item.AlipayCertPath = cfg.AlipayCertPath
		item.AlipayRootCertPath = cfg.AlipayRootCertPath
		item.IsSandbox = cfg.IsSandbox
	}
	return item
}

func orderListItem(row Order) OrderListItem {
	return OrderListItem{ID: row.ID, OrderNo: row.OrderNo, UserID: row.UserID, ChannelID: row.ChannelID, Provider: row.Provider, PayMethod: row.PayMethod, Subject: row.Subject, AmountCents: row.AmountCents, Status: row.Status, StatusText: enum.PaymentOrderStatusLabels[row.Status], OutTradeNo: ptrString(row.OutTradeNo), TradeNo: row.TradeNo, PaidAt: formatOptionalTime(row.PaidAt), ExpiredAt: formatTime(row.ExpiredAt), ClosedAt: formatOptionalTime(row.ClosedAt), CreatedAt: formatTime(row.CreatedAt)}
}

func eventListItem(row Event) EventListItem {
	return EventListItem{ID: row.ID, OrderNo: row.OrderNo, OutTradeNo: row.OutTradeNo, EventType: row.EventType, EventTypeText: enum.PaymentEventTypeLabels[row.EventType], Provider: row.Provider, ProcessStatus: row.ProcessStatus, ProcessText: enum.PaymentEventProcessStatusLabels[row.ProcessStatus], ErrorMessage: row.ErrorMessage, CreatedAt: formatTime(row.CreatedAt)}
}

func resultResponse(order *Order) *ResultResponse {
	return &ResultResponse{OrderNo: order.OrderNo, Status: order.Status, StatusText: enum.PaymentOrderStatusLabels[order.Status], TradeNo: order.TradeNo, PaidAt: formatOptionalTime(order.PaidAt)}
}

func validateOrderDateRange(start, end string) *apperror.Error {
	for _, value := range []string{strings.TrimSpace(start), strings.TrimSpace(end)} {
		if value == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", value); err != nil {
			return apperror.BadRequest("日期格式必须为 YYYY-MM-DD")
		}
	}
	return nil
}

func methodSupported(raw string, method string) bool {
	method = strings.TrimSpace(method)
	if !enum.IsPaymentMethod(method) {
		return false
	}
	if strings.TrimSpace(raw) == method {
		return true
	}
	for _, item := range parseMethods(raw) {
		if item == method {
			return true
		}
	}
	return false
}

func parseMethods(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var methods []string
	if err := json.Unmarshal([]byte(raw), &methods); err != nil {
		return nil
	}
	return methods
}

func methodText(methods []string) string {
	parts := make([]string, 0, len(methods))
	for _, method := range methods {
		parts = append(parts, enum.PaymentMethodLabels[method])
	}
	return strings.Join(parts, "、")
}

func commonStatusText(status int) string {
	if status == enum.CommonYes {
		return "启用"
	}
	if status == enum.CommonNo {
		return "禁用"
	}
	return ""
}

func currentPage(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func totalPage(total int64, pageSize int) int {
	if pageSize <= 0 || total <= 0 {
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

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTime(*value)
}

func ptrString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func mustJSON(value map[string]any) string {
	if value == nil {
		return "{}"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func parseJSONMap(value string) map[string]any {
	var out map[string]any
	if err := json.Unmarshal([]byte(value), &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func sanitizeStringMap(in map[string]string) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		if isSensitiveKey(key) {
			out[key] = "***"
			continue
		}
		out[key] = value
	}
	return out
}

func sanitizeMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		if isSensitiveKey(key) {
			out[key] = "***"
			continue
		}
		out[key] = value
	}
	return out
}

func isSensitiveKey(key string) bool {
	key = strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(key)))
	return strings.Contains(key, "privatekey") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "token") ||
		strings.Contains(key, "sign") ||
		strings.Contains(key, "password")
}

func truncateError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	runes := []rune(message)
	if len(runes) > 500 {
		return string(runes[:500])
	}
	return message
}

type nilGateway struct{}

func (nilGateway) CreatePagePay(ctx context.Context, cfg gateway.ChannelConfig, req gateway.CreatePayRequest) (*gateway.CreatePayResult, error) {
	return nil, ErrGatewayNotConfigured
}
func (nilGateway) Query(ctx context.Context, cfg gateway.ChannelConfig, outTradeNo string) (*gateway.QueryResult, error) {
	return nil, ErrGatewayNotConfigured
}
func (nilGateway) VerifyNotify(ctx context.Context, cfg gateway.ChannelConfig, form map[string]string) (*gateway.NotifyResult, error) {
	return nil, ErrGatewayNotConfigured
}
func (nilGateway) Close(ctx context.Context, cfg gateway.ChannelConfig, outTradeNo string) error {
	return ErrGatewayNotConfigured
}
func (nilGateway) SuccessBody() string { return "success" }
func (nilGateway) FailureBody() string { return "fail" }
