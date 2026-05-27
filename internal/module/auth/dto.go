package auth

type LoginTypeOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type LoginConfigResponse struct {
	LoginTypeArr   []LoginTypeOption `json:"login_type_arr"`
	CaptchaEnabled bool              `json:"captcha_enabled"`
	CaptchaType    string            `json:"captcha_type"`
}

type LoginInput struct {
	LoginAccount  string
	LoginType     string
	Password      string
	Code          string
	CaptchaID     string
	CaptchaAnswer *Answer
	Platform      string
	DeviceID      string
	ClientIP      string
	UserAgent     string
}

type SendCodeInput struct {
	Account string
	Scene   string
}

type ForgetPasswordInput struct {
	Account         string
	Code            string
	NewPassword     string
	ConfirmPassword string
}

type LoginResponse struct {
	UserID           int64  `json:"-"`
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	IsNewUser        bool   `json:"is_new_user"`
}

type RefreshResponse = TokenResult

func loginResponseFromToken(result *TokenResult, userID int64, isNewUser bool) *LoginResponse {
	if result == nil {
		return nil
	}
	return &LoginResponse{
		UserID:           userID,
		AccessToken:      result.AccessToken,
		RefreshToken:     result.RefreshToken,
		ExpiresIn:        result.ExpiresIn,
		RefreshExpiresIn: result.RefreshExpiresIn,
		IsNewUser:        isNewUser,
	}
}
