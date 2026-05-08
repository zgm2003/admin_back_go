package aiknowledge

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Name        string `form:"name"`
	Visibility  string `form:"visibility"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
}
type mutationRequest struct {
	Name           string     `json:"name"`
	Description    *string    `json:"description"`
	Visibility     string     `json:"visibility"`
	PermissionJSON JSONObject `json:"permission_json"`
	ChunkSize      int        `json:"chunk_size"`
	ChunkOverlap   int        `json:"chunk_overlap"`
	TopK           int        `json:"top_k"`
	ScoreThreshold float64    `json:"score_threshold"`
	Status         int        `json:"status" binding:"omitempty,common_status"`
}
type statusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}
type documentListRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Title       string `form:"title"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
}
type documentMutationRequest struct {
	Title      string `json:"title"`
	SourceType string `json:"source_type"`
	Content    string `json:"content"`
	Status     int    `json:"status" binding:"omitempty,common_status"`
}
type chunkListRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	DocumentID  *int64 `form:"document_id" binding:"omitempty,min=1"`
}
type retrievalRequest struct {
	Query string `json:"query" binding:"required"`
	TopK  int    `json:"top_k" binding:"omitempty,min=1,max=20"`
}
