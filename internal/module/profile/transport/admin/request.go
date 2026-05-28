package admin

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

type saveQuickEntriesRequest struct {
	PermissionIDs []int64 `json:"permission_ids" binding:"required,max=6,dive,min=1"`
}
