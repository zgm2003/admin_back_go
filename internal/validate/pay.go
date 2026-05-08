package validate

import (
	"admin_back_go/internal/enum"

	playground "github.com/go-playground/validator/v10"
)

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
