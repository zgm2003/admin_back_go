package payment

import "time"

// AlipayConfig is the active payment configuration table for V1.
type AlipayConfig struct {
	ID                 int64     `gorm:"column:id;primaryKey"`
	Code               string    `gorm:"column:code"`
	Name               string    `gorm:"column:name"`
	AppID              string    `gorm:"column:app_id"`
	AppPrivateKeyEnc   string    `gorm:"column:app_private_key_enc"`
	AppPrivateKeyHint  string    `gorm:"column:app_private_key_hint"`
	AppCertPath        string    `gorm:"column:app_cert_path"`
	AlipayCertPath     string    `gorm:"column:alipay_cert_path"`
	AlipayRootCertPath string    `gorm:"column:alipay_root_cert_path"`
	NotifyURL          string    `gorm:"column:notify_url"`
	ReturnURL          string    `gorm:"column:return_url"`
	Environment        string    `gorm:"column:environment"`
	EnabledMethodsJSON string    `gorm:"column:enabled_methods_json"`
	Status             int       `gorm:"column:status"`
	Remark             string    `gorm:"column:remark"`
	IsDel              int       `gorm:"column:is_del"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (AlipayConfig) TableName() string { return "payment_alipay_configs" }
