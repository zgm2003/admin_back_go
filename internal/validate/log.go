package validate

import (
	"strings"

	"admin_back_go/internal/enum"

	playground "github.com/go-playground/validator/v10"
)

func validateLogLevel(fl playground.FieldLevel) bool {
	return enum.IsLogLevel(strings.ToUpper(trimmedString(fl.Field())))
}
