package aibilling

import (
	"context"

	walletmodule "admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"
)

const (
	SceneAdminImageGenerate  = "admin_image_generate"
	SceneCanvasTextGenerate  = "canvas_text_generate"
	SceneCanvasImageGenerate = "canvas_image_generate"
	SceneCanvasVideoGenerate = "canvas_video_generate"

	UnitRequest = "request"
	UnitImage   = "image"
	UnitSecond  = "second"

	RuleStatusEnabled  = 1
	RuleStatusDisabled = 2

	BillingStatusCharged  = "charged"
	BillingStatusSuccess  = "success"
	BillingStatusFailed   = "failed"
	BillingStatusRefunded = "refunded"
)

type PageInitResponse struct {
	Dict PageInitDict `json:"dict"`
}

type PageInitDict struct {
	SceneArr        []dict.Option[string] `json:"scene_arr"`
	UnitArr         []dict.Option[string] `json:"unit_arr"`
	CommonStatusArr []dict.Option[int]    `json:"common_status_arr"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Scene       string
	Unit        string
	Status      *int
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ListResponse struct {
	List []RuleDTO `json:"list"`
	Page Page      `json:"page"`
}

type RuleDTO struct {
	ID             uint64 `json:"id"`
	Scene          string `json:"scene"`
	SceneName      string `json:"scene_name"`
	Unit           string `json:"unit"`
	UnitName       string `json:"unit_name"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	Status         int    `json:"status"`
	StatusName     string `json:"status_name"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type CreateRuleInput struct {
	Scene          string
	Unit           string
	UnitPriceCents int64
	Status         int
}

type UpdateRuleInput struct {
	Scene          string
	Unit           string
	UnitPriceCents int64
	Status         int
}

type HTTPService interface {
	PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	CreateRule(ctx context.Context, input CreateRuleInput) (uint64, *apperror.Error)
	UpdateRule(ctx context.Context, id uint64, input UpdateRuleInput) *apperror.Error
	ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error
	DeleteRule(ctx context.Context, id uint64) *apperror.Error
	EnabledRule(ctx context.Context, scene string) (*RuleDTO, *apperror.Error)
}

type ChargeInput struct {
	RequestNo  string
	UserID     int64
	Platform   string
	Scene      string
	AgentID    int64
	ProviderID int64
	ModelID    string
	UnitCount  int
	Remark     string
}

type ChargeResult struct {
	RecordID           int64
	AmountCents        int64
	DebitTransactionID int64
}

type RefundInput struct {
	BillingRecordID int64
	Reason          string
}

type WalletService interface {
	Debit(ctx context.Context, input walletmodule.MutationInput) (*walletmodule.MutationResponse, *apperror.Error)
	Credit(ctx context.Context, input walletmodule.MutationInput) (*walletmodule.MutationResponse, *apperror.Error)
}
