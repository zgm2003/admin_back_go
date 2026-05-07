package chat

import (
	"context"
	"errors"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrRepositoryNotConfigured = errors.New("chat repository is not configured")

type Repository interface {
	ListConversations(ctx context.Context, query ConversationListQuery) ([]ConversationRow, int64, error)
	ConversationUnreadCounts(ctx context.Context, userID int64, conversationIDs []int64) (map[int64]int64, error)
	PrivateConversationPeers(ctx context.Context, conversationIDs []int64, currentUserID int64) (map[int64]UserBrief, error)
	ListContacts(ctx context.Context, userID int64) ([]ContactRow, error)
	FindActiveUser(ctx context.Context, userID int64) (*UserBrief, error)
	IsConfirmedContact(ctx context.Context, userID int64, contactUserID int64) (bool, error)
	FindContact(ctx context.Context, userID int64, contactUserID int64) (*Contact, error)
	ContactExists(ctx context.Context, userIDA int64, userIDB int64) (bool, error)
	CreatePendingContactPair(ctx context.Context, initiatorID int64, targetID int64, now time.Time) error
	ConfirmContactPair(ctx context.Context, userIDA int64, userIDB int64, now time.Time) error
	SoftDeleteContactPair(ctx context.Context, userIDA int64, userIDB int64, now time.Time) error
	ClosePrivateConversationForContactPair(ctx context.Context, userIDA int64, userIDB int64, now time.Time) error
	WithTx(ctx context.Context, fn func(Repository) error) error
	LockConfirmedContactPair(ctx context.Context, userIDA int64, userIDB int64) error
	FindPrivateConversation(ctx context.Context, userIDA int64, userIDB int64) (*Conversation, error)
	CreatePrivateConversation(ctx context.Context, userIDA int64, userIDB int64, now time.Time) (*Conversation, error)
	RestoreParticipant(ctx context.Context, conversationID int64, userID int64) error
	FindConversationRow(ctx context.Context, conversationID int64, currentUserID int64) (*ConversationRow, error)
	IsActiveParticipant(ctx context.Context, conversationID int64, userID int64) (bool, error)
	ActiveParticipantUserIDs(ctx context.Context, conversationID int64) ([]int64, error)
	CreateMessage(ctx context.Context, input CreateMessageInput) (*Message, error)
	UpdateConversationLastMessage(ctx context.Context, conversationID int64, messageID int64, messageAt time.Time, preview string) error
	FindUserBrief(ctx context.Context, userID int64) (*UserBrief, error)
	ListMessages(ctx context.Context, query MessageListQuery) ([]Message, int64, error)
	UserBriefs(ctx context.Context, userIDs []int64) (map[int64]UserBrief, error)
	ConversationLastMessageID(ctx context.Context, conversationID int64) (int64, error)
	UpdateLastReadMessageID(ctx context.Context, conversationID int64, userID int64, messageID int64) error
	SoftDeleteConversationForUser(ctx context.Context, conversationID int64, userID int64) (int64, error)
	TogglePin(ctx context.Context, conversationID int64, userID int64) error
}

type GormRepository struct {
	db *gorm.DB
}

type unreadCountRow struct {
	ConversationID int64
	UnreadCount    int64
}

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) WithTx(ctx context.Context, fn func(Repository) error) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&GormRepository{db: tx})
	})
}

func (r *GormRepository) ListConversations(ctx context.Context, query ConversationListQuery) ([]ConversationRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).
		Table("chat_conversations AS c").
		Joins("JOIN chat_participants AS p ON p.conversation_id = c.id AND p.is_del = ? AND p.status = ?", enum.CommonNo, ParticipantStatusActive).
		Where("p.user_id = ?", query.UserID).
		Where("c.is_del = ?", enum.CommonNo)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []ConversationRow
	err := db.Select(`
			c.id, c.type, c.name, c.avatar, c.announcement, c.owner_id,
			c.last_message_id, c.last_message_at, c.last_message_preview,
			c.member_count, c.is_del, c.created_at, c.updated_at,
			p.role, p.last_read_message_id, p.is_pinned`).
		Order("p.is_pinned = 1 DESC").
		Order("c.last_message_at DESC").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	return rows, total, err
}

