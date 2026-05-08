package aiconversation

import (
	"context"
	"errors"
	"strings"
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

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).Table("ai_conversations c").
		Where("c.is_del = ?", enum.CommonNo).
		Where("c.user_id = ?", query.UserID)
	if query.Status != nil {
		db = db.Where("c.status = ?", *query.Status)
	}
	if query.AgentID != nil {
		db = db.Where("c.agent_id = ?", *query.AgentID)
	}
	if title := strings.TrimSpace(query.Title); title != "" {
		db = db.Where("c.title LIKE ?", "%"+title+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var flats []listRowFlat
	err := db.Select("c.id, c.user_id, c.agent_id, c.title, c.last_message_at, c.status, c.is_del, c.created_at, c.updated_at, a.name as agent_name").
		Joins("LEFT JOIN ai_agents a ON a.id = c.agent_id").
		Order("c.last_message_at DESC").
		Order("c.id DESC").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&flats).Error
	if err != nil {
		return nil, 0, err
	}
	rows := make([]ListRow, 0, len(flats))
	for _, row := range flats {
		rows = append(rows, row.toListRow())
	}
	return rows, total, nil
}

func (r *GormRepository) Get(ctx context.Context, id int64) (*Conversation, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}
	var row Conversation
	err := r.db.WithContext(ctx).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) AgentName(ctx context.Context, id int64) (string, error) {
	if r == nil || r.db == nil {
		return "", ErrRepositoryNotConfigured
	}
	var name string
	err := r.db.WithContext(ctx).Table("ai_agents").Where("id = ?", id).Pluck("name", &name).Error
	return name, err
}

func (r *GormRepository) ActiveAgentExists(ctx context.Context, id int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	var count int64
	err := r.db.WithContext(ctx).Table("ai_agents").
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Where("status = ?", enum.CommonYes).
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

func (r *GormRepository) Update(ctx context.Context, id int64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Conversation{}).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).Updates(fields).Error
}

func (r *GormRepository) ChangeStatus(ctx context.Context, id int64, status int) error {
	return r.Update(ctx, id, map[string]any{"status": status})
}

func (r *GormRepository) Delete(ctx context.Context, id int64) error {
	return r.Update(ctx, id, map[string]any{"is_del": enum.CommonYes})
}

type listRowFlat struct {
	ID            int64
	UserID        int64
	AgentID       int64
	Title         string
	LastMessageAt *time.Time
	Status        int
	IsDel         int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	AgentName     string
}

func (f listRowFlat) toListRow() ListRow {
	return ListRow{
		Conversation: Conversation{
			ID: f.ID, UserID: f.UserID, AgentID: f.AgentID, Title: f.Title,
			LastMessageAt: f.LastMessageAt, Status: f.Status, IsDel: f.IsDel,
			CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
		},
		AgentName: f.AgentName,
	}
}
