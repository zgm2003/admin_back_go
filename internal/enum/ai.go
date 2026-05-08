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