func (r *GormRepository) ConversationUnreadCounts(ctx context.Context, userID int64, conversationIDs []int64) (map[int64]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	result := make(map[int64]int64, len(conversationIDs))
	ids := normalizePositiveIDs(conversationIDs)
	if len(ids) == 0 {
		return result, nil
	}
	for _, id := range ids {
		result[id] = 0
	}
	var rows []unreadCountRow
	err := r.db.WithContext(ctx).
		Table("chat_participants AS p").
		Select("p.conversation_id, COUNT(m.id) AS unread_count").
		Joins("JOIN chat_messages AS m ON m.conversation_id = p.conversation_id AND m.is_del = ? AND m.id > p.last_read_message_id AND m.sender_id <> ?", enum.CommonNo, userID).
		Where("p.user_id = ?", userID).
		Where("p.conversation_id IN ?", ids).
		Where("p.is_del = ?", enum.CommonNo).
		Where("p.status = ?", ParticipantStatusActive).
		Group("p.conversation_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.ConversationID] = row.UnreadCount
	}
	return result, nil
}

func (r *GormRepository) PrivateConversationPeers(ctx context.Context, conversationIDs []int64, currentUserID int64) (map[int64]UserBrief, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if len(conversationIDs) == 0 {
		return map[int64]UserBrief{}, nil
	}
	type peerRow struct {
		ConversationID int64
		ID             int64
		Username       string
		Avatar         string
	}
	var rows []peerRow
	err := r.db.WithContext(ctx).
		Table("chat_participants AS p").
		Select("p.conversation_id, u.id, u.username, COALESCE(up.avatar, '') AS avatar").
		Joins("JOIN users AS u ON u.id = p.user_id AND u.is_del = ?", enum.CommonNo).
		Joins("LEFT JOIN user_profiles AS up ON up.user_id = u.id AND up.is_del = ?", enum.CommonNo).
		Where("p.conversation_id IN ?", conversationIDs).
		Where("p.user_id <> ?", currentUserID).
		Where("p.is_del = ?", enum.CommonNo).
		Where("p.status = ?", ParticipantStatusActive).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[int64]UserBrief, len(rows))
	for _, row := range rows {
		result[row.ConversationID] = UserBrief{ID: row.ID, Username: row.Username, Avatar: row.Avatar}
	}
	return result, nil
}

func (r *GormRepository) ListContacts(ctx context.Context, userID int64) ([]ContactRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []ContactRow
	err := r.db.WithContext(ctx).
		Table("chat_contacts AS c").
		Select("c.id, c.user_id, c.contact_user_id, c.is_initiator, c.status, c.is_del, c.created_at, c.updated_at, u.username, COALESCE(up.avatar, '') AS avatar").
		Joins("JOIN users AS u ON u.id = c.contact_user_id AND u.is_del = ?", enum.CommonNo).
		Joins("LEFT JOIN user_profiles AS up ON up.user_id = u.id AND up.is_del = ?", enum.CommonNo).
		Where("c.user_id = ?", userID).
		Where("c.is_del = ?", enum.CommonNo).
		Order("c.status ASC").
		Order("c.id DESC").
		Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) FindActiveUser(ctx context.Context, userID int64) (*UserBrief, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row struct {
		ID       int64
		Username string
		Avatar   string
	}
	err := r.db.WithContext(ctx).
		Table("users AS u").
		Select("u.id, u.username, COALESCE(up.avatar, '') AS avatar").
		Joins("LEFT JOIN user_profiles AS up ON up.user_id = u.id AND up.is_del = ?", enum.CommonNo).
		Where("u.id = ?", userID).
		Where("u.is_del = ?", enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &UserBrief{ID: row.ID, Username: row.Username, Avatar: row.Avatar}, nil
}

func (r *GormRepository) IsConfirmedContact(ctx context.Context, userID int64, contactUserID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&Contact{}).
		Where("user_id = ?", userID).
		Where("contact_user_id = ?", contactUserID).
		Where("status = ?", ContactStatusConfirmed).
		Where("is_del = ?", enum.CommonNo).
		Count(&count).Error
	return count > 0, err
}

