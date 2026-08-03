package billing

import "time"

type WalletHold struct {
	ID            int64      `gorm:"column:id;primaryKey"`
	WalletID      int64      `gorm:"column:wallet_id"`
	UserID        int64      `gorm:"column:user_id"`
	RunID         int64      `gorm:"column:run_id"`
	HeldUnits     int64      `gorm:"column:held_units"`
	CapturedUnits int64      `gorm:"column:captured_units"`
	Status        HoldStatus `gorm:"column:status"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
}

func (WalletHold) TableName() string { return "wallet_holds" }

type UsageCharge struct {
	ID             int64        `gorm:"column:id;primaryKey"`
	RunID          int64        `gorm:"column:run_id"`
	UserID         int64        `gorm:"column:user_id"`
	Currency       string       `gorm:"column:currency"`
	PricingVersion string       `gorm:"column:pricing_version"`
	MultiplierPPM  int64        `gorm:"column:multiplier_ppm"`
	HeldUnits      int64        `gorm:"column:held_units"`
	ActualUnits    int64        `gorm:"column:actual_units"`
	Status         ChargeStatus `gorm:"column:status"`
	FinalizedAt    *time.Time   `gorm:"column:finalized_at"`
	CreatedAt      time.Time    `gorm:"column:created_at"`
	UpdatedAt      time.Time    `gorm:"column:updated_at"`
}

func (UsageCharge) TableName() string { return "ai_usage_charges" }

type UsageChargeItem struct {
	ID             int64         `gorm:"column:id;primaryKey"`
	ChargeID       int64         `gorm:"column:charge_id"`
	AttemptID      int64         `gorm:"column:attempt_id"`
	Category       UsageCategory `gorm:"column:category"`
	TierKey        string        `gorm:"column:tier_key"`
	Quantity       int64         `gorm:"column:quantity"`
	Unit           string        `gorm:"column:unit"`
	UnitPriceUnits int64         `gorm:"column:unit_price_units"`
	UnitScale      int64         `gorm:"column:unit_scale"`
	AmountUnits    int64         `gorm:"column:amount_units"`
	CreatedAt      time.Time     `gorm:"column:created_at"`
}

func (UsageChargeItem) TableName() string { return "ai_usage_charge_items" }

type ProviderAttempt struct {
	ID                    int64         `gorm:"column:id;primaryKey"`
	RunID                 int64         `gorm:"column:run_id"`
	CommandID             *int64        `gorm:"column:command_id"`
	AttemptNo             uint          `gorm:"column:attempt_no"`
	IdempotencyKey        string        `gorm:"column:idempotency_key"`
	State                 AttemptState  `gorm:"column:state"`
	PreparedRequestJSON   string        `gorm:"column:prepared_request_json"`
	PreparedRequestSHA256 []byte        `gorm:"column:prepared_request_sha256"`
	QuoteJSON             string        `gorm:"column:quote_json"`
	ContextPlanID         *uint64       `gorm:"column:context_plan_id"`
	ContextPlanSHA256     []byte        `gorm:"column:context_plan_sha256"`
	UsageJSON             string        `gorm:"column:usage_json"`
	UsageStatus           UsageStatus   `gorm:"column:usage_status"`
	DispatchState         DispatchState `gorm:"column:dispatch_state"`
	ResultCandidateJSON   *string       `gorm:"column:result_candidate_json"`
	ProviderRequestID     string        `gorm:"column:provider_request_id"`
	ResponseSHA256        string        `gorm:"column:response_sha256"`
	ErrorCode             string        `gorm:"column:error_code"`
	DispatchedAt          *time.Time    `gorm:"column:dispatched_at"`
	FinishedAt            *time.Time    `gorm:"column:finished_at"`
	CreatedAt             time.Time     `gorm:"column:created_at"`
	UpdatedAt             time.Time     `gorm:"column:updated_at"`
}

func (ProviderAttempt) TableName() string { return "ai_provider_attempts" }
