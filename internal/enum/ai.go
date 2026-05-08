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
	AIRunStatusRunning  = 1
	AIRunStatusSuccess  = 2
	AIRunStatusFail     = 3
	AIRunStatusCanceled = 4
)

var AIRunStatuses = []int{AIRunStatusRunning, AIRunStatusSuccess, AIRunStatusFail, AIRunStatusCanceled}
var AIRunStatusLabels = map[int]string{AIRunStatusRunning: "运行中", AIRunStatusSuccess: "成功", AIRunStatusFail: "失败", AIRunStatusCanceled: "已取消"}

const (
	AIRunStepTypePrompt     = 1
	AIRunStepTypeRAG        = 2
	AIRunStepTypeLLM        = 3
	AIRunStepTypeToolCall   = 4
	AIRunStepTypeToolResult = 5
	AIRunStepTypeFinalize   = 6
	AIRunStepTypeImage      = 7
)

var AIRunStepTypes = []int{AIRunStepTypePrompt, AIRunStepTypeRAG, AIRunStepTypeLLM, AIRunStepTypeToolCall, AIRunStepTypeToolResult, AIRunStepTypeFinalize, AIRunStepTypeImage}
var AIRunStepTypeLabels = map[int]string{AIRunStepTypePrompt: "提示词构建", AIRunStepTypeRAG: "RAG检索", AIRunStepTypeLLM: "LLM调用", AIRunStepTypeToolCall: "工具调用", AIRunStepTypeToolResult: "工具返回", AIRunStepTypeFinalize: "最终化", AIRunStepTypeImage: "图片生成"}

const (
	AIRunStepStatusSuccess = 1
	AIRunStepStatusFail    = 2
)

var AIRunStepStatuses = []int{AIRunStepStatusSuccess, AIRunStepStatusFail}
var AIRunStepStatusLabels = map[int]string{AIRunStepStatusSuccess: "成功", AIRunStepStatusFail: "失败"}

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

func IsAIMessageRole(value int) bool   { return intIn(value, AIMessageRoles) }
func IsAIRunStatus(value int) bool     { return intIn(value, AIRunStatuses) }
func IsAIRunStepType(value int) bool   { return intIn(value, AIRunStepTypes) }
func IsAIRunStepStatus(value int) bool { return intIn(value, AIRunStepStatuses) }

func intIn(value int, values []int) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
