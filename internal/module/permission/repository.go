package permission

import (
	"context"
	"errors"

	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("permission repository not configured")

type Repository interface {
	PermissionIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error)
	AllActivePermissions(ctx context.Context) ([]Permission, error)
	ListPermissions(ctx context.Context, query PermissionListQuery) ([]Permission, error)
	GetPermission(ctx context.Context, id int64) (*Permission, error)
	ExistsByPlatformCode(ctx context.Context, platform string, code string, excludeID int64) (bool, error)
	ExistsByPlatformPath(ctx context.Context, platform string, path string, excludeID int64) (bool, error)
	ExistsByPlatformI18nKey(ctx context.Context, platform string, i18nKey string, excludeID int64) (bool, error)
	FindDeletedByPlatformCode(ctx context.Context, platform string, code string) (*Permission, error)
	CreatePermission(ctx context.Context, row Permission) (int64, error)
	RestoreDeletedPermission(ctx context.Context, id int64, row Permission) error
	UpdatePermission(ctx context.Context, id int64, fields map[string]any) error
	HasChildrenOutsideIDs(ctx context.Context, ids []int64) (bool, error)
	CascadeIDs(ctx context.Context, ids []int64) ([]int64, error)
	ActiveChildren(ctx context.Context, parentID int64) ([]Permission, error)
	DeletePermissions(ctx context.Context, ids []int64) error
	RoleIDsByPermissionIDs(ctx context.Context, permissionIDs []int64) ([]int64, error)
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

func (r *GormRepository) PermissionIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var ids []int64
	err := r.db.WithContext(ctx).
		Model(&RolePermission{}).
		Where("role_id = ?", roleID).
		Where("is_del = ?", CommonNo).
		Order("id asc").
		Pluck("permission_id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *GormRepository) AllActivePermissions(ctx context.Context) ([]Permission, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var permissions []Permission
	err := r.db.WithContext(ctx).
		Where("is_del = ?", CommonNo).
		Where("status = ?", StatusActive).
		Order("parent_id asc").
		Order("sort asc").
		Order("id asc").
		Find(&permissions).Error
	if err != nil {
		return nil, err
	}
	return permissions, nil
}

func (r *GormRepository) ListPermissions(ctx context.Context, query PermissionListQuery) ([]Permission, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	db := r.db.WithContext(ctx).
		Where("platform = ?", query.Platform).
		Where("is_del = ?", CommonNo)
	if query.Name != "" {
		db = db.Where("name LIKE ?", "%"+query.Name+"%")
	}
	if query.Path != "" {
		db = db.Where("path LIKE ?", "%"+query.Path+"%")
	}
	if query.Type > 0 {
		db = db.Where("type = ?", query.Type)
	}

	var permissions []Permission
	err := db.Order("sort asc").Order("id asc").Find(&permissions).Error
	if err != nil {
		return nil, err
	}
	return permissions, nil
}

func (r *GormRepository) GetPermission(ctx context.Context, id int64) (*Permission, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}

	var permission Permission
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Where("is_del = ?", CommonNo).
		First(&permission).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &permission, nil
}

func (r *GormRepository) ExistsByPlatformCode(ctx context.Context, platform string, code string, excludeID int64) (bool, error) {
	return r.existsByPlatformField(ctx, platform, "code", code, excludeID)
}

func (r *GormRepository) ExistsByPlatformPath(ctx context.Context, platform string, path string, excludeID int64) (bool, error) {
	return r.existsByPlatformField(ctx, platform, "path", path, excludeID)
}

func (r *GormRepository) ExistsByPlatformI18nKey(ctx context.Context, platform string, i18nKey string, excludeID int64) (bool, error) {
	return r.existsByPlatformField(ctx, platform, "i18n_key", i18nKey, excludeID)
}

func (r *GormRepository) FindDeletedByPlatformCode(ctx context.Context, platform string, code string) (*Permission, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if code == "" {
		return nil, nil
	}

	var permission Permission
	err := r.db.WithContext(ctx).
		Where("platform = ?", platform).
		Where("code = ?", code).
		Where("is_del = ?", CommonYes).
		First(&permission).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &permission, nil
}

func (r *GormRepository) existsByPlatformField(ctx context.Context, platform string, field string, value string, excludeID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).
		Model(&Permission{}).
		Where("platform = ?", platform).
		Where(field+" = ?", value).
		Where("is_del = ?", CommonNo)
	if excludeID > 0 {
		db = db.Where("id <> ?", excludeID)
	}

	var count int64
	if err := db.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) CreatePermission(ctx context.Context, row Permission) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	dbRow := permissionWriteRowFrom(row)
	if err := r.db.WithContext(ctx).Create(&dbRow).Error; err != nil {
		return 0, err
	}
	return dbRow.ID, nil
}

func (r *GormRepository) RestoreDeletedPermission(ctx context.Context, id int64, row Permission) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil
	}

	fields := permissionCreateFields(row)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Permission{}).
			Where("id = ?", id).
			Where("is_del = ?", CommonYes).
			Updates(fields).Error; err != nil {
			return err
		}
		return tx.Model(&RolePermission{}).
			Where("permission_id = ?", id).
			Where("is_del = ?", CommonNo).
			Update("is_del", CommonYes).Error
	})
}

func (r *GormRepository) UpdatePermission(ctx context.Context, id int64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Permission{}).
		Where("id = ?", id).
		Where("is_del = ?", CommonNo).
		Updates(permissionUpdateFields(fields)).Error
}

