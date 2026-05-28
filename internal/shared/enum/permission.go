package enum

const (
	PermissionTypeDir    = 1
	PermissionTypePage   = 2
	PermissionTypeButton = 3
)

var PermissionTypes = []int{
	PermissionTypeDir,
	PermissionTypePage,
	PermissionTypeButton,
}

func IsPermissionType(value int) bool {
	for _, item := range PermissionTypes {
		if value == item {
			return true
		}
	}
	return false
}
