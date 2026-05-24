package appauth

import (
	"admin_back_go/internal/module/captcha"
	"admin_back_go/internal/module/user"
)

type loginRequest struct {
	LoginType     string                `json:"login_type" binding:"required,auth_platform_login_type"`
	LoginAccount  string                `json:"login_account" binding:"required,max=100"`
	Password      string                `json:"password" binding:"omitempty,max=128"`
	Code          string                `json:"code" binding:"omitempty,max=20"`
	CaptchaID     string                `json:"captcha_id" binding:"omitempty,max=128"`
	CaptchaAnswer *captchaAnswerRequest `json:"captcha_answer"`
}

type captchaAnswerRequest struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type sendCodeRequest struct {
	Account string `json:"account" binding:"required,max=100"`
	Scene   string `json:"scene" binding:"required,verify_code_scene"`
}

type loginResponse struct {
	Token string  `json:"token"`
	User  appUser `json:"user"`
}

type appUser struct {
	ID       int64  `json:"id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

type profileResponse struct {
	Profile appProfile     `json:"profile"`
	Dict    appProfileDict `json:"dict"`
}

type profileUpdateResponse struct {
	User appUser `json:"user"`
}

type appProfileDict struct {
	AuthAddressTree []user.AddressTreeNode `json:"auth_address_tree"`
	SexArr          []user.SexOption       `json:"sexArr"`
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

type updateProfileRequest struct {
	Nickname      string  `json:"nickname" binding:"required,max=64"`
	Avatar        string  `json:"avatar" binding:"omitempty,max=255"`
	Sex           int     `json:"sex" binding:"user_sex"`
	Birthday      *string `json:"birthday" binding:"omitempty"`
	AddressID     *int64  `json:"address_id" binding:"required,min=0"`
	DetailAddress string  `json:"detail_address" binding:"omitempty,max=255"`
	Bio           string  `json:"bio" binding:"omitempty,max=1000"`
}

type uploadTokenRequest struct {
	Folder   string `json:"folder" binding:"required,upload_folder"`
	FileName string `json:"file_name" binding:"required,max=255"`
	FileSize int64  `json:"file_size" binding:"required,min=1"`
	FileKind string `json:"file_kind" binding:"required,oneof=image file"`
}

func captchaAnswerFromRequest(req *captchaAnswerRequest) *captcha.Answer {
	if req == nil {
		return nil
	}
	return &captcha.Answer{X: req.X, Y: req.Y}
}

func profileFromUserProfile(result *user.ProfileResponse) profileResponse {
	if result == nil {
		return profileResponse{}
	}
	detail := result.Profile
	return profileResponse{
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

func userFromProfile(result *user.ProfileResponse) appUser {
	if result == nil {
		return appUser{}
	}
	return appUser{
		ID:       result.Profile.UserID,
		Nickname: result.Profile.Username,
		Avatar:   result.Profile.Avatar,
	}
}
