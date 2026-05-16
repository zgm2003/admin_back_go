package sms

import "time"

const defaultConfigKey = "default"

// Config maps sms_configs. Secret fields are encrypted and must never be returned by HTTP DTOs.
type Config struct {
	ID            uint64     `gorm:"column:id;primaryKey"`
	ConfigKey     string     `gorm:"column:config_key"`
	SecretIDEnc   string     `gorm:"column:secret_id_enc"`
	SecretIDHint  string     `gorm:"column:secret_id_hint"`
	SecretKeyEnc  string     `gorm:"column:secret_key_enc"`
	SecretKeyHint string     `gorm:"column:secret_key_hint"`
	SmsSdkAppID   string     `gorm:"column:sms_sdk_app_id"`
	SignName      string     `gorm:"column:sign_name"`
	Region        string     `gorm:"column:region"`
	Endpoint      string     `gorm:"column:endpoint"`
	Status        int        `gorm:"column:status"`
	LastTestAt    *time.Time `gorm:"column:last_test_at"`
	LastTestError string     `gorm:"column:last_test_error"`
	IsDel         int        `gorm:"column:is_del"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
}

func (Config) TableName() string { return "sms_configs" }

// Template maps local verify-code scenes to Tencent SMS templates.
type Template struct {
	ID                  uint64    `gorm:"column:id;primaryKey"`
	Scene               string    `gorm:"column:scene"`
	Name                string    `gorm:"column:name"`
	TencentTemplateID   string    `gorm:"column:tencent_template_id"`
	VariablesJSON       string    `gorm:"column:variables_json"`
	SampleVariablesJSON string    `gorm:"column:sample_variables_json"`
	Status              int       `gorm:"column:status"`
	IsDel               int       `gorm:"column:is_del"`
	CreatedAt           time.Time `gorm:"column:created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at"`
}

func (Template) TableName() string { return "sms_templates" }

// Log maps sms_logs. It records delivery facts only; no body, verify code, or template params are stored.
type Log struct {
	ID               uint64     `gorm:"column:id;primaryKey"`
	Scene            string     `gorm:"column:scene"`
	TemplateID       *uint64    `gorm:"column:template_id"`
	ToPhone          string     `gorm:"column:to_phone"`
	Status           int        `gorm:"column:status"`
	TencentRequestID string     `gorm:"column:tencent_request_id"`
	TencentSerialNo  string     `gorm:"column:tencent_serial_no"`
	TencentFee       uint64     `gorm:"column:tencent_fee"`
	ErrorCode        string     `gorm:"column:error_code"`
	ErrorMessage     string     `gorm:"column:error_message"`
	DurationMS       uint64     `gorm:"column:duration_ms"`
	SentAt           *time.Time `gorm:"column:sent_at"`
	IsDel            int        `gorm:"column:is_del"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
}

func (Log) TableName() string { return "sms_logs" }
