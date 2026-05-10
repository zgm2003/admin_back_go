package aiconversation

import (
	"context"
	"errors"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
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
	if query.BeforeID > 0 {
		db = db.Where("c.id < ?", query.BeforeID)
	}
	var flats []listRowFlat
	err := db.Select("c.id, c.user_id, c.agent_id, c.title, c.last_message_at, c.is_del, c.created_at, c.updated_at, a.name as agent_name").
		Joins("LEFT JOIN ai_agents a ON a.id = c.agent_id AND a.is_del = ?", enum.CommonNo).
		Order("c.last_message_at IS NULL ASC").
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

func (r *GormRepository) Get(ctx context.Context, id int64) (*Conversation, string, error) {
	if r == nil || r.db == nil {
		return nil, "", ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, "", nil
	}
	var flat listRowFlat
	err := r.db.WithContext(ctx).Table("ai_conversations c").
		Select("c.id, c.user_id, c.agent_id, c.title, c.last_message_at, c.is_del, c.created_at, c.updated_at, a.name as agent_name").
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
	ID            int64
	UserID        int64
	AgentID       int64
	Title         string
	LastMessageAt *time.Time
	IsDel         int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	AgentName     string
}

func (f listRowFlat) toListRow() ListRow {
	return ListRow{
		Conversation: Conversation{
			ID: f.ID, UserID: f.UserID, AgentID: f.AgentID, Title: f.Title,
			LastMessageAt: f.LastMessageAt, IsDel: f.IsDel,
			CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
		},
		AgentName: f.AgentName,
	}
}
