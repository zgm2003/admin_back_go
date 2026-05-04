package user

type listRequest struct {
	CurrentPage   int    `form:"current_page" binding:"required,min=1"`
	PageSize      int    `form:"page_size" binding:"required,min=1,max=50"`
	Keyword       string `form:"keyword" binding:"omitempty,max=100"`
	Username      string `form:"username" binding:"omitempty,max=64"`
	Email         string `form:"email" binding:"omitempty,max=255"`
	DetailAddress string `form:"detail_address" binding:"omitempty,max=255"`
	AddressID     string `form:"address_id" binding:"omitempty"`
	RoleID        *int64 `form:"role_id" binding:"omitempty,gt=0"`
	Sex           *int   `form:"sex" binding:"omitempty,user_sex"`
	Date          string `form:"date" binding:"omitempty"`
	DateStart     string `form:"date_start" binding:"omitempty"`
	DateEnd       string `form:"date_end" binding:"omitempty"`
}

type updateRequest struct {
	Username      string `json:"username" binding:"required,max=64"`
	Avatar        string `json:"avatar" binding:"omitempty,max=255"`
	RoleID        int64  `json:"role_id" binding:"required,gt=0"`
	Sex           int    `json:"sex" binding:"user_sex"`
	AddressID     *int64 `json:"address_id" binding:"required,min=0"`
	DetailAddress string `json:"detail_address" binding:"omitempty,max=255"`
	Bio           string `json:"bio" binding:"omitempty,max=1000"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}

type deleteBatchRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1,dive,gt=0"`
}

type batchProfileRequest struct {
	IDs           []int64           `json:"ids" binding:"required,min=1,dive,gt=0"`
	Field         BatchProfileField `json:"field" binding:"required"`
	Sex           *int              `json:"sex" binding:"omitempty,user_sex"`
	AddressID     *int64            `json:"address_id" binding:"omitempty,gt=0"`
	DetailAddress string            `json:"detail_address" binding:"omitempty,max=255"`
}