func (r *GormRepository) FindContact(ctx context.Context, userID int64, contactUserID int64) (*Contact, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Contact
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("contact_user_id = ?", contactUserID).
		Where("is_del = ?", enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) ContactExists(ctx context.Context, userIDA int64, userIDB int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&Contact{}).
		Where("((user_id = ? AND contact_user_id = ?) OR (user_id = ? AND contact_user_id = ?))", userIDA, userIDB, userIDB, userIDA).
		Where("is_del = ?", enum.CommonNo).
		Count(&count).Error
	return count > 0, err
}

func (r *GormRepository) CreatePendingContactPair(ctx context.Context, initiatorID int64, targetID int64, now time.Time) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "contact_user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"is_initiator": gorm.Expr("VALUES(is_initiator)"),
			"status":       ContactStatusPending,
			"is_del":       enum.CommonNo,
			"updated_at":   now,
		}),
	}).Create(&[]Contact{
		{UserID: initiatorID, ContactUserID: targetID, IsInitiator: enum.CommonYes, Status: ContactStatusPending, IsDel: enum.CommonNo, CreatedAt: now, UpdatedAt: now},
		{UserID: targetID, ContactUserID: initiatorID, IsInitiator: enum.CommonNo, Status: ContactStatusPending, IsDel: enum.CommonNo, CreatedAt: now, UpdatedAt: now},
	}).Error
}

func (r *GormRepository) ConfirmContactPair(ctx context.Context, userIDA int64, userIDB int64, now time.Time) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Contact{}).
		Where("((user_id = ? AND contact_user_id = ?) OR (user_id = ? AND contact_user_id = ?))", userIDA, userIDB, userIDB, userIDA).
		Where("is_del = ?", enum.CommonNo).
		Updates(map[string]any{"status": ContactStatusConfirmed, "updated_at": now}).Error
}

func (r *GormRepository) SoftDeleteContactPair(ctx context.Context, userIDA int64, userIDB int64, now time.Time) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Contact{}).
		Where("((user_id = ? AND contact_user_id = ?) OR (user_id = ? AND contact_user_id = ?))", userIDA, userIDB, userIDB, userIDA).
		Where("is_del = ?", enum.CommonNo).
		Updates(map[string]any{"is_del": enum.CommonYes, "updated_at": now}).Error
}

func (r *GormRepository) ClosePrivateConversationForContactPair(ctx context.Context, userIDA int64, userIDB int64, now time.Time) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	var ids []int64
	err := r.db.WithContext(ctx).
		Table("chat_conversations AS c").
		Joins("JOIN chat_participants AS pa ON pa.conversation_id = c.id AND pa.user_id = ? AND pa.is_del = ? AND pa.status = ?", userIDA, enum.CommonNo, ParticipantStatusActive).
		Joins("JOIN chat_participants AS pb ON pb.conversation_id = c.id AND pb.user_id = ? AND pb.is_del = ? AND pb.status = ?", userIDB, enum.CommonNo, ParticipantStatusActive).
		Where("c.type = ?", ConversationTypePrivate).
		Where("c.is_del = ?", enum.CommonNo).
		Pluck("c.id", &ids).Error
	if err != nil {
		return err
	}
	ids = normalizePositiveIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&Participant{}).
		Where("conversation_id IN ?", ids).
		Where("user_id IN ?", []int64{userIDA, userIDB}).
		Where("is_del = ?", enum.CommonNo).
		Updates(map[string]any{"status": ParticipantStatusLeft, "updated_at": now}).Error
}

