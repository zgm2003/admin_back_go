package validate

import (
	"admin_back_go/internal/enum"

	playground "github.com/go-playground/validator/v10"
)

func validatePaymentMethod(fl playground.FieldLevel) bool {
	return enum.IsPaymentMethod(trimmedString(fl.Field()))
}

func validatePaymentProvider(fl playground.FieldLevel) bool {
	return enum.IsPaymentProvider(trimmedString(fl.Field()))
}
