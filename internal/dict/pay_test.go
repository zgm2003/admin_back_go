package dict

import (
	"testing"

	"admin_back_go/internal/enum"
)

func TestPayChannelOptionsUseEnumOrder(t *testing.T) {
	got := PayChannelOptions()
	if len(got) != 2 {
		t.Fatalf("unexpected options: %#v", got)
	}
	if got[0].Value != enum.PayChannelWechat || got[0].Label != "微信支付" {
		t.Fatalf("unexpected first channel option: %#v", got[0])
	}
	if got[1].Value != enum.PayChannelAlipay || got[1].Label != "支付宝" {
		t.Fatalf("unexpected second channel option: %#v", got[1])
	}
}

func TestPayMethodOptionsUseEnumOrder(t *testing.T) {
	got := PayMethodOptions()
	want := []string{enum.PayMethodWeb, enum.PayMethodH5, enum.PayMethodApp, enum.PayMethodMini, enum.PayMethodScan, enum.PayMethodMP}
	if len(got) != len(want) {
		t.Fatalf("unexpected method options: %#v", got)
	}
	for i, value := range want {
		if got[i].Value != value {
			t.Fatalf("expected method %q at %d, got %#v", value, i, got[i])
		}
	}
}

func TestPayMethodOptionsForChannel(t *testing.T) {
	got := PayMethodOptionsForChannel(enum.PayChannelWechat)
	values := make([]string, 0, len(got))
	for _, item := range got {
		values = append(values, item.Value)
	}
	want := []string{enum.PayMethodScan, enum.PayMethodH5, enum.PayMethodApp, enum.PayMethodMini, enum.PayMethodMP}
	if len(values) != len(want) {
		t.Fatalf("expected %#v, got %#v", want, values)
	}
	for i, value := range want {
		if values[i] != value {
			t.Fatalf("expected %#v, got %#v", want, values)
		}
	}
}

func TestPayTxnStatusOptions(t *testing.T) {
	options := PayTxnStatusOptions()
	if len(options) != len(enum.PayTxnStatuses) {
		t.Fatalf("len = %d, want %d", len(options), len(enum.PayTxnStatuses))
	}
	if options[0].Value != enum.PayTxnCreated || options[0].Label != enum.PayTxnStatusLabels[enum.PayTxnCreated] {
		t.Fatalf("first option = %#v", options[0])
	}
}

func TestPayNotifyOptions(t *testing.T) {
	notifyTypes := PayNotifyTypeOptions()
	if len(notifyTypes) != len(enum.NotifyTypes) || notifyTypes[0].Value != enum.NotifyPay || notifyTypes[0].Label != "支付回调" {
		t.Fatalf("unexpected notify type options: %#v", notifyTypes)
	}
	processStatuses := PayNotifyProcessStatusOptions()
	if len(processStatuses) != len(enum.NotifyProcessStatuses) || processStatuses[1].Value != enum.NotifyProcessSuccess || processStatuses[1].Label != "处理成功" {
		t.Fatalf("unexpected notify process status options: %#v", processStatuses)
	}
}

func TestPayOrderOptions(t *testing.T) {
	orderTypes := PayOrderTypeOptions()
	if len(orderTypes) != len(enum.PayOrderTypes) || orderTypes[0].Value != enum.PayOrderRecharge {
		t.Fatalf("unexpected order type options: %#v", orderTypes)
	}

	statuses := PayStatusOptions()
	if len(statuses) != len(enum.PayStatuses) || statuses[2].Value != enum.PayStatusPaid {
		t.Fatalf("unexpected pay status options: %#v", statuses)
	}

	bizStatuses := PayBizStatusOptions()
	if len(bizStatuses) != len(enum.PayBizStatuses) || bizStatuses[3].Value != enum.PayBizSuccess {
		t.Fatalf("unexpected biz status options: %#v", bizStatuses)
	}

	presets := RechargePresetOptions()
	if len(presets) != len(enum.RechargePresets) || presets[0].Value != 1000 || presets[0].Label != "10元" {
		t.Fatalf("unexpected recharge presets: %#v", presets)
	}
}

func TestWalletOptions(t *testing.T) {
	types := WalletTypeOptions()
	if len(types) != len(enum.WalletTypes) {
		t.Fatalf("unexpected wallet type options: %#v", types)
	}
	if types[0].Value != enum.WalletTypeRecharge || types[0].Label != enum.WalletTypeLabels[enum.WalletTypeRecharge] {
		t.Fatalf("unexpected first wallet type option: %#v", types[0])
	}
	if types[2].Value != enum.WalletTypeAdjust || types[2].Label != "系统调账" {
		t.Fatalf("unexpected adjust wallet type option: %#v", types[2])
	}

	sources := WalletSourceOptions()
	if len(sources) != len(enum.WalletSources) {
		t.Fatalf("unexpected wallet source options: %#v", sources)
	}
	if sources[0].Value != enum.WalletSourceNone || sources[0].Label != "未关联" {
		t.Fatalf("unexpected first wallet source option: %#v", sources[0])
	}
	if sources[2].Value != enum.WalletSourceManual || sources[2].Label != "人工" {
		t.Fatalf("unexpected manual wallet source option: %#v", sources[2])
	}
}
