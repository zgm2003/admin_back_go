package dict

import (
	"testing"

	"admin_back_go/internal/enum"
)

func TestPaymentDict(t *testing.T) {
	providers := PaymentProviderOptions()
	if len(providers) != 1 || providers[0].Value != enum.PaymentProviderAlipay || providers[0].Label != "支付宝" {
		t.Fatalf("unexpected provider options: %#v", providers)
	}

	methods := PaymentMethodOptions()
	if len(methods) != 2 || methods[0].Value != enum.PaymentMethodWeb || methods[0].Label != "PC网页支付" || methods[1].Value != enum.PaymentMethodH5 || methods[1].Label != "H5支付" {
		t.Fatalf("unexpected method options: %#v", methods)
	}

	statuses := PaymentOrderStatusOptions()
	if len(statuses) != len(enum.PaymentOrderStatuses) || statuses[0].Value != enum.PaymentOrderPending || statuses[0].Label != "待支付" || statuses[2].Value != enum.PaymentOrderSucceeded || statuses[2].Label != "支付成功" {
		t.Fatalf("unexpected order status options: %#v", statuses)
	}

	eventTypes := PaymentEventTypeOptions()
	if len(eventTypes) != len(enum.PaymentEventTypes) || eventTypes[2].Value != enum.PaymentEventNotify || eventTypes[2].Label != "支付回调" || eventTypes[4].Value != enum.PaymentEventSync || eventTypes[4].Label != "定时同步" {
		t.Fatalf("unexpected event type options: %#v", eventTypes)
	}

	processStatuses := PaymentEventProcessStatusOptions()
	if len(processStatuses) != len(enum.PaymentEventProcessStatuses) || processStatuses[1].Value != enum.PaymentEventSuccess || processStatuses[1].Label != "成功" || processStatuses[3].Value != enum.PaymentEventIgnored || processStatuses[3].Label != "已忽略" {
		t.Fatalf("unexpected process status options: %#v", processStatuses)
	}
}
