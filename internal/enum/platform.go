package enum

const (
	PlatformAdmin = "admin"
	PlatformApp   = "app"
)

var Platforms = []string{
	PlatformAdmin,
	PlatformApp,
}

func IsPlatform(value string) bool {
	for _, item := range Platforms {
		if value == item {
			return true
		}
	}
	return false
}
