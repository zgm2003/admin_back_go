package enum

const (
	NotificationTypeInfo    = 1
	NotificationTypeSuccess = 2
	NotificationTypeWarning = 3
	NotificationTypeError   = 4

	NotificationLevelNormal = 1
	NotificationLevelUrgent = 2
)

var NotificationTypeLabels = map[int]string{
	NotificationTypeInfo:    "普通",
	NotificationTypeSuccess: "成功",
	NotificationTypeWarning: "警告",
	NotificationTypeError:   "错误",
}

var NotificationLevelLabels = map[int]string{
	NotificationLevelNormal: "普通",
	NotificationLevelUrgent: "紧急",
}

func IsNotificationType(value int) bool {
	_, ok := NotificationTypeLabels[value]
	return ok
}

func IsNotificationLevel(value int) bool {
	_, ok := NotificationLevelLabels[value]
	return ok
}
