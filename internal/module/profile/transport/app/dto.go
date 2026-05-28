package app

import "admin_back_go/internal/module/profile"

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
	AuthAddressTree []profile.AddressTreeNode `json:"auth_address_tree"`
	SexArr          []profile.SexOption       `json:"sexArr"`
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

func appUserFromInit(currentUser *profile.InitResponse) appUser {
	if currentUser == nil {
		return appUser{}
	}
	return appUser{ID: currentUser.UserID, Nickname: currentUser.Username, Avatar: currentUser.Avatar}
}

func appUserFromProfile(result *profile.ProfileResponse) appUser {
	if result == nil {
		return appUser{}
	}
	return appUser{ID: result.Profile.UserID, Nickname: result.Profile.Username, Avatar: result.Profile.Avatar}
}

func appProfileFromUserProfile(result *profile.ProfileResponse) appProfileResponse {
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
