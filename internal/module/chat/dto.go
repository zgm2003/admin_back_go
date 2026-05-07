package chat

import "time"

type Identity struct {
	UserID   int64
	Platform string
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ConversationListQuery struct {
	UserID      int64
	CurrentPage int
	PageSize    int
}

type MessageListQuery struct {
	ConversationID int64
	CurrentPage    int
	PageSize       int
}

type CreatePrivateInput struct {
	UserID int64
}

type SendMessageInput struct {
	ConversationID int64
	Type           int
	Content        string
	MetaJSON       map[string]any
}

type MarkReadInput struct {
	ConversationID int64
}

type ContactInput struct {
	UserID int64
}

type ConversationListResponse struct {
	List []ConversationItem `json:"list"`
	Page Page               `json:"page"`
}

type CreatePrivateResponse struct {
	Conversation ConversationItem `json:"conversation"`
}

type SendMessageResponse struct {
	Message MessageItem `json:"message"`
}

type MessageListResponse struct {
	List []MessageItem `json:"list"`
	Page Page          `json:"page"`
}

type ContactListResponse struct {
	List []ContactItem `json:"list"`
}

type UserBrief struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
}

type ConversationRow struct {
	Conversation
	Role              int
	LastReadMessageID int64
	IsPinned          int
}

type ConversationItem struct {
	ID                 int64  `json:"id"`
	Type               int    `json:"type"`
	Name               string `json:"name"`
	Avatar             string `json:"avatar"`
	Announcement       string `json:"announcement"`
	OwnerID            int64  `json:"owner_id"`
	LastMessageID      int64  `json:"last_message_id"`
	LastMessageAt      string `json:"last_message_at"`
	LastMessagePreview string `json:"last_message_preview"`
	MemberCount        int    `json:"member_count"`
	UnreadCount        int64  `json:"unread_count"`
	IsPinned           int    `json:"is_pinned"`
	CreatedAt          string `json:"created_at"`
}

type MessageItem struct {
	ID             int64          `json:"id"`
	ConversationID int64          `json:"conversation_id"`
	SenderID       int64          `json:"sender_id"`
	Type           int            `json:"type"`
	Content        string         `json:"content"`
	MetaJSON       map[string]any `json:"meta_json,omitempty"`
	CreatedAt      string         `json:"created_at"`
	Sender         *UserBrief     `json:"sender,omitempty"`
}

type ContactRow struct {
	Contact
	Username string
	Avatar   string
}

type ContactItem struct {
	ID            int64  `json:"id"`
	ContactUserID int64  `json:"contact_user_id"`
	Username      string `json:"username"`
	Avatar        string `json:"avatar"`
	Status        int    `json:"status"`
	IsInitiator   int    `json:"is_initiator"`
	IsOnline      bool   `json:"is_online"`
	CreatedAt     string `json:"created_at"`
}

type CreateMessageInput struct {
	ConversationID int64
	SenderID       int64
	Type           int
	Content        string
	MetaJSON       *string
	CreatedAt      time.Time
}
