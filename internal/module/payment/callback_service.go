package payment

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strconv"
	"strings"

	gateway "admin_back_go/internal/infra/payment"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

func (s *Service) HandleAlipayCallback(ctx context.Context, input AlipayCallbackInput) (*AlipayCallbackResult, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return failCallbackResult(), nil
	}
	callbackRepo, ok := repo.(callbackRepository)
	if !ok {
		return failCallbackResult(), nil
	}
	receivedAt := s.now()
	rawPayload := callbackAuditPayloadJSON(input.Form)
	incomingEvent := CallbackEvent{
		Provider:         providerAlipay,
		DedupeKey:        callbackDedupeKey(providerAlipay, input.Form),
		NotifyID:         strings.TrimSpace(input.Form.Get("notify_id")),
		OutTradeNo:       strings.TrimSpace(input.Form.Get("out_trade_no")),
		TradeNo:          strings.TrimSpace(input.Form.Get("trade_no")),
		TradeStatus:      strings.TrimSpace(input.Form.Get("trade_status")),
		AppID:            strings.TrimSpace(input.Form.Get("app_id")),
		TotalAmountCents: mustParseCallbackAmount(input.Form.Get("total_amount")),
		SignatureValid:   enum.CommonNo,
		ProcessStatus:    callbackProcessPending,
		RawPayloadJSON:   rawPayload,
		ReceivedAt:       receivedAt,
		IsDel:            enum.CommonNo,
	}
	event, _, err := callbackRepo.AcquireCallbackEvent(ctx, incomingEvent)
	if err != nil {
		return failCallbackResult(), nil
	}
	if !callbackEventFactsEqual(event, &incomingEvent) {
		return failCallbackResult(), nil
	}
	if event.ProcessStatus == callbackProcessSuccess || event.ProcessStatus == callbackProcessIgnored {
		return successCallbackResult(), nil
	}
	resolve := func(signatureValid int, status string, message string, orderID int64, tradeNo string) (*AlipayCallbackResult, *apperror.Error) {
		processedAt := s.now()
		resolution := CallbackEventResolution{
			EventID:        event.ID,
			DedupeKey:      incomingEvent.DedupeKey,
			SignatureValid: signatureValid,
			ProcessStatus:  status,
			ProcessMessage: message,
			ProcessedAt:    processedAt,
		}
		if orderID > 0 {
			resolution.PaidOrderID = orderID
			resolution.AlipayTradeNo = strings.TrimSpace(tradeNo)
			resolution.PaidAt = processedAt
		}
		result, err := callbackRepo.ResolveCallbackEvent(ctx, resolution)
		if err != nil || result == nil || result.Event == nil {
			return failCallbackResult(), nil
		}
		if result.Event.ProcessStatus == callbackProcessFailed {
			return failCallbackResult(), nil
		}
		if result.Event.ProcessStatus == callbackProcessSuccess || result.Event.ProcessStatus == callbackProcessIgnored {
			return successCallbackResult(), nil
		}
		return failCallbackResult(), nil
	}

	rawPayloadForOrder := callbackPayloadFromForm(input.Form)
	order, err := repo.GetOrderByNo(ctx, rawPayloadForOrder.OutTradeNo)
	if err != nil {
		return resolve(enum.CommonNo, callbackProcessFailed, "查询支付订单失败", 0, "")
	}
	if order == nil {
		return resolve(enum.CommonNo, callbackProcessIgnored, "支付订单不存在", 0, "")
	}
	cfg, appErr := s.configByOrder(ctx, order)
	if appErr != nil {
		return resolve(enum.CommonNo, callbackProcessFailed, appErr.Message, 0, "")
	}
	platformCfg, appErr := s.gatewayConfigFromConfig(*cfg)
	if appErr != nil {
		return resolve(enum.CommonNo, callbackProcessFailed, appErr.Message, 0, "")
	}
	gw, appErr := s.requireGateway()
	if appErr != nil {
		return resolve(enum.CommonNo, callbackProcessFailed, appErr.Message, 0, "")
	}
	verified, verifyErr := gw.VerifyNotify(ctx, platformCfg, input.Form)
	if verifyErr != nil {
		return resolve(enum.CommonNo, callbackProcessFailed, verifyErr.Error(), 0, "")
	}
	if verified == nil {
		return resolve(enum.CommonNo, callbackProcessFailed, "支付宝回调为空", 0, "")
	}
	if strings.TrimSpace(verified.AppID) != strings.TrimSpace(cfg.AppID) {
		return resolve(enum.CommonYes, callbackProcessFailed, "支付宝应用ID不匹配", 0, "")
	}
	if verified.TotalAmountCents != order.AmountCents {
		return resolve(enum.CommonYes, callbackProcessFailed, "支付宝回调金额不匹配", 0, "")
	}
	if strings.TrimSpace(verified.OutTradeNo) != strings.TrimSpace(order.OrderNo) {
		return resolve(enum.CommonYes, callbackProcessFailed, "支付宝商户订单号不匹配", 0, "")
	}
	switch strings.TrimSpace(verified.TradeStatus) {
	case "TRADE_SUCCESS", "TRADE_FINISHED":
		tradeNo := strings.TrimSpace(verified.TradeNo)
		if tradeNo == "" {
			return resolve(enum.CommonYes, callbackProcessFailed, "支付宝交易号为空", 0, "")
		}
		return resolve(enum.CommonYes, callbackProcessSuccess, "credited", order.ID, tradeNo)
	case "WAIT_BUYER_PAY":
		return resolve(enum.CommonYes, callbackProcessIgnored, "支付宝交易仍待支付", 0, "")
	default:
		return resolve(enum.CommonYes, callbackProcessIgnored, "支付宝交易状态未完成", 0, "")
	}
}

