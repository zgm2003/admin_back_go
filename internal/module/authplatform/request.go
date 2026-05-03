package authplatform

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=50"`
	Name        string `form:"name" binding:"omitempty,max=100"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
}

type createRequest struct {
	Code          string   `json:"code" binding:"required,platform_code"`
	Name          string   `json:"name" binding:"required,max=100"`
	LoginTypes    []string `json:"login_types" binding:"required,min=1,dive,auth_platform_login_type"`
	CaptchaType   string   `json:"captcha_type" binding:"required,captcha_type"`
	AccessTTL     int      `json:"access_ttl" binding:"required,min=60,max=2592000"`
	RefreshTTL    int      `json:"refresh_ttl" binding:"required,min=60,max=31536000"`
	BindPlatform  int      `json:"bind_platform" binding:"required,common_yes_no"`
	BindDevice    int      `json:"bind_device" binding:"required,common_yes_no"`
	BindIP        int      `json:"bind_ip" binding:"required,common_yes_no"`
	SingleSession int      `json:"single_session" binding:"required,common_yes_no"`
	MaxSessions   int      `json:"max_sessions" binding:"min=0,max=100"`
	AllowRegister int      `json:"allow_register" binding:"required,common_yes_no"`
}

type updateRequest struct {
	Name          string   `json:"name" binding:"required,max=100"`
	LoginTypes    []string `json:"login_types" binding:"required,min=1,dive,auth_platform_login_type"`
	CaptchaType   string   `json:"captcha_type" binding:"required,captcha_type"`
	AccessTTL     int      `json:"access_ttl" binding:"required,min=60,max=2592000"`
	RefreshTTL    int      `json:"refresh_ttl" binding:"required,min=60,max=31536000"`
	BindPlatform  int      `json:"bind_platform" binding:"required,common_yes_no"`
	BindDevice    int      `json:"bind_device" binding:"required,common_yes_no"`
	BindIP        int      `json:"bind_ip" binding:"required,common_yes_no"`
	SingleSession int      `json:"single_session" binding:"required,common_yes_no"`
	MaxSessions   int      `json:"max_sessions" binding:"min=0,max=100"`
	AllowRegister int      `json:"allow_register" binding:"required,common_yes_no"`
}

type deleteBatchRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1,dive,gt=0"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}
