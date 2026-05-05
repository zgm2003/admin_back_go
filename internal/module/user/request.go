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

type updateProfileRequest struct {
	Username      string  `json:"username" binding:"required,max=64"`
	Avatar        string  `json:"avatar" binding:"omitempty,max=255"`
	Sex           int     `json:"sex" binding:"user_sex"`
	Birthday      *string `json:"birthday" binding:"omitempty"`
	AddressID     *int64  `json:"address_id" binding:"required,min=0"`
	DetailAddress string  `json:"detail_address" binding:"omitempty,max=255"`
	Bio           string  `json:"bio" binding:"omitempty,max=1000"`
}

type updatePasswordRequest struct {
	VerifyType      string `json:"verify_type" binding:"required,user_verify_type"`
	OldPassword     string `json:"old_password" binding:"omitempty,max=128"`
	Account         string `json:"account" binding:"omitempty,max=255"`
	Code            string `json:"code" binding:"omitempty,max=16"`
	NewPassword     string `json:"new_password" binding:"required,min=6,max=128"`
	ConfirmPassword string `json:"confirm_password" binding:"required,min=6,max=128"`
}

type updateEmailRequest struct {
	Email string `json:"email" binding:"required,email,max=255"`
	Code  string `json:"code" binding:"required,max=16"`
}

type updatePhoneRequest struct {
	Phone string `json:"phone" binding:"required,max=32"`
	Code  string `json:"code" binding:"required,max=16"`
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
