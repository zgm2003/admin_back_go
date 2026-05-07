package enum

import (
	"reflect"
	"testing"
)

func TestPayChannelOrder(t *testing.T) {
	want := []int{PayChannelWechat, PayChannelAlipay}
	if len(PayChannels) != len(want) {
		t.Fatalf("unexpected pay channel count: %#v", PayChannels)
	}
	for i, value := range want {
		if PayChannels[i] != value {
			t.Fatalf("expected pay channel order %#v, got %#v", want, PayChannels)
		}
	}
}

func TestPayMethodOrder(t *testing.T) {
	want := []string{PayMethodWeb, PayMethodH5, PayMethodApp, PayMethodMini, PayMethodScan, PayMethodMP}
	if len(PayMethods) != len(want) {
		t.Fatalf("unexpected pay method count: %#v", PayMethods)
	}
	for i, value := range want {
		if PayMethods[i] != value {
			t.Fatalf("expected pay method order %#v, got %#v", want, PayMethods)
		}
	}
}

func TestNormalizePaySupportedMethods(t *testing.T) {
	got := NormalizePaySupportedMethods(PayChannelWechat, []string{"h5", "scan", "scan", "web", ""})
	want := []string{"h5", "scan"}
	if len(got) != len(want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
	for i, value := range want {
		if got[i] != value {
			t.Fatalf("expected %#v, got %#v", want, got)
		}
	}
}

func TestPaySupportedMethodsValid(t *testing.T) {
	tests := []struct {
		name    string
		channel int
		methods []string
		want    bool
	}{
		{name: "wechat scan h5", channel: PayChannelWechat, methods: []string{PayMethodScan, PayMethodH5}, want: true},
		{name: "wechat web unsupported", channel: PayChannelWechat, methods: []string{PayMethodWeb}, want: false},
		{name: "alipay web scan", channel: PayChannelAlipay, methods: []string{PayMethodWeb, PayMethodScan}, want: true},
		{name: "invalid channel", channel: 99, methods: []string{PayMethodWeb}, want: false},
		{name: "empty methods", channel: PayChannelAlipay, methods: []string{}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PaySupportedMethodsValid(tt.channel, tt.methods); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestPayTransactionStatusesAreStable(t *testing.T) {
	want := []int{PayTxnCreated, PayTxnWaiting, PayTxnSuccess, PayTxnFailed, PayTxnClosed}
	if !reflect.DeepEqual(PayTxnStatuses, want) {
		t.Fatalf("PayTxnStatuses = %#v, want %#v", PayTxnStatuses, want)
	}
	if PayTxnStatusLabels[PayTxnSuccess] != "支付成功" {
		t.Fatalf("unexpected success label: %q", PayTxnStatusLabels[PayTxnSuccess])
	}
	if IsPayTxnStatus(999) {
		t.Fatal("999 must not be a valid transaction status")
	}
}

func TestPayOrderEnumsAreStable(t *testing.T) {
	if !reflect.DeepEqual(PayOrderTypes, []int{PayOrderRecharge, PayOrderConsume, PayOrderGoods}) {
		t.Fatalf("unexpected order type order: %#v", PayOrderTypes)
	}
	if !reflect.DeepEqual(PayStatuses, []int{PayStatusPending, PayStatusPaying, PayStatusPaid, PayStatusClosed, PayStatusException}) {
		t.Fatalf("unexpected pay status order: %#v", PayStatuses)
	}
	if !reflect.DeepEqual(PayBizStatuses, []int{PayBizInit, PayBizPending, PayBizExecuting, PayBizSuccess, PayBizFailed, PayBizManual}) {
		t.Fatalf("unexpected biz status order: %#v", PayBizStatuses)
	}
	if PayOrderTypeLabels[PayOrderRecharge] != "充值" || PayStatusLabels[PayStatusPaid] != "已支付" || PayBizStatusLabels[PayBizSuccess] != "履约成功" {
		t.Fatalf("unexpected pay order labels")
	}
	if IsPayOrderType(999) || IsPayStatus(999) || IsPayBizStatus(999) {
		t.Fatalf("invalid pay order enums must be rejected")
	}
}

func TestWalletEnumsAreStable(t *testing.T) {
	if !reflect.DeepEqual(WalletTypes, []int{WalletTypeRecharge, WalletTypeConsume, WalletTypeAdjust}) {
		t.Fatalf("unexpected wallet type order: %#v", WalletTypes)
	}
	if WalletTypeLabels[WalletTypeRecharge] != "充值入账" || WalletTypeLabels[WalletTypeConsume] != "消费扣款" || WalletTypeLabels[WalletTypeAdjust] != "系统调账" {
		t.Fatalf("unexpected wallet type labels: %#v", WalletTypeLabels)
	}
	if !reflect.DeepEqual(WalletSources, []int{WalletSourceNone, WalletSourceFulfill, WalletSourceManual}) {
		t.Fatalf("unexpected wallet source order: %#v", WalletSources)
	}
	if WalletSourceLabels[WalletSourceNone] != "未关联" || WalletSourceLabels[WalletSourceFulfill] != "履约" || WalletSourceLabels[WalletSourceManual] != "人工" {
		t.Fatalf("unexpected wallet source labels: %#v", WalletSourceLabels)
	}
	if IsWalletType(999) || IsWalletSource(999) {
		t.Fatalf("invalid wallet enums must be rejected")
	}
}

func TestPayRuntimeEnumsAreStable(t *testing.T) {
	if !reflect.DeepEqual(FulfillStatuses, []int{FulfillPending, FulfillRunning, FulfillSuccess, FulfillFailed, FulfillManual}) {
		t.Fatalf("unexpected fulfill status order: %#v", FulfillStatuses)
	}
	if !reflect.DeepEqual(FulfillActions, []int{FulfillActionRecharge, FulfillActionConsume, FulfillActionGoods}) {
		t.Fatalf("unexpected fulfill action order: %#v", FulfillActions)
	}
	if !reflect.DeepEqual(NotifyTypes, []int{NotifyPay}) {
		t.Fatalf("unexpected notify type order: %#v", NotifyTypes)
	}
	if !reflect.DeepEqual(NotifyProcessStatuses, []int{NotifyProcessPending, NotifyProcessSuccess, NotifyProcessFailed, NotifyProcessIgnored}) {
		t.Fatalf("unexpected notify process status order: %#v", NotifyProcessStatuses)
	}
	if FulfillStatusLabels[FulfillSuccess] != "执行成功" ||
		FulfillActionLabels[FulfillActionRecharge] != "充值入账" ||
		NotifyTypeLabels[NotifyPay] != "支付回调" ||
		NotifyProcessStatusLabels[NotifyProcessSuccess] != "处理成功" {
		t.Fatalf("unexpected pay runtime labels")
	}
	if IsFulfillStatus(999) || IsFulfillAction(999) || IsNotifyType(999) || IsNotifyProcessStatus(999) {
		t.Fatalf("invalid pay runtime enums must be rejected")
	}
}
