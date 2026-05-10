package aiknowledge

type baseListRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Name        string `form:"name" binding:"omitempty,max=128"`
	Code        string `form:"code" binding:"omitempty,max=128"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
}

type baseMutationRequest struct {
	Name                   string  `json:"name" binding:"required,max=128"`
	Code                   string  `json:"code" binding:"required,max=128"`
	Description            string  `json:"description" binding:"omitempty,max=1024"`
	ChunkSizeChars         uint    `json:"chunk_size_chars" binding:"required,min=300,max=8000"`
	ChunkOverlapChars      uint    `json:"chunk_overlap_chars" binding:"omitempty,max=1000"`
	DefaultTopK            uint    `json:"default_top_k" binding:"required,min=1,max=20"`
	DefaultMinScore        float64 `json:"default_min_score" binding:"min=0,max=100"`
	DefaultMaxContextChars uint    `json:"default_max_context_chars" binding:"required,min=1000,max=30000"`
	Status                 int     `json:"status" binding:"required,common_status"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}

type documentListRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Title       string `form:"title" binding:"omitempty,max=191"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
}

type documentMutationRequest struct {
	Title      string `json:"title" binding:"required,max=191"`
	SourceType string `json:"source_type" binding:"omitempty,oneof=text markdown file"`
	SourceRef  string `json:"source_ref" binding:"omitempty,max=512"`
	Content    string `json:"content" binding:"required"`
	Status     int    `json:"status" binding:"required,common_status"`
}

type retrievalTestRequest struct {
	Query           string   `json:"query" binding:"required,max=4000"`
	TopK            uint     `json:"top_k" binding:"omitempty,min=1,max=20"`
	MinScore        *float64 `json:"min_score" binding:"omitempty,min=0,max=100"`
	MaxContextChars uint     `json:"max_context_chars" binding:"omitempty,min=1000,max=30000"`
}

type updateAgentKnowledgeBindingsRequest struct {
	Bindings []AgentKnowledgeBindingInput `json:"bindings" binding:"omitempty,dive"`
}
