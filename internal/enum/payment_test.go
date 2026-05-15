package enum

import "testing"

func TestPaymentMethodEnums(t *testing.T) {
	if !IsPaymentMethod(PaymentMethodWeb) || !IsPaymentMethod(PaymentMethodH5) || IsPaymentMethod("scan") {
		t.Fatalf("payment method enum validation mismatch")
	}
}
