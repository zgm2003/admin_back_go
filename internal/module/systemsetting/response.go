package systemsetting

import "admin_back_go/internal/shared/dict"

type PageInitResponse struct {
	Dict PageInitDict `json:"dict"`
}

type PageInitDict struct {
	SystemSettingValueTypeArr []dict.Option[int] `json:"system_setting_value_type_arr"`
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
	ID            int64  `json:"id"`
	SettingKey    string `json:"setting_key"`
	SettingValue  string `json:"setting_value"`
	ValueType     int    `json:"value_type"`
	ValueTypeName string `json:"value_type_name"`
	Remark        string `json:"remark"`
	Status        int    `json:"status"`
	StatusName    string `json:"status_name"`
	IsDel         int    `json:"is_del"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type CreateResponse struct {
	ID int64 `json:"id"`
}

type EmptyResponse struct{}

// Kept only until the Admin HTTP surface moves into this package in Wave 02.
type InitResponse = PageInitResponse
type InitDict = PageInitDict
