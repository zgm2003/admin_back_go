package validate

import (
	"admin_back_go/internal/enum"

	playground "github.com/go-playground/validator/v10"
)

func validatePermissionType(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsPermissionType(value)
}
