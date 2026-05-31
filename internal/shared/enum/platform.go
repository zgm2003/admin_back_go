package enum

const (
	PlatformAll    = "all"
	PlatformAdmin  = "admin"
	PlatformApp    = "app"
	PlatformCanvas = "canvas"
)

var NotificationTaskPlatforms = []string{
	PlatformAll,
	PlatformAdmin,
	PlatformApp,
}

var Platforms = []string{
	PlatformAdmin,
	PlatformApp,
	PlatformCanvas,
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
