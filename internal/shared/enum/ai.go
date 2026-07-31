package enum

const (
	AIMessageRoleUser      = 1
	AIMessageRoleAssistant = 2
	AIMessageRoleSystem    = 3
)

var AIMessageRoles = []int{AIMessageRoleUser, AIMessageRoleAssistant, AIMessageRoleSystem}
var AIMessageRoleLabels = map[int]string{AIMessageRoleUser: "user", AIMessageRoleAssistant: "assistant", AIMessageRoleSystem: "system"}

const (
	AIRunStatusRunning        = "running"
	AIRunStatusSuccess        = "success"
	AIRunStatusFailed         = "failed"
	AIRunStatusCanceled       = "canceled"
	AIRunStatusTimeout        = "timeout"
	AIRunStatusOutcomeUnknown = "outcome_unknown"
)

var AIRunStatuses = []string{AIRunStatusRunning, AIRunStatusSuccess, AIRunStatusFailed, AIRunStatusCanceled, AIRunStatusTimeout, AIRunStatusOutcomeUnknown}
var AIRunStatusLabels = map[string]string{AIRunStatusRunning: "运行中", AIRunStatusSuccess: "成功", AIRunStatusFailed: "失败", AIRunStatusCanceled: "已取消", AIRunStatusTimeout: "超时", AIRunStatusOutcomeUnknown: "结果未知"}

const (
	AIRunEventStart            = "start"
	AIRunEventCompleted        = "completed"
	AIRunEventFailed           = "failed"
	AIRunEventCanceled         = "canceled"
	AIRunEventTimeout          = "timeout"
	AIRunEventRetryScheduled   = "retry_scheduled"
	AIRunEventUsageRecorded    = "usage_recorded"
	AIRunEventOutcomeUnknown   = "outcome_unknown"
	AIRunEventSettled          = "settled"
	AIRunEventReleased         = "released"
	AIRunEventUnbilled         = "unbilled"
	AIRunEventFileMaterialized = "file_materialized_v1"
)

var AIRunEvents = []string{AIRunEventStart, AIRunEventCompleted, AIRunEventFailed, AIRunEventCanceled, AIRunEventTimeout, AIRunEventRetryScheduled, AIRunEventUsageRecorded, AIRunEventOutcomeUnknown, AIRunEventSettled, AIRunEventReleased, AIRunEventUnbilled, AIRunEventFileMaterialized}
var AIRunEventLabels = map[string]string{AIRunEventStart: "开始生成", AIRunEventCompleted: "生成完成", AIRunEventFailed: "生成失败", AIRunEventCanceled: "用户停止", AIRunEventTimeout: "运行超时", AIRunEventRetryScheduled: "已安排重试", AIRunEventUsageRecorded: "用量已记录", AIRunEventOutcomeUnknown: "结果未知", AIRunEventSettled: "已结算", AIRunEventReleased: "已释放", AIRunEventUnbilled: "未计费", AIRunEventFileMaterialized: "文件请求已物化"}

func IsAIMessageRole(value int) bool  { return intIn(value, AIMessageRoles) }
func IsAIRunStatus(value string) bool { return stringIn(value, AIRunStatuses) }
func IsAIRunEvent(value string) bool  { return stringIn(value, AIRunEvents) }

func intIn(value int, values []int) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func stringIn(value string, values []string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
