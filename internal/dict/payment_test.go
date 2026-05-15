package dict

import (
	"testing"

	"admin_back_go/internal/enum"
)

func TestPaymentMethodOptions(t *testing.T) {
	methods := PaymentMethodOptions()
	if len(methods) != 2 || methods[0].Value != enum.PaymentMethodWeb || methods[0].Label != "PC网页支付" || methods[1].Value != enum.PaymentMethodH5 || methods[1].Label != "H5支付" {
		t.Fatalf("unexpected payment method options: %#v", methods)
	}
}
