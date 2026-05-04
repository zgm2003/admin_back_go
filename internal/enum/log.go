package enum

const (
	LogLevelDebug    = "DEBUG"
	LogLevelInfo     = "INFO"
	LogLevelWarning  = "WARNING"
	LogLevelError    = "ERROR"
	LogLevelCritical = "CRITICAL"
)

var LogLevels = []string{
	LogLevelDebug,
	LogLevelInfo,
	LogLevelWarning,
	LogLevelError,
	LogLevelCritical,
}

func IsLogLevel(value string) bool {
	for _, item := range LogLevels {
		if value == item {
			return true
		}
	}
	return false
}

const (
	LogTail100  = 100
	LogTail300  = 300
	LogTail500  = 500
	LogTail1000 = 1000
	LogTail2000 = 2000
)

var LogTails = []int{
	LogTail100,
	LogTail300,
	LogTail500,
	LogTail1000,
	LogTail2000,
}

func IsLogTail(value int) bool {
	for _, item := range LogTails {
		if value == item {
			return true
		}
	}
	return false
}
