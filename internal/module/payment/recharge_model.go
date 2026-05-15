package payment

import "time"

type Recharge struct {
	ID             int64      `gorm:"column:id;primaryKey"`
	RechargeNo     string     `gorm:"column:recharge_no"`
	UserID         int64      `gorm:"column:user_id"`
	PackageCode    string     `gorm:"column:package_code"`
	PackageName    string     `gorm:"column:package_name"`
	AmountCents    int64      `gorm:"column:amount_cents"`
	PaymentOrderID int64      `gorm:"column:payment_order_id"`
	Status         string     `gorm:"column:status"`
	PaidAt         *time.Time `gorm:"column:paid_at"`
	CreditedAt     *time.Time `gorm:"column:credited_at"`
	FailureReason  string     `gorm:"column:failure_reason"`
	IsDel          int        `gorm:"column:is_del"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
}

func (Recharge) TableName() string { return "payment_recharges" }

type RechargeWithOrder struct {
	Recharge
	PaymentOrderNo string     `gorm:"column:payment_order_no"`
	PayURL         string     `gorm:"column:pay_url"`
	OrderStatus    string     `gorm:"column:order_status"`
	AlipayTradeNo  string     `gorm:"column:alipay_trade_no"`
	OrderPaidAt    *time.Time `gorm:"column:order_paid_at"`
}