func (r *GormRepository) HasChildrenOutsideIDs(ctx context.Context, ids []int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	ids = normalizeIDsForMutation(ids)
	if len(ids) == 0 {
		return false, nil
	}

	var count int64
	err := r.db.WithContext(ctx).
		Model(&Permission{}).
		Where("parent_id IN ?", ids).
		Where("id NOT IN ?", ids).
		Where("is_del = ?", CommonNo).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) CascadeIDs(ctx context.Context, ids []int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	ids = normalizeIDsForMutation(ids)
	if len(ids) == 0 {
		return []int64{}, nil
	}

	var rows []Permission
	err := r.db.WithContext(ctx).
		Where("is_del = ?", CommonNo).
		Find(&rows, "id > ?", 0).Error
	if err != nil {
		return nil, err
	}

	childrenByParent := make(map[int64][]int64, len(rows))
	for _, row := range rows {
		childrenByParent[row.ParentID] = append(childrenByParent[row.ParentID], row.ID)
	}

	included := make(map[int64]struct{}, len(ids))
	stack := append([]int64{}, ids...)
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current <= 0 {
			continue
		}
		if _, ok := included[current]; ok {
			continue
		}
		included[current] = struct{}{}
		stack = append(stack, childrenByParent[current]...)
	}

	result := make([]int64, 0, len(included))
	for _, id := range ids {
		if _, ok := included[id]; ok {
			result = append(result, id)
			delete(included, id)
		}
	}
	for id := range included {
		result = append(result, id)
	}
	return result, nil
}

func (r *GormRepository) ActiveChildren(ctx context.Context, parentID int64) ([]Permission, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var permissions []Permission
	err := r.db.WithContext(ctx).
		Where("parent_id = ?", parentID).
		Where("is_del = ?", CommonNo).
		Find(&permissions).Error
	if err != nil {
		return nil, err
	}
	return permissions, nil
}

func (r *GormRepository) DeletePermissions(ctx context.Context, ids []int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	ids = normalizeIDsForMutation(ids)
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Permission{}).
			Where("id IN ?", ids).
			Where("is_del = ?", CommonNo).
			Update("is_del", CommonYes).Error; err != nil {
			return err
		}
		return tx.Model(&RolePermission{}).
			Where("permission_id IN ?", ids).
			Where("is_del = ?", CommonNo).
			Update("is_del", CommonYes).Error
	})
}

func (r *GormRepository) RoleIDsByPermissionIDs(ctx context.Context, permissionIDs []int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	permissionIDs = normalizeIDsForMutation(permissionIDs)
	if len(permissionIDs) == 0 {
		return []int64{}, nil
	}

	var roleIDs []int64
	err := r.db.WithContext(ctx).
		Model(&RolePermission{}).
		Where("permission_id IN ?", permissionIDs).
		Where("is_del = ?", CommonNo).
		Distinct("role_id").
		Order("role_id asc").
		Pluck("role_id", &roleIDs).Error
	if err != nil {
		return nil, err
	}
	return roleIDs, nil
}

func (r *GormRepository) UserIDsByRoleIDs(ctx context.Context, roleIDs []int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	roleIDs = normalizeIDsForMutation(roleIDs)
	if len(roleIDs) == 0 {
		return []int64{}, nil
	}

	var userIDs []int64
	err := r.db.WithContext(ctx).
		Table("users").
		Where("role_id IN ?", roleIDs).
		Where("is_del = ?", CommonNo).
		Order("id asc").
		Pluck("id", &userIDs).Error
	if err != nil {
		return nil, err
	}
	return userIDs, nil
}

type permissionWriteRow struct {
	ID        int64   `gorm:"column:id"`
	Name      string  `gorm:"column:name"`
	Path      string  `gorm:"column:path"`
	Icon      string  `gorm:"column:icon"`
	ParentID  int64   `gorm:"column:parent_id"`
	Component string  `gorm:"column:component"`
	Platform  string  `gorm:"column:platform"`
	Type      int     `gorm:"column:type"`
	Sort      int     `gorm:"column:sort"`
	Code      *string `gorm:"column:code"`
	I18nKey   string  `gorm:"column:i18n_key"`
	ShowMenu  int     `gorm:"column:show_menu"`
	Status    int     `gorm:"column:status"`
	IsDel     int     `gorm:"column:is_del"`
}

func (permissionWriteRow) TableName() string {
	return "permissions"
}

func permissionWriteRowFrom(row Permission) permissionWriteRow {
	return permissionWriteRow{
		ID:        row.ID,
		Name:      row.Name,
		Path:      row.Path,
		Icon:      row.Icon,
		ParentID:  row.ParentID,
		Component: row.Component,
		Platform:  row.Platform,
		Type:      row.Type,
		Sort:      row.Sort,
		Code:      permissionCodeValue(row.Code),
		I18nKey:   row.I18nKey,
		ShowMenu:  row.ShowMenu,
		Status:    row.Status,
		IsDel:     row.IsDel,
	}
}

func permissionCreateFields(row Permission) map[string]any {
	return map[string]any{
		"name":      row.Name,
		"path":      row.Path,
		"icon":      row.Icon,
		"parent_id": row.ParentID,
		"component": row.Component,
		"platform":  row.Platform,
		"type":      row.Type,
		"sort":      row.Sort,
		"code":      permissionCodeAny(row.Code),
		"i18n_key":  row.I18nKey,
		"show_menu": row.ShowMenu,
		"status":    row.Status,
		"is_del":    row.IsDel,
	}
}

func permissionUpdateFields(fields map[string]any) map[string]any {
	if fields == nil {
		return nil
	}

	normalized := make(map[string]any, len(fields))
	for key, value := range fields {
		if key == "code" {
			if code, ok := value.(string); ok && code == "" {
				normalized[key] = nil
				continue
			}
		}
		normalized[key] = value
	}
	return normalized
}

func permissionCodeValue(code string) *string {
	if code == "" {
		return nil
	}
	return &code
}

func permissionCodeAny(code string) any {
	if code == "" {
		return nil
	}
	return code
}
