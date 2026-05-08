package enum

import "testing"

func TestPaymentEnums(t *testing.T) {
	if !IsPaymentProvider(PaymentProviderAlipay) || IsPaymentProvider("wechat") {
		t.Fatalf("unexpected provider validation")
	}
	if !IsPaymentMethod(PaymentMethodWeb) || !IsPaymentMethod(PaymentMethodH5) || IsPaymentMethod("scan") {
		t.Fatalf("unexpected method validation")
	}
	if !IsPaymentOrderStatus(PaymentOrderPending) || !IsPaymentOrderStatus(PaymentOrderSucceeded) || IsPaymentOrderStatus(99) {
		t.Fatalf("unexpected order status validation")
	}
	if !IsPaymentEventType(PaymentEventNotify) || !IsPaymentEventType(PaymentEventSync) || IsPaymentEventType("refund") {
		t.Fatalf("unexpected event type validation")
	}
	if !IsPaymentEventProcessStatus(PaymentEventIgnored) || IsPaymentEventProcessStatus(99) {
		t.Fatalf("unexpected event process status validation")
	}
}
