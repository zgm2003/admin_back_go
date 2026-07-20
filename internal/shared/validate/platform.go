package validate

import (
	"regexp"

	"admin_back_go/internal/shared/enum"

	playground "github.com/go-playground/validator/v10"
)

var platformCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,48}$`)

func validatePlatformScope(fl playground.FieldLevel) bool {
	return enum.IsRegisteredPlatform(trimmedString(fl.Field()))
}

func validatePlatformCode(fl playground.FieldLevel) bool {
	return platformCodePattern.MatchString(trimmedString(fl.Field()))
}
