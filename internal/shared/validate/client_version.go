package validate

import (
	"admin_back_go/internal/shared/enum"

	playground "github.com/go-playground/validator/v10"
)

func validateClientPlatform(fl playground.FieldLevel) bool {
	return enum.IsClientPlatform(trimmedString(fl.Field()))
}
