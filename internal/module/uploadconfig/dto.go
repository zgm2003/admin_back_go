package uploadconfig

import "admin_back_go/internal/dict"

type DriverInitResponse struct {
	Dict DriverInitDict `json:"dict"`
}

type DriverInitDict struct {
	UploadDriverArr []dict.Option[string] `json:"upload_driver_arr"`
}

type RuleInitResponse struct {
	Dict RuleInitDict `json:"dict"`
}

type RuleInitDict struct {
	UploadImageExtArr []dict.Option[string] `json:"upload_image_ext_arr"`
	UploadFileExtArr  []dict.Option[string] `json:"upload_file_ext_arr"`
}

type SettingInitResponse struct {
	Dict SettingInitDict `json:"dict"`
}

type SettingInitDict struct {
	UploadDriverList []dict.Option[int] `json:"upload_driver_list"`
	UploadRuleList   []dict.Option[int] `json:"upload_rule_list"`
	CommonStatusArr  []dict.Option[int] `json:"common_status_arr"`
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type DriverListQuery struct {
	CurrentPage int
	PageSize    int
	Driver      string
}

type RuleListQuery struct {
	CurrentPage int
	PageSize    int
	Title       string
}

type SettingListQuery struct {
	CurrentPage int
	PageSize    int
	Remark      string
	Status      *int
	DriverID    *int64
	RuleID      *int64
}

type DriverListResponse struct {
	List []DriverItem `json:"list"`
	Page Page         `json:"page"`
}

type RuleListResponse struct {
	List []RuleItem `json:"list"`
	Page Page       `json:"page"`
}

type SettingListResponse struct {
	List []SettingItem `json:"list"`
	Page Page          `json:"page"`
}

type DriverItem struct {
	ID            int64   `json:"id"`
	Driver        string  `json:"driver"`
	DriverShow    string  `json:"driver_show"`
	SecretIDHint  string  `json:"secret_id_hint"`
	SecretKeyHint string  `json:"secret_key_hint"`
	Bucket        string  `json:"bucket"`
	Region        string  `json:"region"`
	RoleARN       *string `json:"role_arn"`
	AppID         *string `json:"appid"`
	Endpoint      *string `json:"endpoint"`
	BucketDomain  *string `json:"bucket_domain"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

type RuleItem struct {
	ID        int64    `json:"id"`
	Title     string   `json:"title"`
	MaxSizeMB int      `json:"max_size_mb"`
	ImageExts []string `json:"image_exts"`
	FileExts  []string `json:"file_exts"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

type SettingItem struct {
	ID         int64  `json:"id"`
	DriverID   int64  `json:"driver_id"`
	RuleID     int64  `json:"rule_id"`
	DriverName string `json:"driver_name"`
	RuleName   string `json:"rule_name"`
	Status     int    `json:"status"`
	StatusName string `json:"status_name"`
	Remark     string `json:"remark"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type DriverCreateInput struct {
	Driver       string
	SecretID     string
	SecretKey    string
	Bucket       string
	Region       string
	RoleARN      string
	AppID        string
	Endpoint     string
	BucketDomain string
}

type DriverUpdateInput struct {
	Driver       string
	SecretID     string
	SecretKey    string
	Bucket       string
	Region       string
	RoleARN      string
	AppID        string
	Endpoint     string
	BucketDomain string
}

type RuleMutationInput struct {
	Title     string
	MaxSizeMB int
	ImageExts []string
	FileExts  []string
}

type SettingMutationInput struct {
	DriverID int64
	RuleID   int64
	Status   int
	Remark   string
}