func (r *GormRepository) LockConfirmedContactPair(ctx context.Context, userIDA int64, userIDB int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	var rows []Contact
	return r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("((user_id = ? AND contact_user_id = ?) OR (user_id = ? AND contact_user_id = ?))", userIDA, userIDB, userIDB, userIDA).
		Where("status = ?", ContactStatusConfirmed).
		Where("is_del = ?", enum.CommonNo).
		Find(&rows).Error
}

func (r *GormRepository) FindPrivateConversation(ctx context.Context, userIDA int64, userIDB int64) (*Conversation, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Conversation
	err := r.db.WithContext(ctx).
		Table("chat_conversations AS c").
		Joins("JOIN chat_participants AS pa ON pa.conversation_id = c.id AND pa.user_id = ? AND pa.is_del = ? AND pa.status = ?", userIDA, enum.CommonNo, ParticipantStatusActive).
		Joins("JOIN chat_participants AS pb ON pb.conversation_id = c.id AND pb.user_id = ? AND pb.is_del = ? AND pb.status = ?", userIDB, enum.CommonNo, ParticipantStatusActive).
		Where("c.type = ?", ConversationTypePrivate).
		Where("c.is_del = ?", enum.CommonNo).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("c.*").
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) CreatePrivateConversation(ctx context.Context, userIDA int64, userIDB int64, now time.Time) (*Conversation, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	row := Conversation{Type: ConversationTypePrivate, Name: "", OwnerID: 0, MemberCount: 2, IsDel: enum.CommonNo, LastMessageAt: now, CreatedAt: now, UpdatedAt: now}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	participants := []Participant{
		{ConversationID: row.ID, UserID: userIDA, Role: ParticipantRoleMember, Status: ParticipantStatusActive, IsPinned: enum.CommonNo, IsDel: enum.CommonNo, CreatedAt: now, UpdatedAt: now},
		{ConversationID: row.ID, UserID: userIDB, Role: ParticipantRoleMember, Status: ParticipantStatusActive, IsPinned: enum.CommonNo, IsDel: enum.CommonNo, CreatedAt: now, UpdatedAt: now},
	}
	if err := r.db.WithContext(ctx).Create(&participants).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) RestoreParticipant(ctx context.Context, conversationID int64, userID int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Participant{}).
		Where("conversation_id = ?", conversationID).
		Where("user_id = ?", userID).
		Updates(map[string]any{"is_del": enum.CommonNo, "status": ParticipantStatusActive}).Error
}

func (r *GormRepository) FindConversationRow(ctx context.Context, conversationID int64, currentUserID int64) (*ConversationRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row ConversationRow
	err := r.db.WithContext(ctx).
		Table("chat_conversations AS c").
		Select(`
			c.id, c.type, c.name, c.avatar, c.announcement, c.owner_id,
			c.last_message_id, c.last_message_at, c.last_message_preview,
			c.member_count, c.is_del, c.created_at, c.updated_at,
			p.role, p.last_read_message_id, p.is_pinned`).
		Joins("JOIN chat_participants AS p ON p.conversation_id = c.id AND p.user_id = ? AND p.is_del = ? AND p.status = ?", currentUserID, enum.CommonNo, ParticipantStatusActive).
		Where("c.id = ?", conversationID).
		Where("c.is_del = ?", enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) IsActiveParticipant(ctx context.Context, conversationID int64, userID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&Participant{}).
		Where("conversation_id = ?", conversationID).
		Where("user_id = ?", userID).
		Where("is_del = ?", enum.CommonNo).
		Where("status = ?", ParticipantStatusActive).
		Count(&count).Error
	return count > 0, err
}

