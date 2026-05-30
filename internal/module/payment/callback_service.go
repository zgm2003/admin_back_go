package payment

import (
	"context"
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
	receivedAt := s.now()
	rawPayload := callbackAuditPayloadJSON(input.Form)
	eventID, err := repo.CreateCallbackEvent(ctx, CallbackEvent{
		Provider:         providerAlipay,
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
	})
	if err != nil {
		eventID = 0
	}
	mark := func(signatureValid int, status string, message string) (*AlipayCallbackResult, *apperror.Error) {
		if eventID > 0 {
			_ = repo.UpdateCallbackEventProcessed(ctx, eventID, signatureValid, status, message, s.now())
		}
		if status == callbackProcessFailed {
			return failCallbackResult(), nil
		}
		return successCallbackResult(), nil
	}

	rawPayloadForOrder := callbackPayloadFromForm(input.Form)
	order, err := repo.GetOrderByNo(ctx, rawPayloadForOrder.OutTradeNo)
	if err != nil {
		return mark(enum.CommonNo, callbackProcessFailed, "查询支付订单失败")
	}
	if order == nil {
		return mark(enum.CommonNo, callbackProcessIgnored, "支付订单不存在")
	}
	cfg, appErr := s.configByOrder(ctx, order)
	if appErr != nil {
		return mark(enum.CommonNo, callbackProcessFailed, appErr.Message)
	}
	platformCfg, appErr := s.gatewayConfigFromConfig(*cfg)
	if appErr != nil {
		return mark(enum.CommonNo, callbackProcessFailed, appErr.Message)
	}
	gw, appErr := s.requireGateway()
	if appErr != nil {
		return mark(enum.CommonNo, callbackProcessFailed, appErr.Message)
	}
	verified, verifyErr := gw.VerifyNotify(ctx, platformCfg, input.Form)
	if verifyErr != nil {
		return mark(enum.CommonNo, callbackProcessFailed, verifyErr.Error())
	}
	if verified == nil {
		return mark(enum.CommonNo, callbackProcessFailed, "支付宝回调为空")
	}
	if strings.TrimSpace(verified.AppID) != strings.TrimSpace(cfg.AppID) {
		return mark(enum.CommonYes, callbackProcessFailed, "支付宝应用ID不匹配")
	}
	if verified.TotalAmountCents != order.AmountCents {
		return mark(enum.CommonYes, callbackProcessFailed, "支付宝回调金额不匹配")
	}
	switch strings.TrimSpace(verified.TradeStatus) {
	case "TRADE_SUCCESS", "TRADE_FINISHED":
		if _, appErr := s.FinalizeOrderPaid(ctx, order.ID, verified.TradeNo, s.now(), finalizeSourceCallback); appErr != nil {
			return mark(enum.CommonYes, callbackProcessFailed, appErr.Message)
		}
		return mark(enum.CommonYes, callbackProcessSuccess, "credited")
	case "WAIT_BUYER_PAY":
		return mark(enum.CommonYes, callbackProcessIgnored, "支付宝交易仍待支付")
	default:
		return mark(enum.CommonYes, callbackProcessIgnored, "支付宝交易状态未完成")
	}
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
	yuan, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || yuan < 0 {
		return 0, apperror.BadRequestKey("payment.callback.amount.invalid", nil, "无效的支付宝回调金额")
	}
	var cents int64
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
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

func successCallbackResult() *AlipayCallbackResult {
	return &AlipayCallbackResult{Text: callbackResultSuccess}
}
func failCallbackResult() *AlipayCallbackResult {
	return &AlipayCallbackResult{Text: callbackResultFail}
}
