package payment

import "time"

type RechargePackage struct {
	ID          int64     `gorm:"column:id;primaryKey"`
	Code        string    `gorm:"column:code"`
	Name        string    `gorm:"column:name"`
	AmountCents int64     `gorm:"column:amount_cents"`
	Badge       string    `gorm:"column:badge"`
	Sort        int       `gorm:"column:sort"`
	Status      int       `gorm:"column:status"`
	IsDel       int       `gorm:"column:is_del"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (RechargePackage) TableName() string { return "payment_recharge_packages" }
