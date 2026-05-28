package payment

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"admin_back_go/internal/enum"
	gateway "admin_back_go/internal/infra/payment"
)

func TestHandleAlipayCallbackSuccessFinalizesRechargeAndAudits(t *testing.T) {
	repo := newFakeRechargeRepo()
	repo.configs = []Config{enabledRechargeConfig(1, "alipay_default", 1, []string{enum.PaymentMethodWeb})}
	repo.order = &Order{ID: 1, OrderNo: "PAY20260521100000000000", ConfigID: 1, ConfigCode: "alipay_default", Provider: providerAlipay, AmountCents: 1000, Status: orderStatusPaying, IsDel: enum.CommonNo}
	repo.recharge = &Recharge{ID: 1, RechargeNo: "RCG20260521100000000000", UserID: 7, PaymentOrderID: repo.order.ID, Status: rechargeStatusPaying, AmountCents: 1000, IsDel: enum.CommonNo}
	repo.wallet = &Wallet{ID: 1, UserID: 7, IsDel: enum.CommonNo}
	gw := &fakeOrderGateway{notifyPayload: &gateway.NotifyPayload{NotifyID: "notify-1", OutTradeNo: repo.order.OrderNo, TradeNo: "202605212200", TradeStatus: "TRADE_SUCCESS", AppID: repo.configs[0].AppID, TotalAmountCents: 1000, Raw: map[string]string{"out_trade_no": repo.order.OrderNo}}}
	service := newRechargeService(repo, gw)

	result, appErr := service.HandleAlipayCallback(context.Background(), AlipayCallbackInput{Form: callbackForm(repo.order.OrderNo, "10.00")})
	if appErr != nil {
		t.Fatalf("HandleAlipayCallback error=%v", appErr)
	}
	if result == nil || result.Text != callbackResultSuccess {
		t.Fatalf("expected success text, got %#v", result)
	}
	if repo.order.Status != orderStatusPaid || repo.recharge.Status != rechargeStatusCredited || repo.creditCount != 1 {
		t.Fatalf("expected paid/credited once, order=%#v recharge=%#v credit=%d", repo.order, repo.recharge, repo.creditCount)
	}
	if repo.callbackEvent.ProcessStatus != callbackProcessSuccess || repo.callbackEvent.SignatureValid != enum.CommonYes {
		t.Fatalf("expected success callback audit, got %#v", repo.callbackEvent)
	}
}

func TestHandleAlipayCallbackInvalidSignatureReturnsFailWithoutMutation(t *testing.T) {
	repo := newFakeRechargeRepo()
	repo.configs = []Config{enabledRechargeConfig(1, "alipay_default", 1, []string{enum.PaymentMethodWeb})}
	repo.order = &Order{ID: 1, OrderNo: "PAY20260521100000000000", ConfigID: 1, ConfigCode: "alipay_default", Provider: providerAlipay, AmountCents: 1000, Status: orderStatusPaying, IsDel: enum.CommonNo}
	gw := &fakeOrderGateway{notifyErr: errors.New("bad sign")}
	service := newRechargeService(repo, gw)

	result, appErr := service.HandleAlipayCallback(context.Background(), AlipayCallbackInput{Form: callbackForm(repo.order.OrderNo, "10.00")})
	if appErr != nil {
		t.Fatalf("HandleAlipayCallback should return plain fail result, got appErr=%v", appErr)
	}
	if result == nil || result.Text != callbackResultFail || repo.order.Status != orderStatusPaying {
		t.Fatalf("expected fail without mutation, result=%#v order=%#v", result, repo.order)
	}
	if repo.callbackEvent.ProcessStatus != callbackProcessFailed || repo.callbackEvent.SignatureValid != enum.CommonNo {
		t.Fatalf("expected failed audit, got %#v", repo.callbackEvent)
	}
}

func TestHandleAlipayCallbackUnknownOrderIsIgnoredSuccess(t *testing.T) {
	repo := newFakeRechargeRepo()
	repo.configs = []Config{enabledRechargeConfig(1, "alipay_default", 1, []string{enum.PaymentMethodWeb})}
	repo.order = nil
	gw := &fakeOrderGateway{notifyPayload: &gateway.NotifyPayload{NotifyID: "notify-1", OutTradeNo: "PAY20260521999999999999", TradeNo: "202605212200", TradeStatus: "TRADE_SUCCESS", AppID: repo.configs[0].AppID, TotalAmountCents: 1000, Raw: map[string]string{"out_trade_no": "PAY20260521999999999999"}}}
	service := newRechargeService(repo, gw)

	result, appErr := service.HandleAlipayCallback(context.Background(), AlipayCallbackInput{Form: callbackForm("PAY20260521999999999999", "10.00")})
	if appErr != nil || result == nil || result.Text != callbackResultSuccess {
		t.Fatalf("unknown order should return success without retry storm result=%#v err=%v", result, appErr)
	}
	if repo.callbackEvent.ProcessStatus != callbackProcessIgnored || !strings.Contains(repo.callbackEvent.ProcessMessage, "不存在") {
		t.Fatalf("expected ignored audit, got %#v", repo.callbackEvent)
	}
}

func callbackForm(outTradeNo string, amount string) url.Values {
	form := url.Values{}
	form.Set("notify_id", "notify-1")
	form.Set("out_trade_no", outTradeNo)
	form.Set("trade_no", "202605212200")
	form.Set("trade_status", "TRADE_SUCCESS")
	form.Set("app_id", "2026000000000000")
	form.Set("total_amount", amount)
	form.Set("sign", "signature")
	form.Set("sign_type", "RSA2")
	return form
}
