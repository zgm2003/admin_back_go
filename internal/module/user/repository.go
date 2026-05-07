package user

import (
	"context"
	"errors"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

const (
	commonNo = enum.CommonNo
)

var ErrRepositoryNotConfigured = errors.New("user repository not configured")

type Repository interface {
	WithTx(ctx context.Context, fn func(Repository) error) error
	FindUser(ctx context.Context, userID int64) (*User, error)
	FindProfile(ctx context.Context, userID int64) (*Profile, error)
	FindRole(ctx context.Context, roleID int64) (*Role, error)
	QuickEntries(ctx context.Context, userID int64) ([]QuickEntry, error)
	RoleOptions(ctx context.Context) ([]Role, error)
	ActiveAddresses(ctx context.Context) ([]Address, error)
	List(ctx context.Context, query ListQuery) ([]ListRow, int64, error)
	ExportUsersByIDs(ctx context.Context, ids []int64) ([]ExportUserRow, error)
	RoleByID(ctx context.Context, id int64) (*Role, error)
	UpdateUser(ctx context.Context, id int64, fields map[string]any) error
	ExistsEmailForOtherUser(ctx context.Context, userID int64, email string) (bool, error)
	ExistsPhoneForOtherUser(ctx context.Context, userID int64, phone string) (bool, error)
	UpdateProfile(ctx context.Context, userID int64, fields map[string]any) error
	EnsureProfile(ctx context.Context, profile Profile) error
	UpdateStatus(ctx context.Context, id int64, status int) error
	SoftDelete(ctx context.Context, ids []int64) error
	BatchUpdateProfile(ctx context.Context, input BatchProfileUpdate) error
}

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(client *database.Client) Repository {
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

func (r *GormRepository) FindUser(ctx context.Context, userID int64) (*User, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var user User
	err := r.db.WithContext(ctx).
		Where("id = ?", userID).
		Where("is_del = ?", commonNo).
		First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *GormRepository) RoleOptions(ctx context.Context) ([]Role, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var rows []Role
	err := r.db.WithContext(ctx).
		Where("is_del = ?", commonNo).
		Order("id asc").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *GormRepository) ActiveAddresses(ctx context.Context) ([]Address, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var rows []Address
	err := r.db.WithContext(ctx).
		Where("is_del = ?", commonNo).
		Order("parent_id asc, id asc").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}

	db := r.db.WithContext(ctx).
		Table("users AS u").
		Joins("LEFT JOIN user_profiles AS up ON up.user_id = u.id AND up.is_del = ?", commonNo).
		Joins("LEFT JOIN roles AS r ON r.id = u.role_id AND r.is_del = ?", commonNo).
		Where("u.is_del = ?", commonNo)

	if query.Keyword != "" {
		keyword := query.Keyword + "%"
		db = db.Where("(u.username LIKE ? OR u.email LIKE ? OR u.phone LIKE ?)", keyword, keyword, keyword)
	}
	if query.Username != "" {
		db = db.Where("u.username LIKE ?", query.Username+"%")
	}
	if query.Email != "" {
		db = db.Where("u.email LIKE ?", query.Email+"%")
	}
	if query.DetailAddress != "" {
		db = db.Where("up.detail_address LIKE ?", query.DetailAddress+"%")
	}
	if query.RoleID > 0 {
		db = db.Where("u.role_id = ?", query.RoleID)
	}
	if query.Sex != nil {
		db = db.Where("up.sex = ?", *query.Sex)
	}
	if len(query.AddressIDs) == 1 {
		db = db.Where("up.address_id = ?", query.AddressIDs[0])
	} else if len(query.AddressIDs) > 1 {
		db = db.Where("up.address_id IN ?", query.AddressIDs)
	}
	if len(query.DateRange) == 2 {
		start, end := query.DateRange[0], query.DateRange[1]
		if start != "" && end != "" {
			db = db.Where("u.created_at BETWEEN ? AND ?", start+" 00:00:00", end+" 23:59:59")
		}
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []ListRow
	err := db.Select(`
			u.id,
			u.username,
			u.email,
			u.phone,
			u.status,
			u.role_id,
			r.name AS role_name,
			up.avatar,
			up.sex,
			up.address_id,
			up.detail_address,
			up.bio,
			u.created_at
		`).
		Order("u.id desc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) ExportUsersByIDs(ctx context.Context, ids []int64) ([]ExportUserRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return []ExportUserRow{}, nil
	}
	var rows []ExportUserRow
	err := r.db.WithContext(ctx).
		Table("users AS u").
		Select(`u.id, u.username, u.email, u.phone,
			COALESCE(up.avatar, '') AS avatar,
			COALESCE(up.sex, 0) AS sex,
			COALESCE(r.name, '') AS role_name`).
		Joins("LEFT JOIN user_profiles AS up ON up.user_id = u.id AND up.is_del = ?", commonNo).
		Joins("LEFT JOIN roles AS r ON r.id = u.role_id AND r.is_del = ?", commonNo).
		Where("u.id IN ?", ids).
		Where("u.is_del = ?", commonNo).
		Order("u.id asc").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *GormRepository) RoleByID(ctx context.Context, id int64) (*Role, error) {
	return r.FindRole(ctx, id)
}

func (r *GormRepository) UpdateUser(ctx context.Context, id int64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&User{}).
		Where("id = ?", id).
		Where("is_del = ?", commonNo).
		Updates(fields).Error
}

func (r *GormRepository) ExistsEmailForOtherUser(ctx context.Context, userID int64, email string) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	var count int64
	err := r.db.WithContext(ctx).
		Model(&User{}).
		Where("email = ?", email).
		Where("id <> ?", userID).
		Where("is_del = ?", commonNo).
		Count(&count).Error
	return count > 0, err
}

