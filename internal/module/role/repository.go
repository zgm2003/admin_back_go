package role

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("role repository not configured")

type Repository interface {
	WithTx(ctx context.Context, fn func(Repository) error) error
	List(ctx context.Context, query ListQuery) ([]Role, int64, error)
	RolesByIDs(ctx context.Context, ids []int64) (map[int64]Role, error)
	RoleByID(ctx context.Context, id int64) (*Role, error)
	ExistsByName(ctx context.Context, name string, excludeID int64) (bool, error)
	FindDeletedByName(ctx context.Context, name string) (*Role, error)
	Create(ctx context.Context, row Role) (int64, error)
	RestoreDeleted(ctx context.Context, id int64, row Role) error
	Update(ctx context.Context, id int64, fields map[string]any) error
	Delete(ctx context.Context, ids []int64) error
	HasDefaultIn(ctx context.Context, ids []int64) (bool, error)
	CountUsersByRoleIDs(ctx context.Context, ids []int64) (int64, error)
	ClearDefault(ctx context.Context) error
	SetDefault(ctx context.Context, id int64) error
	PermissionIDsByRoleIDs(ctx context.Context, roleIDs []int64) (map[int64][]int64, error)
	AllActivePermissions(ctx context.Context) ([]permission.Permission, error)
	SyncPermissions(ctx context.Context, roleID int64, permissionIDs []int64) error
	DeleteRolePermissionsByRoleIDs(ctx context.Context, roleIDs []int64) error
	UserIDsByRoleIDs(ctx context.Context, roleIDs []int64) ([]int64, error)
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

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]Role, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}

	db := r.db.WithContext(ctx).Model(&Role{}).Where("is_del = ?", permission.CommonNo)
	if query.Name != "" {
		db = db.Where("name LIKE ?", "%"+query.Name+"%")
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []Role
	err := db.Order("id asc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) RolesByIDs(ctx context.Context, ids []int64) (map[int64]Role, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return map[int64]Role{}, nil
	}

	var rows []Role
	err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Where("is_del = ?", permission.CommonNo).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make(map[int64]Role, len(rows))
	for _, row := range rows {
		result[row.ID] = row
	}
	return result, nil
}

func (r *GormRepository) RoleByID(ctx context.Context, id int64) (*Role, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}

	var row Role
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Where("is_del = ?", permission.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) ExistsByName(ctx context.Context, name string, excludeID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}

	db := r.db.WithContext(ctx).
		Model(&Role{}).
		Where("name = ?", name).
		Where("is_del = ?", permission.CommonNo)
	if excludeID > 0 {
		db = db.Where("id <> ?", excludeID)
	}

	var count int64
	if err := db.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) FindDeletedByName(ctx context.Context, name string) (*Role, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}

	var row Role
	err := r.db.WithContext(ctx).
		Where("name = ?", name).
		Where("is_del = ?", permission.CommonYes).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) Create(ctx context.Context, row Role) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) RestoreDeleted(ctx context.Context, id int64, row Role) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Role{}).
			Where("id = ?", id).
			Where("is_del = ?", permission.CommonYes).
			Updates(map[string]any{
				"name":       row.Name,
				"is_default": row.IsDefault,
				"is_del":     row.IsDel,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&RolePermission{}).
			Where("role_id = ?", id).
			Where("is_del = ?", permission.CommonYes).
			Update("is_del", permission.CommonNo).Error
	})
}

func (r *GormRepository) Update(ctx context.Context, id int64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Role{}).
		Where("id = ?", id).
		Where("is_del = ?", permission.CommonNo).
		Updates(fields).Error
}

func (r *GormRepository) Delete(ctx context.Context, ids []int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&Role{}).
		Where("id IN ?", ids).
		Where("is_del = ?", permission.CommonNo).
		Update("is_del", permission.CommonYes).Error
}

func (r *GormRepository) HasDefaultIn(ctx context.Context, ids []int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return false, nil
	}

	var count int64
	err := r.db.WithContext(ctx).
		Model(&Role{}).
		Where("id IN ?", ids).
		Where("is_del = ?", permission.CommonNo).
		Where("is_default = ?", permission.CommonYes).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) CountUsersByRoleIDs(ctx context.Context, ids []int64) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return 0, nil
	}

	var count int64
	err := r.db.WithContext(ctx).
		Model(&User{}).
		Where("role_id IN ?", ids).
		Where("is_del = ?", permission.CommonNo).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *GormRepository) ClearDefault(ctx context.Context) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Role{}).
		Where("is_default = ?", permission.CommonYes).
		Where("is_del = ?", permission.CommonNo).
		Update("is_default", permission.CommonNo).Error
}

