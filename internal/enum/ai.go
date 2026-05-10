package enum

const (
	AIDriverOpenAI      = "openai"
	AIDriverClaude      = "claude"
	AIDriverDeepSeek    = "deepseek"
	AIDriverGemini      = "gemini"
	AIDriverMistral     = "mistral"
	AIDriverCohere      = "cohere"
	AIDriverGrok        = "grok"
	AIDriverOllama      = "ollama"
	AIDriverHuggingFace = "huggingface"
	AIDriverQwen        = "qwen"
	AIDriverMoonshot    = "moonshot"
	AIDriverZhipu       = "zhipu"
	AIDriverHunyuan     = "hunyuan"
	AIDriverWenxin      = "wenxin"
)

var AIDrivers = []string{
	AIDriverOpenAI,
	AIDriverClaude,
	AIDriverDeepSeek,
	AIDriverGemini,
	AIDriverMistral,
	AIDriverCohere,
	AIDriverGrok,
	AIDriverOllama,
	AIDriverHuggingFace,
	AIDriverQwen,
	AIDriverMoonshot,
	AIDriverZhipu,
	AIDriverHunyuan,
	AIDriverWenxin,
}

var AIDriverLabels = map[string]string{
	AIDriverOpenAI:      "OpenAI",
	AIDriverClaude:      "Claude",
	AIDriverDeepSeek:    "DeepSeek",
	AIDriverGemini:      "Gemini",
	AIDriverMistral:     "Mistral",
	AIDriverCohere:      "Cohere",
	AIDriverGrok:        "Grok (xAI)",
	AIDriverOllama:      "Ollama (本地)",
	AIDriverHuggingFace: "HuggingFace",
	AIDriverQwen:        "通义千问",
	AIDriverMoonshot:    "Moonshot",
	AIDriverZhipu:       "智谱",
	AIDriverHunyuan:     "混元",
	AIDriverWenxin:      "文心一言",
}

const (
	AIModeChat     = "chat"
	AIModeRAG      = "rag"
	AIModeTool     = "tool"
	AIModeWorkflow = "workflow"
)

var AIModes = []string{
	AIModeChat,
	AIModeRAG,
	AIModeTool,
	AIModeWorkflow,
}

var AIModeLabels = map[string]string{
	AIModeChat:     "对话",
	AIModeRAG:      "RAG",
	AIModeTool:     "工具",
	AIModeWorkflow: "工作流",
}

const (
	AICapabilityChat     = "chat"
	AICapabilityTools    = "tools"
	AICapabilityRAG      = "rag"
	AICapabilityWorkflow = "workflow"
)

var AICapabilities = []string{
	AICapabilityTools,
	AICapabilityRAG,
	AICapabilityWorkflow,
}

var AICapabilityLabels = map[string]string{
	AICapabilityTools:    "工具调用",
	AICapabilityRAG:      "RAG知识库",
	AICapabilityWorkflow: "工作流编排",
}

const (
	AISceneGoodsScript  = "goods_script"
	AISceneCineProject  = "cine_project"
	AISceneCineKeyframe = "cine_keyframe"
)

var RetiredAIScenes = []string{
	AISceneGoodsScript,
	AISceneCineProject,
	AISceneCineKeyframe,
}

var RetiredAISceneLabels = map[string]string{
	AISceneGoodsScript:  "商品口播生成",
	AISceneCineProject:  "AI短剧工厂",
	AISceneCineKeyframe: "短剧分镜图片生成",
}

const (
	AIKnowledgeVisibilityPrivate = "private"
	AIKnowledgeVisibilityTeam    = "team"
	AIKnowledgeVisibilityPublic  = "public"
)

var AIKnowledgeVisibilities = []string{
	AIKnowledgeVisibilityPrivate,
	AIKnowledgeVisibilityTeam,
	AIKnowledgeVisibilityPublic,
}

var AIKnowledgeVisibilityLabels = map[string]string{
	AIKnowledgeVisibilityPrivate: "私有",
	AIKnowledgeVisibilityTeam:    "团队",
	AIKnowledgeVisibilityPublic:  "公开",
}

const (
	AIKnowledgeSourceManual = "manual"
	AIKnowledgeSourceText   = "text"
)

var AIKnowledgeSourceTypes = []string{
	AIKnowledgeSourceManual,
	AIKnowledgeSourceText,
}

var AIKnowledgeSourceTypeLabels = map[string]string{
	AIKnowledgeSourceManual: "手动录入",
	AIKnowledgeSourceText:   "文本",
}

const (
	AIKnowledgeIndexIndexed = 1
	AIKnowledgeIndexFailed  = 2
)

var AIKnowledgeIndexStatuses = []int{
	AIKnowledgeIndexIndexed,
	AIKnowledgeIndexFailed,
}

var AIKnowledgeIndexStatusLabels = map[int]string{
	AIKnowledgeIndexIndexed: "已索引",
	AIKnowledgeIndexFailed:  "索引失败",
}

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

const (
	AIExecutorInternal      = 1
	AIExecutorHTTPWhitelist = 2
	AIExecutorSQLReadonly   = 3
)

var AIExecutorTypes = []int{
	AIExecutorInternal,
	AIExecutorHTTPWhitelist,
	AIExecutorSQLReadonly,
}

var AIExecutorTypeLabels = map[int]string{
	AIExecutorInternal:      "内置函数",
	AIExecutorHTTPWhitelist: "HTTP白名单",
	AIExecutorSQLReadonly:   "只读SQL",
}

func IsAIDriver(value string) bool {
	for _, item := range AIDrivers {
		if item == value {
			return true
		}
	}
	return false
}

func IsAIExecutorType(value int) bool {
	for _, item := range AIExecutorTypes {
		if item == value {
			return true
		}
	}
	return false
}

func IsAIMode(value string) bool {
	for _, item := range AIModes {
		if item == value {
			return true
		}
	}
	return false
}

func IsAICapability(value string) bool {
	for _, item := range AICapabilities {
		if item == value {
			return true
		}
	}
	return false
}

func IsRetiredAIScene(value string) bool {
	for _, item := range RetiredAIScenes {
		if item == value {
			return true
		}
	}
	return false
}

func IsAIKnowledgeVisibility(value string) bool {
	for _, item := range AIKnowledgeVisibilities {
		if item == value {
			return true
		}
	}
	return false
}

func IsAIKnowledgeSourceType(value string) bool {
	for _, item := range AIKnowledgeSourceTypes {
		if item == value {
			return true
		}
	}
	return false
}

func IsAIKnowledgeIndexStatus(value int) bool {
	for _, item := range AIKnowledgeIndexStatuses {
		if item == value {
			return true
		}
	}
	return false
}

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
