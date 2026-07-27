package admin

type listRequest struct {
	Family  string `form:"family" binding:"omitempty,oneof=gpt claude"`
	ModelID string `form:"model_id" binding:"omitempty,max=191"`
}

type rateRequest struct {
	Category  string  `json:"category" binding:"required,oneof=input output cache_read cache_write"`
	Unit      string  `json:"unit" binding:"required,eq=token"`
	TierKey   *string `json:"tier_key" binding:"required,max=64"`
	Price     string  `json:"price" binding:"required,max=64"`
	UnitScale int64   `json:"unit_scale" binding:"required,gt=0"`
}

type updateRequest struct {
	ExpectedVersion *int64        `json:"expected_version" binding:"required,gte=0"`
	Rates           []rateRequest `json:"rates" binding:"required,min=1,dive"`
	SourceURL       string        `json:"source_url" binding:"required,max=2048"`
	VerifiedAt      string        `json:"verified_at" binding:"required,len=10"`
}

type restoreRequest struct {
	ExpectedVersion int64 `form:"expected_version" binding:"required,gt=0"`
}
