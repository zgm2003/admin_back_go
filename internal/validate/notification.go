package validate

import (
	"admin_back_go/internal/enum"

	playground "github.com/go-playground/validator/v10"
)

func validateNotificationType(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsNotificationType(value)
}

func validateNotificationLevel(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsNotificationLevel(value)
}

func validateNotificationTargetType(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsNotificationTargetType(value)
}

func validateNotificationTaskStatus(fl playground.FieldLevel) bool {
	value, ok := intValue(fl.Field())
	return ok && enum.IsNotificationTaskStatus(value)
}

func validateNotificationTaskPlatform(fl playground.FieldLevel) bool {
	return enum.IsNotificationTaskPlatform(trimmedString(fl.Field()))
}
