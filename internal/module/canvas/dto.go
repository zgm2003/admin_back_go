package canvas

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type PromptListQuery struct {
	CurrentPage int
	PageSize    int
	Keyword     string
	Category    string
	Tags        []string
	Status      int
	IsDel       int
}

type AssetListQuery struct {
	CurrentPage int
	PageSize    int
	Keyword     string
	Type        string
	Status      int
	IsDel       int
}

type PromptInput struct {
	Slug      string
	Category  string
	Title     string
	CoverURL  string
	Prompt    string
	Preview   string
	TagsJSON  string
	SourceURL string
	Status    int
}

type AssetInput struct {
	Slug        string
	Type        string
	Category    string
	Title       string
	CoverURL    string
	Description string
	Content     string
	URL         string
	TagsJSON    string
	Status      int
}

type PromptListResponse struct {
	List []PromptItem `json:"list"`
	Page Page         `json:"page"`
}

type AssetListResponse struct {
	List []AssetItem `json:"list"`
	Page Page        `json:"page"`
}

type PromptItem struct {
	ID        int64  `json:"id"`
	Slug      string `json:"slug"`
	Category  string `json:"category"`
	Title     string `json:"title"`
	CoverURL  string `json:"cover_url"`
	Prompt    string `json:"prompt"`
	Preview   string `json:"preview"`
	TagsJSON  string `json:"tags_json"`
	SourceURL string `json:"source_url"`
	Status    int    `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type AssetItem struct {
	ID          int64  `json:"id"`
	Slug        string `json:"slug"`
	Type        string `json:"type"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	CoverURL    string `json:"cover_url"`
	Description string `json:"description"`
	Content     string `json:"content"`
	URL         string `json:"url"`
	TagsJSON    string `json:"tags_json"`
	Status      int    `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

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
}

type CanvasAgentOption struct {
	ID               uint64 `json:"id"`
	Name             string `json:"name"`
	Avatar           string `json:"avatar"`
	ModelID          string `json:"model_id"`
	ModelDisplayName string `json:"model_display_name"`
	Scene            string `json:"scene"`
}
