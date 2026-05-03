package authplatform

import "admin_back_go/internal/dict"

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	CommonStatusArr            []dict.Option[int]    `json:"common_status_arr"`
	AuthPlatformLoginTypeArr   []dict.Option[string] `json:"auth_platform_login_type_arr"`
	AuthPlatformCaptchaTypeArr []dict.Option[string] `json:"auth_platform_captcha_type_arr"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Name        string
	Status      *int
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ListResponse struct {
	List []ListItem `json:"list"`
	Page Page       `json:"page"`
}

type ListItem struct {
	ID            int64    `json:"id"`
	Code          string   `json:"code"`
	Name          string   `json:"name"`
	LoginTypes    []string `json:"login_types"`
	CaptchaType   string   `json:"captcha_type"`
	AccessTTL     int      `json:"access_ttl"`
	RefreshTTL    int      `json:"refresh_ttl"`
	BindPlatform  int      `json:"bind_platform"`
	BindDevice    int      `json:"bind_device"`
	BindIP        int      `json:"bind_ip"`
	SingleSession int      `json:"single_session"`
	MaxSessions   int      `json:"max_sessions"`
	AllowRegister int      `json:"allow_register"`
	Status        int      `json:"status"`
	StatusName    string   `json:"status_name"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

type CreateInput struct {
	Code          string
	Name          string
	LoginTypes    []string
	CaptchaType   string
	AccessTTL     int
	RefreshTTL    int
	BindPlatform  int
	BindDevice    int
	BindIP        int
	SingleSession int
	MaxSessions   int
	AllowRegister int
}

type UpdateInput struct {
	Name          string
	LoginTypes    []string
	CaptchaType   string
	AccessTTL     int
	RefreshTTL    int
	BindPlatform  int
	BindDevice    int
	BindIP        int
	SingleSession int
	MaxSessions   int
	AllowRegister int
}
