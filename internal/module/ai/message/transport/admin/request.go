package admin

type listRequest struct {
	BeforeID int64 `form:"before_id" binding:"omitempty,min=1"`
	Limit    int   `form:"limit" binding:"omitempty,min=1,max=100"`
}

type sendRequest struct {
	Content       string              `json:"content" binding:"max=20000"`
	RequestID     string              `json:"request_id" binding:"required,max=128"`
	Attachments   []attachmentRequest `json:"attachments" binding:"omitempty,max=5,dive"`
	RuntimeParams map[string]float64  `json:"runtime_params" binding:"omitempty"`
}

type attachmentRequest struct {
	Type      string `json:"type" binding:"required,oneof=image file"`
	ObjectKey string `json:"object_key" binding:"required,max=1024"`
	MIMEType  string `json:"mime_type" binding:"required,max=255"`
	URL       string `json:"url" binding:"required,max=2048"`
	Name      string `json:"name" binding:"required,max=255"`
	Size      int64  `json:"size" binding:"required,gt=0"`
}

type cancelRequest struct {
	RequestID    string  `json:"request_id" binding:"required,max=128"`
	DeliveredSeq *uint32 `json:"delivered_seq" binding:"required"`
}

type revisionRequest struct {
	Content     string               `json:"content" binding:"required,max=20000"`
	RequestID   string               `json:"request_id" binding:"required,max=128"`
	Attachments *[]attachmentRequest `json:"attachments" binding:"omitempty,max=5,dive"`
}

type regenerationRequest struct {
	RequestID string `json:"request_id" binding:"required,max=128"`
}

type deleteMessagesRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1,dive,gt=0"`
}
