package paytransaction

import "time"

type Transaction struct {
	ID            int64      `gorm:"column:id;primaryKey"`
	TransactionNo string     `gorm:"column:transaction_no"`
	OrderID       int64      `gorm:"column:order_id"`
	OrderNo       string     `gorm:"column:order_no"`
	AttemptNo     int        `gorm:"column:attempt_no"`
	ChannelID     int64      `gorm:"column:channel_id"`
	Channel       int        `gorm:"column:channel"`
	PayMethod     string     `gorm:"column:pay_method"`
	Amount        int        `gorm:"column:amount"`
	TradeNo       string     `gorm:"column:trade_no"`
	TradeStatus   string     `gorm:"column:trade_status"`
	Status        int        `gorm:"column:status"`
	PaidAt        *time.Time `gorm:"column:paid_at"`
	ClosedAt      *time.Time `gorm:"column:closed_at"`
	ChannelResp   string     `gorm:"column:channel_resp"`
	RawNotify     string     `gorm:"column:raw_notify"`
	IsDel         int        `gorm:"column:is_del"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
}

func (Transaction) TableName() string {
	return "pay_transactions"
}
