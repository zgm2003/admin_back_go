package app

type updateProfileRequest struct {
	Nickname      string  `json:"nickname" binding:"required,max=64"`
	Avatar        string  `json:"avatar" binding:"omitempty,max=255"`
	Sex           int     `json:"sex" binding:"user_sex"`
	Birthday      *string `json:"birthday" binding:"omitempty"`
	AddressID     *int64  `json:"address_id" binding:"required,min=0"`
	DetailAddress string  `json:"detail_address" binding:"omitempty,max=255"`
	Bio           string  `json:"bio" binding:"omitempty,max=1000"`
}
