package aiconversation

import (
	"context"
	"errors"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrRepositoryNotConfigured = errors.New("aiconversation repository not configured")

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]ListRow, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, ErrRepositoryNotConfigured
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}
	db := r.db.WithContext(ctx).Table("ai_conversations c").
		Where("c.user_id = ?", query.UserID).
		Where("c.is_del = ?", enum.CommonNo)
	if query.AgentID != nil {
		db = db.Where("c.agent_id = ?", *query.AgentID)
	}
	if query.BeforeTime != nil && query.BeforeID > 0 {
		db = db.Where("(c.last_message_at < ? OR (c.last_message_at = ? AND c.id < ?))", *query.BeforeTime, *query.BeforeTime, query.BeforeID)
	}
	var flats []listRowFlat
	err := db.Select("c.id, c.user_id, c.agent_id, c.title, c.last_message_at, c.last_read_message_id, c.is_del, c.created_at, c.updated_at, a.name as agent_name").
		Joins("LEFT JOIN ai_agents a ON a.id = c.agent_id AND a.is_del = ?", enum.CommonNo).
		Order("c.last_message_at DESC").
		Order("c.id DESC").
		Limit(limit + 1).
		Scan(&flats).Error
	if err != nil {
		return nil, false, err
	}
	hasMore := len(flats) > limit
	if hasMore {
		flats = flats[:limit]
	}
	rows := make([]ListRow, 0, len(flats))
	for _, row := range flats {
		rows = append(rows, row.toListRow())
	}
	return rows, hasMore, nil
}

func (r *GormRepository) UnreadCounts(ctx context.Context, conversationIDs []int64) (map[int64]uint64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	counts := make(map[int64]uint64)
	if len(conversationIDs) == 0 {
		return counts, nil
	}
	var rows []struct {
		ConversationID int64
		UnreadCount    uint64
	}
	err := r.db.WithContext(ctx).Table("ai_messages m").
		Select("m.conversation_id, COUNT(*) AS unread_count").
		Joins("JOIN ai_conversations c ON c.id = m.conversation_id").
		Where("m.conversation_id IN ?", conversationIDs).
		Where("m.role = ?", enum.AIMessageRoleAssistant).
		Where("m.is_del = ?", enum.CommonNo).
		Where("m.id > c.last_read_message_id").
		Where("c.is_del = ?", enum.CommonNo).
		Group("m.conversation_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.ConversationID] = row.UnreadCount
	}
	return counts, nil
}

func (r *GormRepository) Get(ctx context.Context, id int64) (*Conversation, string, error) {
	if r == nil || r.db == nil {
		return nil, "", ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, "", nil
	}
	var flat listRowFlat
	err := r.db.WithContext(ctx).Table("ai_conversations c").
		Select("c.id, c.user_id, c.agent_id, c.title, c.last_message_at, c.last_read_message_id, c.is_del, c.created_at, c.updated_at, a.name as agent_name").
		Joins("LEFT JOIN ai_agents a ON a.id = c.agent_id AND a.is_del = ?", enum.CommonNo).
		Where("c.id = ?", id).
		Where("c.is_del = ?", enum.CommonNo).
		Limit(1).
		Scan(&flat).Error
	if err != nil {
		return nil, "", err
	}
	if flat.ID == 0 {
		return nil, "", nil
	}
	row := flat.toListRow()
	return &row.Conversation, row.AgentName, nil
}

func (r *GormRepository) ActiveChatAgentExists(ctx context.Context, id int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Table("ai_agents").
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Where("status = ?", enum.CommonYes).
		Where("JSON_CONTAINS(scenes_json, JSON_QUOTE(?))", "chat").
		Count(&count).Error
	return count > 0, err
}

func (r *GormRepository) Create(ctx context.Context, row Conversation) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) UpdateTitle(ctx context.Context, id int64, userID int64, title string) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Table("ai_conversations").
		Where("id = ? AND user_id = ? AND is_del = ?", id, userID, enum.CommonNo).
		Updates(map[string]any{"title": title, "updated_at": time.Now()}).Error
}

func (r *GormRepository) AdvanceReadCursor(ctx context.Context, conversationID int64, userID int64, messageID int64) (int64, bool, error) {
	if r == nil || r.db == nil {
		return 0, false, ErrRepositoryNotConfigured
	}
	var cursor int64
	valid := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversation struct {
			ID                int64
			LastReadMessageID int64
		}
		err := tx.Table("ai_conversations").
			Select("id, last_read_message_id").
			Where("id = ? AND user_id = ? AND is_del = ?", conversationID, userID, enum.CommonNo).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Take(&conversation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}

		var message struct{ ID int64 }
		err = tx.Table("ai_messages").
			Select("id").
			Where("id = ? AND conversation_id = ? AND role = ? AND is_del = ?", messageID, conversationID, enum.AIMessageRoleAssistant, enum.CommonNo).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Take(&message).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}

		if err := tx.Table("ai_conversations").
			Where("id = ? AND user_id = ? AND is_del = ?", conversationID, userID, enum.CommonNo).
			Update("last_read_message_id", gorm.Expr("GREATEST(last_read_message_id, ?)", messageID)).Error; err != nil {
			return err
		}
		cursor = conversation.LastReadMessageID
		if messageID > cursor {
			cursor = messageID
		}
		valid = true
		return nil
	})
	return cursor, valid, err
}

func (r *GormRepository) Delete(ctx context.Context, id int64, userID int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("ai_conversations").
			Where("id = ? AND user_id = ? AND is_del = ?", id, userID, enum.CommonNo).
			Update("is_del", enum.CommonYes).Error; err != nil {
			return err
		}
		return tx.Table("ai_messages").
			Where("conversation_id = ? AND is_del = ?", id, enum.CommonNo).
			Update("is_del", enum.CommonYes).Error
	})
}

type listRowFlat struct {
	ID                int64
	UserID            int64
	AgentID           int64
	Title             string
	LastMessageAt     *time.Time
	LastReadMessageID int64
	IsDel             int
	CreatedAt         time.Time
	UpdatedAt         time.Time
	AgentName         string
}

func (f listRowFlat) toListRow() ListRow {
	return ListRow{
		Conversation: Conversation{
			ID: f.ID, UserID: f.UserID, AgentID: f.AgentID, Title: f.Title,
			LastMessageAt: f.LastMessageAt, LastReadMessageID: f.LastReadMessageID, IsDel: f.IsDel,
			CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
		},
		AgentName: f.AgentName,
	}
}
