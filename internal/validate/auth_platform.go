package validate

import (
	"admin_back_go/internal/enum"

	playground "github.com/go-playground/validator/v10"
)

func validateAuthPlatformLoginType(fl playground.FieldLevel) bool {
	return enum.IsLoginType(trimmedString(fl.Field()))
}

func validateCaptchaType(fl playground.FieldLevel) bool {
	return enum.IsCaptchaType(trimmedString(fl.Field()))
}
