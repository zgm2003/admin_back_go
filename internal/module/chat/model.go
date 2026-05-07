package chat

import "time"

const (
	ConversationTypePrivate = 1
	ConversationTypeGroup   = 2

	MessageTypeText   = 1
	MessageTypeImage  = 2
	MessageTypeFile   = 3
	MessageTypeSystem = 4

	ParticipantRoleOwner  = 1
	ParticipantRoleAdmin  = 2
	ParticipantRoleMember = 3

	ParticipantStatusActive = 1
	ParticipantStatusLeft   = 2
	ParticipantStatusKicked = 3

	ContactStatusPending   = 1
	ContactStatusConfirmed = 2

	EventChatMessageCreatedV1 = "chat.message.created.v1"
	EventChatReadV1           = "chat.read.v1"
)

type Conversation struct {
	ID                 int64     `gorm:"column:id"`
	Type               int       `gorm:"column:type"`
	Name               string    `gorm:"column:name"`
	Avatar             string    `gorm:"column:avatar"`
	Announcement       *string   `gorm:"column:announcement"`
	OwnerID            int64     `gorm:"column:owner_id"`
	LastMessageID      int64     `gorm:"column:last_message_id"`
	LastMessageAt      time.Time `gorm:"column:last_message_at"`
	LastMessagePreview string    `gorm:"column:last_message_preview"`
	MemberCount        int       `gorm:"column:member_count"`
	IsDel              int       `gorm:"column:is_del"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (Conversation) TableName() string { return "chat_conversations" }

type Participant struct {
	ID                int64     `gorm:"column:id"`
	ConversationID    int64     `gorm:"column:conversation_id"`
	UserID            int64     `gorm:"column:user_id"`
	Role              int       `gorm:"column:role"`
	Status            int       `gorm:"column:status"`
	LastReadMessageID int64     `gorm:"column:last_read_message_id"`
	IsPinned          int       `gorm:"column:is_pinned"`
	IsDel             int       `gorm:"column:is_del"`
	CreatedAt         time.Time `gorm:"column:created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
}

func (Participant) TableName() string { return "chat_participants" }

type Message struct {
	ID             int64     `gorm:"column:id"`
	ConversationID int64     `gorm:"column:conversation_id"`
	SenderID       int64     `gorm:"column:sender_id"`
	Type           int       `gorm:"column:type"`
	Content        string    `gorm:"column:content"`
	MetaJSON       *string   `gorm:"column:meta_json"`
	IsDel          int       `gorm:"column:is_del"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (Message) TableName() string { return "chat_messages" }

type Contact struct {
	ID            int64     `gorm:"column:id"`
	UserID        int64     `gorm:"column:user_id"`
	ContactUserID int64     `gorm:"column:contact_user_id"`
	IsInitiator   int       `gorm:"column:is_initiator"`
	Status        int       `gorm:"column:status"`
	IsDel         int       `gorm:"column:is_del"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (Contact) TableName() string { return "chat_contacts" }

type User struct {
	ID       int64  `gorm:"column:id"`
	Username string `gorm:"column:username"`
	Status   int    `gorm:"column:status"`
	IsDel    int    `gorm:"column:is_del"`
}

func (User) TableName() string { return "users" }

type UserProfile struct {
	UserID int64  `gorm:"column:user_id"`
	Avatar string `gorm:"column:avatar"`
	IsDel  int    `gorm:"column:is_del"`
}

func (UserProfile) TableName() string { return "user_profiles" }
