package paychannel

import "time"

type Channel struct {
	ID                int64     `gorm:"column:id;primaryKey"`
	Name              string    `gorm:"column:name"`
	Channel           int       `gorm:"column:channel"`
	MchID             string    `gorm:"column:mch_id"`
	AppID             string    `gorm:"column:app_id"`
	NotifyURL         string    `gorm:"column:notify_url"`
	AppPrivateKeyEnc  string    `gorm:"column:app_private_key_enc"`
	AppPrivateKeyHint string    `gorm:"column:app_private_key_hint"`
	PublicCertPath    string    `gorm:"column:public_cert_path"`
	PlatformCertPath  string    `gorm:"column:platform_cert_path"`
	RootCertPath      string    `gorm:"column:root_cert_path"`
	ExtraConfig       string    `gorm:"column:extra_config"`
	IsSandbox         int       `gorm:"column:is_sandbox"`
	Sort              int       `gorm:"column:sort"`
	Remark            string    `gorm:"column:remark"`
	Status            int       `gorm:"column:status"`
	IsDel             int       `gorm:"column:is_del"`
	CreatedAt         time.Time `gorm:"column:created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
}

func (Channel) TableName() string {
	return "pay_channel"
}
