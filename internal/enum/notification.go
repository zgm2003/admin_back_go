package enum

const (
	NotificationTypeInfo    = 1
	NotificationTypeSuccess = 2
	NotificationTypeWarning = 3
	NotificationTypeError   = 4

	NotificationLevelNormal = 1
	NotificationLevelUrgent = 2

	NotificationTargetAll   = 1
	NotificationTargetUsers = 2
	NotificationTargetRoles = 3

	NotificationTaskStatusPending = 1
	NotificationTaskStatusSending = 2
	NotificationTaskStatusSuccess = 3
	NotificationTaskStatusFailed  = 4
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

var NotificationTargetTypeLabels = map[int]string{
	NotificationTargetAll:   "全部用户",
	NotificationTargetUsers: "指定用户",
	NotificationTargetRoles: "指定角色",
}

var NotificationTaskStatusLabels = map[int]string{
	NotificationTaskStatusPending: "待发送",
	NotificationTaskStatusSending: "发送中",
	NotificationTaskStatusSuccess: "已完成",
	NotificationTaskStatusFailed:  "失败",
}

func IsNotificationType(value int) bool {
	_, ok := NotificationTypeLabels[value]
	return ok
}

func IsNotificationLevel(value int) bool {
	_, ok := NotificationLevelLabels[value]
	return ok
}

func IsNotificationTargetType(value int) bool {
	_, ok := NotificationTargetTypeLabels[value]
	return ok
}

func IsNotificationTaskStatus(value int) bool {
	_, ok := NotificationTaskStatusLabels[value]
	return ok
}
