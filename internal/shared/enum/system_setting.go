package enum

const (
	SystemSettingValueString = 1
	SystemSettingValueNumber = 2
	SystemSettingValueBool   = 3
	SystemSettingValueJSON   = 4
)

var SystemSettingValueTypes = []int{
	SystemSettingValueString,
	SystemSettingValueNumber,
	SystemSettingValueBool,
	SystemSettingValueJSON,
}

func IsSystemSettingValueType(value int) bool {
	for _, item := range SystemSettingValueTypes {
		if value == item {
			return true
		}
	}
	return false
}