func (r *GormRepository) ExistsPhoneForOtherUser(ctx context.Context, userID int64, phone string) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	var count int64
	err := r.db.WithContext(ctx).
		Model(&User{}).
		Where("phone = ?", phone).
		Where("id <> ?", userID).
		Where("is_del = ?", commonNo).
		Count(&count).Error
	return count > 0, err
}

func (r *GormRepository) UpdateProfile(ctx context.Context, userID int64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if userID <= 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&Profile{}).
		Where("user_id = ?", userID).
		Where("is_del = ?", commonNo).
		Updates(fields).Error
}

func (r *GormRepository) EnsureProfile(ctx context.Context, profile Profile) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if profile.UserID <= 0 {
		return nil
	}
	now := time.Now()
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = now
	}
	if profile.UpdatedAt.IsZero() {
		profile.UpdatedAt = now
	}
	if profile.IsDel == 0 {
		profile.IsDel = commonNo
	}
	return r.db.WithContext(ctx).Create(&profile).Error
}

func (r *GormRepository) UpdateStatus(ctx context.Context, id int64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&User{}).
		Where("id = ?", id).
		Where("is_del = ?", commonNo).
		Updates(map[string]any{"status": status, "updated_at": time.Now()}).Error
}

func (r *GormRepository) SoftDelete(ctx context.Context, ids []int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return nil
	}

	now := time.Now()
	if err := r.db.WithContext(ctx).
		Model(&User{}).
		Where("id IN ?", ids).
		Where("is_del = ?", commonNo).
		Updates(map[string]any{"is_del": enum.CommonYes, "updated_at": now}).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).
		Model(&Profile{}).
		Where("user_id IN ?", ids).
		Where("is_del = ?", commonNo).
		Updates(map[string]any{"is_del": enum.CommonYes, "updated_at": now}).Error
}

func (r *GormRepository) BatchUpdateProfile(ctx context.Context, input BatchProfileUpdate) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	ids := normalizeIDs(input.IDs)
	if len(ids) == 0 {
		return nil
	}

	fields := map[string]any{"updated_at": time.Now()}
	switch input.Field {
	case BatchProfileFieldSex:
		fields["sex"] = input.Sex
	case BatchProfileFieldAddressID:
		fields["address_id"] = input.AddressID
	case BatchProfileFieldDetailAddress:
		fields["detail_address"] = input.DetailAddress
	default:
		return nil
	}

	return r.db.WithContext(ctx).
		Model(&Profile{}).
		Where("user_id IN ?", ids).
		Where("is_del = ?", commonNo).
		Updates(fields).Error
}

func (r *GormRepository) FindProfile(ctx context.Context, userID int64) (*Profile, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var profile Profile
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("is_del = ?", commonNo).
		First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (r *GormRepository) FindRole(ctx context.Context, roleID int64) (*Role, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if roleID <= 0 {
		return nil, nil
	}

	var role Role
	err := r.db.WithContext(ctx).
		Where("id = ?", roleID).
		Where("is_del = ?", commonNo).
		First(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *GormRepository) QuickEntries(ctx context.Context, userID int64) ([]QuickEntry, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var entries []QuickEntry
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("permission_id > ?", 0).
		Where("is_del = ?", commonNo).
		Order("sort asc").
		Find(&entries).Error
	if err != nil {
		return nil, err
	}
	return entries, nil
}
