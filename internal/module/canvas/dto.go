package canvas

type SettingsInput struct {
	UserID int64
}

type SettingsResponse struct {
	AllowRegister bool              `json:"allow_register"`
	Scenes        []string          `json:"scenes"`
	Agents        CanvasAgentGroups `json:"agents"`
}

type CanvasAgentGroups struct {
	Text  []CanvasAgentOption `json:"text"`
	Image []CanvasAgentOption `json:"image"`
	Video []CanvasAgentOption `json:"video"`
	Audio []CanvasAgentOption `json:"audio"`
}

type CanvasAgentOption struct {
	ID               uint64 `json:"id"`
	Name             string `json:"name"`
	Avatar           string `json:"avatar"`
	ModelID          string `json:"model_id"`
	ModelDisplayName string `json:"model_display_name"`
	Scene            string `json:"scene"`
}
