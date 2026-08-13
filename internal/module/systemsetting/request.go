package systemsetting

type ListRequest struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=50"`
	Key         string `form:"key" binding:"max=100"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
}

type CreateRequest struct {
	Key    string `json:"key" binding:"required,max=100"`
	Value  string `json:"value"`
	Type   int    `json:"type" binding:"required,system_setting_value_type"`
	Remark string `json:"remark" binding:"max=255"`
}

type UpdateRequest struct {
	Value  string `json:"value"`
	Type   int    `json:"type" binding:"required,system_setting_value_type"`
	Remark string `json:"remark" binding:"max=255"`
}

type DeleteRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1,dive,gt=0"`
}

type StatusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}

// Kept only until the Admin HTTP surface moves into this package in Wave 02.
type ListQuery = ListRequest
type CreateInput = CreateRequest
type UpdateInput = UpdateRequest
