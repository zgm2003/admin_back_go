package payment

import "time"

// Order maps the payment_orders table exactly. Keep business rules in service.
type Order struct {
	ID            int64      `gorm:"column:id;primaryKey"`
	OrderNo       string     `gorm:"column:order_no"`
	ConfigID      int64      `gorm:"column:config_id"`
	ConfigCode    string     `gorm:"column:config_code"`
	Provider      string     `gorm:"column:provider"`
	PayMethod     string     `gorm:"column:pay_method"`
	Subject       string     `gorm:"column:subject"`
	AmountCents   int64      `gorm:"column:amount_cents"`
	Status        string     `gorm:"column:status"`
	PayURL        string     `gorm:"column:pay_url"`
	ReturnURL     string     `gorm:"column:return_url"`
	AlipayTradeNo string     `gorm:"column:alipay_trade_no"`
	ExpiredAt     time.Time  `gorm:"column:expired_at"`
	PaidAt        *time.Time `gorm:"column:paid_at"`
	ClosedAt      *time.Time `gorm:"column:closed_at"`
	FailureReason string     `gorm:"column:failure_reason"`
	IsDel         int        `gorm:"column:is_del"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
}

func (Order) TableName() string { return "payment_orders" }