func callbackDedupeKey(provider string, form map[string][]string) []byte {
	notifyID := strings.TrimSpace(firstFormValue(form, "notify_id"))
	parts := []string{"payment_callback_v1", strings.TrimSpace(provider)}
	if notifyID != "" {
		parts = append(parts, "notify_id", notifyID)
	} else {
		parts = append(parts,
			"callback_facts",
			strings.TrimSpace(firstFormValue(form, "out_trade_no")),
			strings.TrimSpace(firstFormValue(form, "trade_no")),
			strings.TrimSpace(firstFormValue(form, "trade_status")),
			strings.TrimSpace(firstFormValue(form, "app_id")),
			strings.TrimSpace(firstFormValue(form, "total_amount")),
		)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return sum[:]
}

func callbackPayloadFromForm(form map[string][]string) *gateway.NotifyPayload {
	return &gateway.NotifyPayload{
		NotifyID:         strings.TrimSpace(firstFormValue(form, "notify_id")),
		OutTradeNo:       strings.TrimSpace(firstFormValue(form, "out_trade_no")),
		TradeNo:          strings.TrimSpace(firstFormValue(form, "trade_no")),
		TradeStatus:      strings.TrimSpace(firstFormValue(form, "trade_status")),
		AppID:            strings.TrimSpace(firstFormValue(form, "app_id")),
		TotalAmountCents: mustParseCallbackAmount(firstFormValue(form, "total_amount")),
		Raw:              formFirstValues(form),
	}
}

func callbackAuditPayloadJSON(form map[string][]string) string {
	raw, err := json.Marshal(formFirstValuesLimited(form, 4096))
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func formFirstValues(form map[string][]string) map[string]string {
	return formFirstValuesLimited(form, 0)
}

func formFirstValuesLimited(form map[string][]string, maxValueRunes int) map[string]string {
	raw := make(map[string]string, len(form))
	for key, values := range form {
		if len(values) == 0 {
			continue
		}
		value := values[0]
		if maxValueRunes > 0 {
			value = trimMax(value, maxValueRunes)
		}
		raw[key] = value
	}
	return raw
}

func firstFormValue(form map[string][]string, key string) string {
	if len(form[key]) == 0 {
		return ""
	}
	return form[key][0]
}

func mustParseCallbackAmount(value string) int64 {
	cents, err := amountStringToCents(value)
	if err != nil {
		return 0
	}
	return cents
}

func amountStringToCents(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, apperror.BadRequestKey("payment.callback.amount.invalid", nil, "无效的支付宝回调金额")
	}
	if !asciiDigits(parts[0]) {
		return 0, apperror.BadRequestKey("payment.callback.amount.invalid", nil, "无效的支付宝回调金额")
	}
	yuan, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || yuan < 0 {
		return 0, apperror.BadRequestKey("payment.callback.amount.invalid", nil, "无效的支付宝回调金额")
	}
	var cents int64
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			return 0, apperror.BadRequestKey("payment.callback.amount.invalid", nil, "无效的支付宝回调金额")
		}
		if parts[1] != "" && !asciiDigits(parts[1]) {
			return 0, apperror.BadRequestKey("payment.callback.amount.invalid", nil, "无效的支付宝回调金额")
		}
		centText := (parts[1] + "00")[:2]
		cents, err = strconv.ParseInt(centText, 10, 64)
		if err != nil {
			return 0, apperror.BadRequestKey("payment.callback.amount.invalid", nil, "无效的支付宝回调金额")
		}
	}
	return yuan*100 + cents, nil
}

func asciiDigits(value string) bool {
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return value != ""
}

func successCallbackResult() *AlipayCallbackResult {
	return &AlipayCallbackResult{Text: callbackResultSuccess}
}
func failCallbackResult() *AlipayCallbackResult {
	return &AlipayCallbackResult{Text: callbackResultFail}
}
