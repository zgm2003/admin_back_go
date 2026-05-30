package admin

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Scene       string `form:"scene" binding:"omitempty,max=64"`
	Unit        string `form:"unit" binding:"omitempty,max=16"`
	Status      *int   `form:"status" binding:"omitempty,oneof=1 2"`
}

type createRequest struct {
	Scene          string `json:"scene" binding:"required,max=64"`
	Unit           string `json:"unit" binding:"required,max=16"`
	UnitPriceCents int64  `json:"unit_price_cents" binding:"required,min=1"`
	Status         int    `json:"status" binding:"required,oneof=1 2"`
}

type updateRequest struct {
	Unit           string `json:"unit" binding:"required,max=16"`
	UnitPriceCents int64  `json:"unit_price_cents" binding:"required,min=1"`
	Status         int    `json:"status" binding:"required,oneof=1 2"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,oneof=1 2"`
}
