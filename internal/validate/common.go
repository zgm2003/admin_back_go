package validate

import (
	"admin_back_go/internal/enum"

	playground "github.com/go-playground/validator/v10"
)

func validateCommonYesNo(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsCommonYesNo(value)
}

func validateCommonStatus(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsCommonStatus(value)
}
