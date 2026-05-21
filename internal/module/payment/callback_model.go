package payment

import "time"

type CallbackEvent struct {
	ID               int64      `gorm:"column:id;primaryKey"`
	Provider         string     `gorm:"column:provider"`
	NotifyID         string     `gorm:"column:notify_id"`
	OutTradeNo       string     `gorm:"column:out_trade_no"`
	TradeNo          string     `gorm:"column:trade_no"`
	TradeStatus      string     `gorm:"column:trade_status"`
	AppID            string     `gorm:"column:app_id"`
	TotalAmountCents int64      `gorm:"column:total_amount_cents"`
	SignatureValid   int        `gorm:"column:signature_valid"`
	ProcessStatus    string     `gorm:"column:process_status"`
	ProcessMessage   string     `gorm:"column:process_message"`
	RawPayloadJSON   string     `gorm:"column:raw_payload_json"`
	ReceivedAt       time.Time  `gorm:"column:received_at"`
	ProcessedAt      *time.Time `gorm:"column:processed_at"`
	IsDel            int        `gorm:"column:is_del"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
}

func (CallbackEvent) TableName() string { return "payment_callback_events" }
