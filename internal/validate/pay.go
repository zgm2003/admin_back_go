package validate

import (
	"admin_back_go/internal/enum"

	playground "github.com/go-playground/validator/v10"
)

func validatePayChannel(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsPayChannel(value)
}

func validatePayMethod(fl playground.FieldLevel) bool {
	return enum.IsPayMethod(trimmedString(fl.Field()))
}

func validatePayTxnStatus(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsPayTxnStatus(value)
}

func validatePayNotifyType(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsNotifyType(value)
}

func validatePayNotifyProcessStatus(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsNotifyProcessStatus(value)
}

func validatePayOrderType(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsPayOrderType(value)
}

func validatePayStatus(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsPayStatus(value)
}

func validatePayBizStatus(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsPayBizStatus(value)
}

func validateWalletType(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsWalletType(value)
}

func validateWalletSource(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsWalletSource(value)
}

func validatePayReconcileStatus(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && value >= 1 && value <= 6
}

func validatePayReconcileBillType(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && value == 1
}

func validatePaymentProvider(fl playground.FieldLevel) bool {
	return enum.IsPaymentProvider(trimmedString(fl.Field()))
}

func validatePaymentMethod(fl playground.FieldLevel) bool {
	return enum.IsPaymentMethod(trimmedString(fl.Field()))
}

func validatePaymentOrderStatus(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsPaymentOrderStatus(value)
}

func validatePaymentEventType(fl playground.FieldLevel) bool {
	return enum.IsPaymentEventType(trimmedString(fl.Field()))
}

func validatePaymentEventProcessStatus(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsPaymentEventProcessStatus(value)
}
