package validate

import (
	"admin_back_go/internal/enum"

	playground "github.com/go-playground/validator/v10"
)

func validateUploadDriver(fl playground.FieldLevel) bool {
	return enum.IsUploadDriver(trimmedString(fl.Field()))
}

func validateUploadImageExt(fl playground.FieldLevel) bool {
	return enum.IsUploadImageExt(trimmedString(fl.Field()))
}

func validateUploadFileExt(fl playground.FieldLevel) bool {
	return enum.IsUploadFileExt(trimmedString(fl.Field()))
}