func (r *GormRepository) SetDefault(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Role{}).
		Where("id = ?", id).
		Where("is_del = ?", permission.CommonNo).
		Update("is_default", permission.CommonYes).Error
}

func (r *GormRepository) PermissionIDsByRoleIDs(ctx context.Context, roleIDs []int64) (map[int64][]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	roleIDs = normalizeIDs(roleIDs)
	if len(roleIDs) == 0 {
		return map[int64][]int64{}, nil
	}

	var rows []RolePermission
	err := r.db.WithContext(ctx).
		Where("role_id IN ?", roleIDs).
		Where("is_del = ?", permission.CommonNo).
		Order("id asc").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make(map[int64][]int64, len(roleIDs))
	for _, row := range rows {
		result[row.RoleID] = append(result[row.RoleID], row.PermissionID)
	}
	return result, nil
}

func (r *GormRepository) AllActivePermissions(ctx context.Context) ([]permission.Permission, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []permission.Permission
	err := r.db.WithContext(ctx).
		Where("is_del = ?", permission.CommonNo).
		Where("status = ?", permission.StatusActive).
		Order("parent_id asc").
		Order("sort asc").
		Order("id asc").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *GormRepository) SyncPermissions(ctx context.Context, roleID int64, permissionIDs []int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if roleID <= 0 {
		return nil
	}

	currentMap, err := r.PermissionIDsByRoleIDs(ctx, []int64{roleID})
	if err != nil {
		return err
	}
	currentIDs := normalizeIDs(currentMap[roleID])
	nextIDs := normalizeIDs(permissionIDs)
	toAdd, toRemove := diffRolePermissionIDs(currentIDs, nextIDs)

	for _, permissionID := range toAdd {
		if err := r.bindOrRestore(ctx, roleID, permissionID); err != nil {
			return err
		}
	}

	if len(toRemove) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&RolePermission{}).
		Where("role_id = ?", roleID).
		Where("permission_id IN ?", toRemove).
		Where("is_del = ?", permission.CommonNo).
		Update("is_del", permission.CommonYes).Error
}

func diffRolePermissionIDs(currentIDs []int64, nextIDs []int64) ([]int64, []int64) {
	currentIDs = normalizeIDs(currentIDs)
	nextIDs = normalizeIDs(nextIDs)
	currentSet := idSet(currentIDs)
	nextSet := idSet(nextIDs)

	toAdd := make([]int64, 0)
	for _, permissionID := range nextIDs {
		if _, ok := currentSet[permissionID]; !ok {
			toAdd = append(toAdd, permissionID)
		}
	}

	toRemove := make([]int64, 0)
	for _, permissionID := range currentIDs {
		if _, ok := nextSet[permissionID]; !ok {
			toRemove = append(toRemove, permissionID)
		}
	}

	return toAdd, toRemove
}

func (r *GormRepository) DeleteRolePermissionsByRoleIDs(ctx context.Context, roleIDs []int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	roleIDs = normalizeIDs(roleIDs)
	if len(roleIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&RolePermission{}).
		Where("role_id IN ?", roleIDs).
		Where("is_del = ?", permission.CommonNo).
		Update("is_del", permission.CommonYes).Error
}

func (r *GormRepository) UserIDsByRoleIDs(ctx context.Context, roleIDs []int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	roleIDs = normalizeIDs(roleIDs)
	if len(roleIDs) == 0 {
		return []int64{}, nil
	}

	var userIDs []int64
	err := r.db.WithContext(ctx).
		Model(&User{}).
		Where("role_id IN ?", roleIDs).
		Where("is_del = ?", permission.CommonNo).
		Order("id asc").
		Pluck("id", &userIDs).Error
	if err != nil {
		return nil, err
	}
	return userIDs, nil
}

func (r *GormRepository) bindOrRestore(ctx context.Context, roleID int64, permissionID int64) error {
	var row RolePermission
	err := r.db.WithContext(ctx).
		Where("role_id = ?", roleID).
		Where("permission_id = ?", permissionID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.WithContext(ctx).Create(&RolePermission{
			RoleID:       roleID,
			PermissionID: permissionID,
			IsDel:        permission.CommonNo,
		}).Error
	}
	if err != nil {
		return err
	}
	if row.IsDel == permission.CommonNo {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&RolePermission{}).
		Where("id = ?", row.ID).
		Update("is_del", permission.CommonNo).Error
}

func idSet(ids []int64) map[int64]struct{} {
	result := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		result[id] = struct{}{}
	}
	return result
}
