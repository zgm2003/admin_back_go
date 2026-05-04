package uploadconfig

type driverListRequest struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=50"`
	Driver      string `form:"driver" binding:"omitempty,upload_driver"`
}

type driverCreateRequest struct {
	Driver       string `json:"driver" binding:"required,upload_driver"`
	SecretID     string `json:"secret_id" binding:"required,max=255"`
	SecretKey    string `json:"secret_key" binding:"required,max=255"`
	Bucket       string `json:"bucket" binding:"required,max=255"`
	Region       string `json:"region" binding:"required,max=100"`
	RoleARN      string `json:"role_arn" binding:"max=255"`
	AppID        string `json:"appid" binding:"max=100"`
	Endpoint     string `json:"endpoint" binding:"max=255"`
	BucketDomain string `json:"bucket_domain" binding:"max=255"`
}

type driverUpdateRequest struct {
	Driver       string `json:"driver" binding:"required,upload_driver"`
	SecretID     string `json:"secret_id" binding:"max=255"`
	SecretKey    string `json:"secret_key" binding:"max=255"`
	Bucket       string `json:"bucket" binding:"required,max=255"`
	Region       string `json:"region" binding:"required,max=100"`
	RoleARN      string `json:"role_arn" binding:"max=255"`
	AppID        string `json:"appid" binding:"max=100"`
	Endpoint     string `json:"endpoint" binding:"max=255"`
	BucketDomain string `json:"bucket_domain" binding:"max=255"`
}

type ruleListRequest struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=50"`
	Title       string `form:"title" binding:"max=50"`
}

type ruleMutationRequest struct {
	Title     string   `json:"title" binding:"required,max=50"`
	MaxSizeMB int      `json:"max_size_mb" binding:"required,min=1,max=10240"`
	ImageExts []string `json:"image_exts" binding:"omitempty,dive,upload_image_ext"`
	FileExts  []string `json:"file_exts" binding:"omitempty,dive,upload_file_ext"`
}

type settingListRequest struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=50"`
	Remark      string `form:"remark" binding:"max=255"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
	DriverID    *int64 `form:"driver_id" binding:"omitempty,gt=0"`
	RuleID      *int64 `form:"rule_id" binding:"omitempty,gt=0"`
}

type settingMutationRequest struct {
	DriverID int64  `json:"driver_id" binding:"required,gt=0"`
	RuleID   int64  `json:"rule_id" binding:"required,gt=0"`
	Status   int    `json:"status" binding:"required,common_status"`
	Remark   string `json:"remark" binding:"max=255"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}

type deleteBatchRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1,dive,gt=0"`
}
