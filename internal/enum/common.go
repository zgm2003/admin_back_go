package enum

const (
	DefaultNull = "-"

	PageSizeMin = 1
	PageSizeMax = 50

	CommonYes = 1
	CommonNo  = 2
)

func IsCommonYesNo(value int) bool {
	return value == CommonYes || value == CommonNo
}

func IsCommonStatus(value int) bool {
	return IsCommonYesNo(value)
}
