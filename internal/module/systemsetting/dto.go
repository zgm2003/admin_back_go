package systemsetting

import "admin_back_go/internal/dict"

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	SystemSettingValueTypeArr []dict.Option[int] `json:"system_setting_value_type_arr"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Key         string
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

type CreateInput struct {
	Key    string
	Value  string
	Type   int
	Remark string
}

type UpdateInput struct {
	Value  string
	Type   int
	Remark string
}
