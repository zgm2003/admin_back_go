package user

type appUser struct {
	ID       int64  `json:"id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

type appProfileResponse struct {
	Profile appProfile     `json:"profile"`
	Dict    appProfileDict `json:"dict"`
}

type appProfileUpdateResponse struct {
	User appUser `json:"user"`
}

type appProfileDict struct {
	AuthAddressTree []AddressTreeNode `json:"auth_address_tree"`
	SexArr          []SexOption       `json:"sexArr"`
}

type appProfile struct {
	UserID        int64  `json:"user_id"`
	Nickname      string `json:"nickname"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	Avatar        string `json:"avatar"`
	AddressID     int64  `json:"address_id"`
	DetailAddress string `json:"detail_address"`
	Sex           int    `json:"sex"`
	Birthday      string `json:"birthday"`
	Bio           string `json:"bio"`
	HasPassword   bool   `json:"has_password"`
}

type appUpdateProfileRequest struct {
	Nickname      string  `json:"nickname" binding:"required,max=64"`
	Avatar        string  `json:"avatar" binding:"omitempty,max=255"`
	Sex           int     `json:"sex" binding:"user_sex"`
	Birthday      *string `json:"birthday" binding:"omitempty"`
	AddressID     *int64  `json:"address_id" binding:"required,min=0"`
	DetailAddress string  `json:"detail_address" binding:"omitempty,max=255"`
	Bio           string  `json:"bio" binding:"omitempty,max=1000"`
}

func appUserFromInit(currentUser *InitResponse) appUser {
	if currentUser == nil {
		return appUser{}
	}
	return appUser{ID: currentUser.UserID, Nickname: currentUser.Username, Avatar: currentUser.Avatar}
}

func appUserFromProfile(result *ProfileResponse) appUser {
	if result == nil {
		return appUser{}
	}
	return appUser{ID: result.Profile.UserID, Nickname: result.Profile.Username, Avatar: result.Profile.Avatar}
}

func appProfileFromUserProfile(result *ProfileResponse) appProfileResponse {
	if result == nil {
		return appProfileResponse{}
	}
	detail := result.Profile
	return appProfileResponse{
		Profile: appProfile{
			UserID:        detail.UserID,
			Nickname:      detail.Username,
			Email:         detail.Email,
			Phone:         detail.Phone,
			Avatar:        detail.Avatar,
			AddressID:     detail.AddressID,
			DetailAddress: detail.DetailAddress,
			Sex:           detail.Sex,
			Birthday:      detail.Birthday,
			Bio:           detail.Bio,
			HasPassword:   detail.HasPassword,
		},
		Dict: appProfileDict{
			AuthAddressTree: result.Dict.AuthAddressTree,
			SexArr:          result.Dict.SexArr,
		},
	}
}
