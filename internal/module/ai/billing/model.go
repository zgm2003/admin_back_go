package aibilling

import "time"

type Rule struct {
	ID             uint64    `gorm:"column:id;primaryKey"`
	Scene          string    `gorm:"column:scene"`
	Unit           string    `gorm:"column:unit"`
	UnitPriceCents int64     `gorm:"column:unit_price_cents"`
	Status         int       `gorm:"column:status"`
	IsDel          int       `gorm:"column:is_del"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (Rule) TableName() string { return "ai_billing_rules" }

type BillingRecord struct {
	ID                  int64      `gorm:"column:id;primaryKey"`
	RequestNo           string     `gorm:"column:request_no"`
	UserID              int64      `gorm:"column:user_id"`
	Platform            string     `gorm:"column:platform"`
	Scene               string     `gorm:"column:scene"`
	AgentID             int64      `gorm:"column:agent_id"`
	ProviderID          int64      `gorm:"column:provider_id"`
	ModelID             string     `gorm:"column:model_id"`
	Unit                string     `gorm:"column:unit"`
	UnitCount           int        `gorm:"column:unit_count"`
	UnitPriceCents      int64      `gorm:"column:unit_price_cents"`
	AmountCents         int64      `gorm:"column:amount_cents"`
	Status              string     `gorm:"column:status"`
	DebitTransactionID  *int64     `gorm:"column:debit_transaction_id"`
	RefundTransactionID *int64     `gorm:"column:refund_transaction_id"`
	ProviderTaskID      string     `gorm:"column:provider_task_id"`
	ErrorMessage        string     `gorm:"column:error_message"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at"`
	FinishedAt          *time.Time `gorm:"column:finished_at"`
}

func (BillingRecord) TableName() string { return "ai_billing_records" }
