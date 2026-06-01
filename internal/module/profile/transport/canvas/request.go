package canvas

type updateProfileRequest struct {
	Username      string  `json:"username" binding:"required,max=64"`
	Avatar        string  `json:"avatar" binding:"omitempty,max=512"`
	Sex           int     `json:"sex" binding:"omitempty,oneof=0 1 2"`
	Birthday      *string `json:"birthday" binding:"omitempty,max=10"`
	AddressID     *int64  `json:"address_id" binding:"required"`
	DetailAddress string  `json:"detail_address" binding:"omitempty,max=255"`
	Bio           string  `json:"bio" binding:"omitempty,max=500"`
}
