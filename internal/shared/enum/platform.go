package enum

const (
	PlatformAll   = "all"
	PlatformAdmin = "admin"
	PlatformApp   = "app"
)

var NotificationTaskPlatforms = []string{
	PlatformAll,
	PlatformAdmin,
	PlatformApp,
}

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

func IsNotificationTaskPlatform(value string) bool {
	for _, item := range NotificationTaskPlatforms {
		if value == item {
			return true
		}
	}
	return false
}
