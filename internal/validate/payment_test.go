package validate

import (
	"testing"

	"admin_back_go/internal/enum"

	"github.com/go-playground/validator/v10"
)

func TestPaymentMethodValidation(t *testing.T) {
	type payload struct {
		Provider string `validate:"payment_provider"`
		Method   string `validate:"payment_method"`
	}
	engine := validator.New()
	if err := engine.RegisterValidation("payment_provider", validatePaymentProvider); err != nil {
		t.Fatalf("register payment_provider: %v", err)
	}
	if err := engine.RegisterValidation("payment_method", validatePaymentMethod); err != nil {
		t.Fatalf("register payment_method: %v", err)
	}

	if err := engine.Struct(payload{Provider: enum.PaymentProviderAlipay, Method: enum.PaymentMethodWeb}); err != nil {
		t.Fatalf("valid payment provider/method rejected: %v", err)
	}
	if err := engine.Struct(payload{Provider: "wechat", Method: enum.PaymentMethodWeb}); err == nil {
		t.Fatalf("invalid payment provider accepted")
	}
	if err := engine.Struct(payload{Provider: enum.PaymentProviderAlipay, Method: "scan"}); err == nil {
		t.Fatalf("invalid payment method accepted")
	}
}
