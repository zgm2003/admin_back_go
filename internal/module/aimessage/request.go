package aimessage

type listRequest struct {
	BeforeID int64 `form:"before_id" binding:"omitempty,min=1"`
	Limit    int   `form:"limit" binding:"omitempty,min=1,max=100"`
}

type sendRequest struct {
	Content       string             `json:"content" binding:"max=20000"`
	RequestID     string             `json:"request_id" binding:"required,max=80"`
	Attachments   []Attachment       `json:"attachments" binding:"omitempty,max=5,dive"`
	RuntimeParams map[string]float64 `json:"runtime_params" binding:"omitempty"`
}

type cancelRequest struct {
	RequestID string `json:"request_id" binding:"required,max=80"`
}
