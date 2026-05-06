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
