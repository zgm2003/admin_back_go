package uploadconfig

import "time"

type Driver struct {
	ID            int64     `gorm:"column:id"`
	Driver        string    `gorm:"column:driver"`
	SecretIDEnc   string    `gorm:"column:secret_id_enc"`
	SecretIDHint  string    `gorm:"column:secret_id_hint"`
	SecretKeyEnc  string    `gorm:"column:secret_key_enc"`
	SecretKeyHint string    `gorm:"column:secret_key_hint"`
	Bucket        string    `gorm:"column:bucket"`
	Region        string    `gorm:"column:region"`
	AppID         string    `gorm:"column:appid"`
	Endpoint      string    `gorm:"column:endpoint"`
	BucketDomain  string    `gorm:"column:bucket_domain"`
	RoleARN       string    `gorm:"column:role_arn"`
	IsDel         int       `gorm:"column:is_del"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (Driver) TableName() string {
	return "upload_driver"
}

type Rule struct {
	ID        int64     `gorm:"column:id"`
	Title     string    `gorm:"column:title"`
	MaxSizeMB int       `gorm:"column:max_size_mb"`
	ImageExts string    `gorm:"column:image_exts"`
	FileExts  string    `gorm:"column:file_exts"`
	IsDel     int       `gorm:"column:is_del"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Rule) TableName() string {
	return "upload_rule"
}

type Setting struct {
	ID        int64     `gorm:"column:id"`
	DriverID  int64     `gorm:"column:driver_id"`
	RuleID    int64     `gorm:"column:rule_id"`
	Status    int       `gorm:"column:status"`
	IsDel     int       `gorm:"column:is_del"`
	Remark    string    `gorm:"column:remark"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Setting) TableName() string {
	return "upload_setting"
}

type SettingListRow struct {
	ID        int64     `gorm:"column:id"`
	DriverID  int64     `gorm:"column:driver_id"`
	RuleID    int64     `gorm:"column:rule_id"`
	Status    int       `gorm:"column:status"`
	Remark    string    `gorm:"column:remark"`
	Driver    string    `gorm:"column:driver"`
	Bucket    string    `gorm:"column:bucket"`
	RuleTitle string    `gorm:"column:rule_title"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}
