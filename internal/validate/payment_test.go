package validate

import (
	"testing"

	"admin_back_go/internal/enum"

	"github.com/gin-gonic/gin/binding"
	playground "github.com/go-playground/validator/v10"
)

type paymentValidatorSample struct {
	Provider      string `validate:"payment_provider"`
	Method        string `validate:"payment_method"`
	OrderStatus   int    `validate:"payment_order_status"`
	EventType     string `validate:"payment_event_type"`
	ProcessStatus int    `validate:"payment_event_process_status"`
}

func TestPaymentValidators(t *testing.T) {
	validator := playground.New()
	validators := map[string]playground.Func{
		"payment_provider":             validatePaymentProvider,
		"payment_method":               validatePaymentMethod,
		"payment_order_status":         validatePaymentOrderStatus,
		"payment_event_type":           validatePaymentEventType,
		"payment_event_process_status": validatePaymentEventProcessStatus,
	}
	for tag, fn := range validators {
		if err := validator.RegisterValidation(tag, fn); err != nil {
			t.Fatalf("register %s: %v", tag, err)
		}
	}

	valid := paymentValidatorSample{
		Provider:      enum.PaymentProviderAlipay,
		Method:        enum.PaymentMethodWeb,
		OrderStatus:   enum.PaymentOrderSucceeded,
		EventType:     enum.PaymentEventNotify,
		ProcessStatus: enum.PaymentEventSuccess,
	}
	if err := validator.Struct(valid); err != nil {
		t.Fatalf("expected valid payment sample: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*paymentValidatorSample)
	}{
		{name: "invalid provider", mutate: func(sample *paymentValidatorSample) { sample.Provider = "wechat" }},
		{name: "invalid method", mutate: func(sample *paymentValidatorSample) { sample.Method = "scan" }},
		{name: "invalid order status", mutate: func(sample *paymentValidatorSample) { sample.OrderStatus = 99 }},
		{name: "invalid event type", mutate: func(sample *paymentValidatorSample) { sample.EventType = "refund" }},
		{name: "invalid process status", mutate: func(sample *paymentValidatorSample) { sample.ProcessStatus = 99 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sample := valid
			tc.mutate(&sample)
			if err := validator.Struct(sample); err == nil {
				t.Fatalf("expected %s to fail", tc.name)
			}
		})
	}
}

func TestRegisterAddsPaymentValidators(t *testing.T) {
	if err := Register(); err != nil {
		t.Fatalf("register validators: %v", err)
	}

	type request struct {
		Provider      string `binding:"required,payment_provider"`
		Method        string `binding:"required,payment_method"`
		OrderStatus   int    `binding:"required,payment_order_status"`
		EventType     string `binding:"required,payment_event_type"`
		ProcessStatus int    `binding:"required,payment_event_process_status"`
	}

	valid := request{
		Provider:      enum.PaymentProviderAlipay,
		Method:        enum.PaymentMethodH5,
		OrderStatus:   enum.PaymentOrderPaying,
		EventType:     enum.PaymentEventSync,
		ProcessStatus: enum.PaymentEventIgnored,
	}
	if err := binding.Validator.ValidateStruct(valid); err != nil {
		t.Fatalf("expected valid payment request: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*request)
	}{
		{name: "invalid provider", mutate: func(req *request) { req.Provider = "wechat" }},
		{name: "invalid method", mutate: func(req *request) { req.Method = "scan" }},
		{name: "invalid order status", mutate: func(req *request) { req.OrderStatus = 99 }},
		{name: "invalid event type", mutate: func(req *request) { req.EventType = "refund" }},
		{name: "invalid process status", mutate: func(req *request) { req.ProcessStatus = 99 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := valid
			tc.mutate(&req)
			if err := binding.Validator.ValidateStruct(req); err == nil {
				t.Fatalf("expected %s to fail", tc.name)
			}
		})
	}
}
