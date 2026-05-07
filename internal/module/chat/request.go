package chat

import "encoding/json"

type conversationListRequest struct {
	CurrentPage int `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int `form:"page_size" binding:"omitempty,min=1,max=50"`
}

type createPrivateRequest struct {
	UserID int64 `json:"user_id" binding:"required,gt=0"`
}

type sendMessageRequest struct {
	Type     int             `json:"type" binding:"required,min=1,max=4"`
	Content  string          `json:"content" binding:"required,max=5000"`
	MetaJSON json.RawMessage `json:"meta_json" binding:"omitempty"`
}

type messageListRequest struct {
	CurrentPage int `form:"current_page" binding:"required,min=1"`
	PageSize    int `form:"page_size" binding:"omitempty,min=1,max=50"`
}

func decodeMetaJSON(raw json.RawMessage) (map[string]any, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, true
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, false
	}
	return meta, true
}
