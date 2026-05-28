package notification

import "time"

const timeLayout = "2006-01-02 15:04:05"

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}
