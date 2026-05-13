package mail

import "time"

const defaultConfigKey = "default"

// Config maps mail_configs. Secret fields are encrypted and must never be returned by HTTP DTOs.
type Config struct {
	ID            uint64     `gorm:"column:id;primaryKey"`
	ConfigKey     string     `gorm:"column:config_key"`
	SecretIDEnc   string     `gorm:"column:secret_id_enc"`
	SecretIDHint  string     `gorm:"column:secret_id_hint"`
	SecretKeyEnc  string     `gorm:"column:secret_key_enc"`
	SecretKeyHint string     `gorm:"column:secret_key_hint"`
	Region        string     `gorm:"column:region"`
	Endpoint      string     `gorm:"column:endpoint"`
	FromEmail     string     `gorm:"column:from_email"`
	FromName      string     `gorm:"column:from_name"`
	ReplyTo       string     `gorm:"column:reply_to"`
	Status        int        `gorm:"column:status"`
	IsDel         int        `gorm:"column:is_del"`
	LastTestAt    *time.Time `gorm:"column:last_test_at"`
	LastTestError string     `gorm:"column:last_test_error"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
}

func (Config) TableName() string { return "mail_configs" }

// Template maps local verify-code scenes to Tencent SES templates.
type Template struct {
	ID                  uint64    `gorm:"column:id;primaryKey"`
	Scene               string    `gorm:"column:scene"`
	Name                string    `gorm:"column:name"`
	Subject             string    `gorm:"column:subject"`
	TencentTemplateID   uint64    `gorm:"column:tencent_template_id"`
	VariablesJSON       string    `gorm:"column:variables_json"`
	SampleVariablesJSON string    `gorm:"column:sample_variables_json"`
	Status              int       `gorm:"column:status"`
	IsDel               int       `gorm:"column:is_del"`
	CreatedAt           time.Time `gorm:"column:created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at"`
}

func (Template) TableName() string { return "mail_templates" }

// Log maps mail_logs. It records delivery facts only; no body, verify code, or TemplateData is stored.
type Log struct {
	ID               uint64     `gorm:"column:id;primaryKey"`
	Scene            string     `gorm:"column:scene"`
	TemplateID       *uint64    `gorm:"column:template_id"`
	ToEmail          string     `gorm:"column:to_email"`
	Subject          string     `gorm:"column:subject"`
	TencentRequestID string     `gorm:"column:tencent_request_id"`
	TencentMessageID string     `gorm:"column:tencent_message_id"`
	Status           int        `gorm:"column:status"`
	IsDel            int        `gorm:"column:is_del"`
	ErrorCode        string     `gorm:"column:error_code"`
	ErrorMessage     string     `gorm:"column:error_message"`
	DurationMS       uint64     `gorm:"column:duration_ms"`
	SentAt           *time.Time `gorm:"column:sent_at"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
}

func (Log) TableName() string { return "mail_logs" }