func (r *GormRepository) ActiveParticipantUserIDs(ctx context.Context, conversationID int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var ids []int64
	err := r.db.WithContext(ctx).Model(&Participant{}).
		Where("conversation_id = ?", conversationID).
		Where("is_del = ?", enum.CommonNo).
		Where("status = ?", ParticipantStatusActive).
		Pluck("user_id", &ids).Error
	return ids, err
}

func (r *GormRepository) CreateMessage(ctx context.Context, input CreateMessageInput) (*Message, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	row := Message{ConversationID: input.ConversationID, SenderID: input.SenderID, Type: input.Type, Content: input.Content, MetaJSON: input.MetaJSON, IsDel: enum.CommonNo, CreatedAt: input.CreatedAt, UpdatedAt: input.CreatedAt}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) UpdateConversationLastMessage(ctx context.Context, conversationID int64, messageID int64, messageAt time.Time, preview string) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Conversation{}).
		Where("id = ?", conversationID).
		Where("is_del = ?", enum.CommonNo).
		Updates(map[string]any{"last_message_id": messageID, "last_message_at": messageAt, "last_message_preview": preview}).Error
}

func (r *GormRepository) FindUserBrief(ctx context.Context, userID int64) (*UserBrief, error) {
	return r.FindActiveUser(ctx, userID)
}

func (r *GormRepository) ListMessages(ctx context.Context, query MessageListQuery) ([]Message, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).Model(&Message{}).
		Where("conversation_id = ?", query.ConversationID).
		Where("is_del = ?", enum.CommonNo)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Message
	err := db.Order("id DESC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) UserBriefs(ctx context.Context, userIDs []int64) (map[int64]UserBrief, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if len(userIDs) == 0 {
		return map[int64]UserBrief{}, nil
	}
	var rows []struct {
		ID       int64
		Username string
		Avatar   string
	}
	err := r.db.WithContext(ctx).
		Table("users AS u").
		Select("u.id, u.username, COALESCE(up.avatar, '') AS avatar").
		Joins("LEFT JOIN user_profiles AS up ON up.user_id = u.id AND up.is_del = ?", enum.CommonNo).
		Where("u.id IN ?", userIDs).
		Where("u.is_del = ?", enum.CommonNo).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[int64]UserBrief, len(rows))
	for _, row := range rows {
		result[row.ID] = UserBrief{ID: row.ID, Username: row.Username, Avatar: row.Avatar}
	}
	return result, nil
}

func (r *GormRepository) ConversationLastMessageID(ctx context.Context, conversationID int64) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	var row Conversation
	err := r.db.WithContext(ctx).Select("last_message_id").Where("id = ?", conversationID).Where("is_del = ?", enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return row.LastMessageID, err
}

func (r *GormRepository) UpdateLastReadMessageID(ctx context.Context, conversationID int64, userID int64, messageID int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Participant{}).
		Where("conversation_id = ?", conversationID).
		Where("user_id = ?", userID).
		Where("is_del = ?", enum.CommonNo).
		Where("status = ?", ParticipantStatusActive).
		Update("last_read_message_id", messageID).Error
}

func (r *GormRepository) SoftDeleteConversationForUser(ctx context.Context, conversationID int64, userID int64) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	result := r.db.WithContext(ctx).Model(&Participant{}).
		Where("conversation_id = ?", conversationID).
		Where("user_id = ?", userID).
		Where("is_del = ?", enum.CommonNo).
		Update("is_del", enum.CommonYes)
	return result.RowsAffected, result.Error
}

func (r *GormRepository) TogglePin(ctx context.Context, conversationID int64, userID int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	var row Participant
	err := r.db.WithContext(ctx).Where("conversation_id = ?", conversationID).Where("user_id = ?", userID).Where("is_del = ?", enum.CommonNo).First(&row).Error
	if err != nil {
		return err
	}
	next := enum.CommonYes
	if row.IsPinned == enum.CommonYes {
		next = enum.CommonNo
	}
	return r.db.WithContext(ctx).Model(&Participant{}).Where("id = ?", row.ID).Update("is_pinned", next).Error
}
