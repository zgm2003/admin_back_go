package validate

import (
	"testing"

	"admin_back_go/internal/enum"

	playground "github.com/go-playground/validator/v10"
)

type payValidatorSample struct {
	Channel    int    `validate:"pay_channel"`
	Method     string `validate:"pay_method"`
	TxnStatus  int    `validate:"pay_txn_status"`
	OrderType  int    `validate:"pay_order_type"`
	PayStatus  int    `validate:"pay_status"`
	BizStatus  int    `validate:"pay_biz_status"`
	WalletType int    `validate:"wallet_type"`
	WalletSrc  int    `validate:"wallet_source"`
}

func TestPayValidators(t *testing.T) {
	validator := playground.New()
	if err := validator.RegisterValidation("pay_channel", validatePayChannel); err != nil {
		t.Fatalf("register pay_channel: %v", err)
	}
	if err := validator.RegisterValidation("pay_method", validatePayMethod); err != nil {
		t.Fatalf("register pay_method: %v", err)
	}
	if err := validator.RegisterValidation("pay_txn_status", validatePayTxnStatus); err != nil {
		t.Fatalf("register pay_txn_status: %v", err)
	}
	if err := validator.RegisterValidation("pay_order_type", validatePayOrderType); err != nil {
		t.Fatalf("register pay_order_type: %v", err)
	}
	if err := validator.RegisterValidation("pay_status", validatePayStatus); err != nil {
		t.Fatalf("register pay_status: %v", err)
	}
	if err := validator.RegisterValidation("pay_biz_status", validatePayBizStatus); err != nil {
		t.Fatalf("register pay_biz_status: %v", err)
	}
	if err := validator.RegisterValidation("wallet_type", validateWalletType); err != nil {
		t.Fatalf("register wallet_type: %v", err)
	}
	if err := validator.RegisterValidation("wallet_source", validateWalletSource); err != nil {
		t.Fatalf("register wallet_source: %v", err)
	}

	if err := validator.Struct(payValidatorSample{Channel: 1, Method: "scan", TxnStatus: enum.PayTxnSuccess, OrderType: enum.PayOrderRecharge, PayStatus: enum.PayStatusPaid, BizStatus: enum.PayBizSuccess, WalletType: enum.WalletTypeAdjust, WalletSrc: enum.WalletSourceManual}); err != nil {
		t.Fatalf("expected valid pay sample: %v", err)
	}
	if err := validator.Struct(payValidatorSample{Channel: 9, Method: "scan", TxnStatus: enum.PayTxnSuccess, OrderType: enum.PayOrderRecharge, PayStatus: enum.PayStatusPaid, BizStatus: enum.PayBizSuccess, WalletType: enum.WalletTypeAdjust, WalletSrc: enum.WalletSourceManual}); err == nil {
		t.Fatalf("expected invalid pay channel to fail")
	}
	if err := validator.Struct(payValidatorSample{Channel: 1, Method: "bank", TxnStatus: enum.PayTxnSuccess, OrderType: enum.PayOrderRecharge, PayStatus: enum.PayStatusPaid, BizStatus: enum.PayBizSuccess, WalletType: enum.WalletTypeAdjust, WalletSrc: enum.WalletSourceManual}); err == nil {
		t.Fatalf("expected invalid pay method to fail")
	}
	if err := validator.Struct(payValidatorSample{Channel: 1, Method: "scan", TxnStatus: 999, OrderType: enum.PayOrderRecharge, PayStatus: enum.PayStatusPaid, BizStatus: enum.PayBizSuccess, WalletType: enum.WalletTypeAdjust, WalletSrc: enum.WalletSourceManual}); err == nil {
		t.Fatalf("expected invalid pay transaction status to fail")
	}
	if err := validator.Struct(payValidatorSample{Channel: 1, Method: "scan", TxnStatus: enum.PayTxnSuccess, OrderType: 999, PayStatus: enum.PayStatusPaid, BizStatus: enum.PayBizSuccess, WalletType: enum.WalletTypeAdjust, WalletSrc: enum.WalletSourceManual}); err == nil {
		t.Fatalf("expected invalid pay order type to fail")
	}
	if err := validator.Struct(payValidatorSample{Channel: 1, Method: "scan", TxnStatus: enum.PayTxnSuccess, OrderType: enum.PayOrderRecharge, PayStatus: 999, BizStatus: enum.PayBizSuccess, WalletType: enum.WalletTypeAdjust, WalletSrc: enum.WalletSourceManual}); err == nil {
		t.Fatalf("expected invalid pay status to fail")
	}
	if err := validator.Struct(payValidatorSample{Channel: 1, Method: "scan", TxnStatus: enum.PayTxnSuccess, OrderType: enum.PayOrderRecharge, PayStatus: enum.PayStatusPaid, BizStatus: 999, WalletType: enum.WalletTypeAdjust, WalletSrc: enum.WalletSourceManual}); err == nil {
		t.Fatalf("expected invalid pay biz status to fail")
	}
	if err := validator.Struct(payValidatorSample{Channel: 1, Method: "scan", TxnStatus: enum.PayTxnSuccess, OrderType: enum.PayOrderRecharge, PayStatus: enum.PayStatusPaid, BizStatus: enum.PayBizSuccess, WalletType: 999, WalletSrc: enum.WalletSourceManual}); err == nil {
		t.Fatalf("expected invalid wallet type to fail")
	}
	if err := validator.Struct(payValidatorSample{Channel: 1, Method: "scan", TxnStatus: enum.PayTxnSuccess, OrderType: enum.PayOrderRecharge, PayStatus: enum.PayStatusPaid, BizStatus: enum.PayBizSuccess, WalletType: enum.WalletTypeAdjust, WalletSrc: 999}); err == nil {
		t.Fatalf("expected invalid wallet source to fail")
	}
}
