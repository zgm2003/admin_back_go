package aimodel

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Name        string `form:"name" binding:"max=50"`
	Driver      string `form:"driver" binding:"max=30"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
}

type mutationRequest struct {
	Name      string `json:"name" binding:"required,max=50"`
	Driver    string `json:"driver" binding:"required,max=30"`
	ModelCode string `json:"model_code" binding:"required,max=80"`
	Endpoint  string `json:"endpoint" binding:"max=255"`
	APIKey    string `json:"api_key"`
	Status    int    `json:"status" binding:"omitempty,common_status"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}
