package enum

const (
	AIMessageRoleUser      = 1
	AIMessageRoleAssistant = 2
	AIMessageRoleSystem    = 3
)

var AIMessageRoles = []int{AIMessageRoleUser, AIMessageRoleAssistant, AIMessageRoleSystem}
var AIMessageRoleLabels = map[int]string{AIMessageRoleUser: "user", AIMessageRoleAssistant: "assistant", AIMessageRoleSystem: "system"}

const (
	AIRunStatusRunning  = "running"
	AIRunStatusSuccess  = "success"
	AIRunStatusFailed   = "failed"
	AIRunStatusCanceled = "canceled"
	AIRunStatusTimeout  = "timeout"
)

var AIRunStatuses = []string{AIRunStatusRunning, AIRunStatusSuccess, AIRunStatusFailed, AIRunStatusCanceled, AIRunStatusTimeout}
var AIRunStatusLabels = map[string]string{AIRunStatusRunning: "运行中", AIRunStatusSuccess: "成功", AIRunStatusFailed: "失败", AIRunStatusCanceled: "已取消", AIRunStatusTimeout: "超时"}

const (
	AIRunEventStart     = "start"
	AIRunEventCompleted = "completed"
	AIRunEventFailed    = "failed"
	AIRunEventCanceled  = "canceled"
	AIRunEventTimeout   = "timeout"
)

var AIRunEvents = []string{AIRunEventStart, AIRunEventCompleted, AIRunEventFailed, AIRunEventCanceled, AIRunEventTimeout}
var AIRunEventLabels = map[string]string{AIRunEventStart: "开始生成", AIRunEventCompleted: "生成完成", AIRunEventFailed: "生成失败", AIRunEventCanceled: "用户停止", AIRunEventTimeout: "运行超时"}

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
