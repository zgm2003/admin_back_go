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
	AIRunModalityChat  = "chat"
	AIRunModalityText  = "text"
	AIRunModalityImage = "image"
	AIRunModalityVideo = "video"
)

var AIRunModalities = []string{AIRunModalityChat, AIRunModalityText, AIRunModalityImage, AIRunModalityVideo}
var AIRunModalityLabels = map[string]string{AIRunModalityChat: "对话", AIRunModalityText: "文本", AIRunModalityImage: "图片", AIRunModalityVideo: "视频"}

const (
	AIRunSourceChatMessage     = "ai_chat_message"
	AIRunSourceTextTask        = "ai_text_task"
	AIRunSourceImageTask       = "ai_image_task"
	AIRunSourceCanvasVideoTask = "canvas_video_task"
)

var AIRunSourceTypes = []string{AIRunSourceChatMessage, AIRunSourceTextTask, AIRunSourceImageTask, AIRunSourceCanvasVideoTask}
var AIRunSourceTypeLabels = map[string]string{AIRunSourceChatMessage: "AI对话消息", AIRunSourceTextTask: "AI文本任务", AIRunSourceImageTask: "AI图片任务", AIRunSourceCanvasVideoTask: "Canvas视频任务"}

const (
	AIRunUsagePending     = "pending"
	AIRunUsageReported    = "reported"
	AIRunUsageUnavailable = "unavailable"
)

var AIRunUsageStatuses = []string{AIRunUsagePending, AIRunUsageReported, AIRunUsageUnavailable}
var AIRunUsageStatusLabels = map[string]string{AIRunUsagePending: "等待用量", AIRunUsageReported: "已上报", AIRunUsageUnavailable: "未提供"}

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
func IsAIRunModality(value string) bool {
	return stringIn(value, AIRunModalities)
}
func IsAIRunSourceType(value string) bool {
	return stringIn(value, AIRunSourceTypes)
}
func IsAIRunUsageStatus(value string) bool {
	return stringIn(value, AIRunUsageStatuses)
}
func IsAIRunTerminalUsageStatus(value string) bool {
	return value == AIRunUsageReported || value == AIRunUsageUnavailable
}
func IsAIRunEvent(value string) bool { return stringIn(value, AIRunEvents) }

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
