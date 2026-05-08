package payment

import "time"

type Channel struct {
	ID               int64     `gorm:"column:id;primaryKey"`
	Code             string    `gorm:"column:code"`
	Name             string    `gorm:"column:name"`
	Provider         string    `gorm:"column:provider"`
	Status           int       `gorm:"column:status"`
	SupportedMethods string    `gorm:"column:supported_methods"`
	Remark           string    `gorm:"column:remark"`
	IsDel            int       `gorm:"column:is_del"`
	CreatedAt        time.Time `gorm:"column:created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (Channel) TableName() string { return "payment_channels" }

type ChannelConfig struct {
	ID                 int64     `gorm:"column:id;primaryKey"`
	ChannelID          int64     `gorm:"column:channel_id"`
	AppID              string    `gorm:"column:app_id"`
	MerchantID         string    `gorm:"column:merchant_id"`
	SignType           string    `gorm:"column:sign_type"`
	IsSandbox          int       `gorm:"column:is_sandbox"`
	NotifyURL          string    `gorm:"column:notify_url"`
	ReturnURL          string    `gorm:"column:return_url"`
	PrivateKeyEnc      string    `gorm:"column:private_key_enc"`
	PrivateKeyHint     string    `gorm:"column:private_key_hint"`
	AppCertPath        string    `gorm:"column:app_cert_path"`
	AlipayCertPath     string    `gorm:"column:alipay_cert_path"`
	AlipayRootCertPath string    `gorm:"column:alipay_root_cert_path"`
	ExtraConfig        string    `gorm:"column:extra_config"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (ChannelConfig) TableName() string { return "payment_channel_configs" }

type Order struct {
	ID           int64      `gorm:"column:id;primaryKey"`
	OrderNo      string     `gorm:"column:order_no"`
	UserID       int64      `gorm:"column:user_id"`
	ChannelID    int64      `gorm:"column:channel_id"`
	Provider     string     `gorm:"column:provider"`
	PayMethod    string     `gorm:"column:pay_method"`
	Subject      string     `gorm:"column:subject"`
	AmountCents  int64      `gorm:"column:amount_cents"`
	Currency     string     `gorm:"column:currency"`
	Status       int        `gorm:"column:status"`
	OutTradeNo   *string    `gorm:"column:out_trade_no"`
	TradeNo      string     `gorm:"column:trade_no"`
	PayURL       string     `gorm:"column:pay_url"`
	PaidAt       *time.Time `gorm:"column:paid_at"`
	ExpiredAt    time.Time  `gorm:"column:expired_at"`
	ClosedAt     *time.Time `gorm:"column:closed_at"`
	ClientIP     string     `gorm:"column:client_ip"`
	ReturnURL    string     `gorm:"column:return_url"`
	BusinessType string     `gorm:"column:business_type"`
	BusinessRef  string     `gorm:"column:business_ref"`
	IsDel        int        `gorm:"column:is_del"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at"`
}

func (Order) TableName() string { return "payment_orders" }

type Event struct {
	ID            int64     `gorm:"column:id;primaryKey"`
	OrderNo       string    `gorm:"column:order_no"`
	OutTradeNo    string    `gorm:"column:out_trade_no"`
	EventType     string    `gorm:"column:event_type"`
	Provider      string    `gorm:"column:provider"`
	RequestData   string    `gorm:"column:request_data"`
	ResponseData  string    `gorm:"column:response_data"`
	ProcessStatus int       `gorm:"column:process_status"`
	ErrorMessage  string    `gorm:"column:error_message"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

func (Event) TableName() string { return "payment_events" }
