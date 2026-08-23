package uploadconfig

import (
	"admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/pagination"
)

type DriverPageInitResponse struct {
	Dict DriverPageInitDict `json:"dict"`
}

type DriverPageInitDict struct {
	UploadDriverArr []dict.Option[string] `json:"upload_driver_arr"`
}

type RulePageInitResponse struct {
	Dict RulePageInitDict `json:"dict"`
}

type UploadImageExtOption struct {
	Label string `json:"label"`
	Value string `json:"value" validate:"upload_image_ext"`
}

type UploadFileExtOption struct {
	Label string `json:"label"`
	Value string `json:"value" validate:"upload_file_ext"`
}

type RulePageInitDict struct {
	UploadImageExtArr []UploadImageExtOption `json:"upload_image_ext_arr"`
	UploadFileExtArr  []UploadFileExtOption  `json:"upload_file_ext_arr"`
}

type SettingPageInitResponse struct {
	Dict SettingPageInitDict `json:"dict"`
}

type SettingPageInitDict struct {
	UploadDriverList []dict.Option[int] `json:"upload_driver_list"`
	UploadRuleList   []dict.Option[int] `json:"upload_rule_list"`
	CommonStatusArr  []dict.Option[int] `json:"common_status_arr"`
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
	List []DriverItem    `json:"list"`
	Page pagination.Page `json:"page"`
}

type RuleListResponse struct {
	List []RuleItem      `json:"list"`
	Page pagination.Page `json:"page"`
}

type SettingListResponse struct {
	List []SettingItem   `json:"list"`
	Page pagination.Page `json:"page"`
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
	ImageExts []string `json:"image_exts" validate:"dive,upload_image_ext"`
	FileExts  []string `json:"file_exts" validate:"dive,upload_file_ext"`
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
