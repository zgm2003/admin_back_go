package enum

const (
	PlatformAll   = "all"
	PlatformAdmin = "admin"
)

var registeredPlatforms = [...]string{PlatformAdmin}

func RegisteredPlatforms() []string {
	return append([]string(nil), registeredPlatforms[:]...)
}

func IsRegisteredPlatform(value string) bool {
	for _, item := range registeredPlatforms {
		if value == item {
			return true
		}
	}
	return false
}

func NotificationAudiencePlatforms() []string {
	result := make([]string, 0, len(registeredPlatforms)+1)
	result = append(result, PlatformAll)
	return append(result, registeredPlatforms[:]...)
}

func IsNotificationAudiencePlatform(value string) bool {
	for _, item := range NotificationAudiencePlatforms() {
		if value == item {
			return true
		}
	}
	return false
}
