package payment

import "time"

// Config is the active payment configuration table for V1.
// V1 only accepts provider=alipay, but the table name and common credential
// columns belong to the payment-config domain instead of the Alipay adapter.
type Config struct {
	ID                 int64     `gorm:"column:id;primaryKey"`
	Provider           string    `gorm:"column:provider"`
	Code               string    `gorm:"column:code"`
	Name               string    `gorm:"column:name"`
	AppID              string    `gorm:"column:app_id"`
	PrivateKeyEnc      string    `gorm:"column:private_key_enc"`
	PrivateKeyHint     string    `gorm:"column:private_key_hint"`
	AppCertPath        string    `gorm:"column:app_cert_path"`
	PlatformCertPath   string    `gorm:"column:platform_cert_path"`
	RootCertPath       string    `gorm:"column:root_cert_path"`
	NotifyURL          string    `gorm:"column:notify_url"`
	Environment        string    `gorm:"column:environment"`
	EnabledMethodsJSON string    `gorm:"column:enabled_methods_json"`
	Sort               int       `gorm:"column:sort"`
	Status             int       `gorm:"column:status"`
	Remark             string    `gorm:"column:remark"`
	IsDel              int       `gorm:"column:is_del"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (Config) TableName() string { return "payment_configs" }
