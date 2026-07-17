package permission

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrPrincipalRepositoryNotConfigured = errors.New("principal repository not configured")
	ErrPrincipalNotFound                = errors.New("principal user not found")
)

type PrincipalRepository interface {
	LoadSnapshot(context.Context, int64, string) (PrincipalSnapshot, error)
	CurrentVersions(context.Context, []PrincipalSubject) ([]PrincipalVersion, error)
	AllVersions(context.Context) ([]PrincipalVersion, error)
}

type PrincipalVersionBumper interface {
	BumpPrincipalVersions(context.Context, []PrincipalSubject) ([]PrincipalVersion, error)
}

type GormPrincipalRepository struct {
	db *gorm.DB
}

func NewGormPrincipalRepository(client *database.Client) PrincipalRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormPrincipalRepository{db: client.Gorm}
}

type principalIdentityRow struct {
	UserID     int64  `gorm:"column:user_id"`
	RoleID     int64  `gorm:"column:role_id"`
	UserStatus int    `gorm:"column:user_status"`
	UserIsDel  int    `gorm:"column:user_is_del"`
	RoleIsDel  *int   `gorm:"column:role_is_del"`
	Version    uint64 `gorm:"column:version"`
}

func (r *GormPrincipalRepository) LoadSnapshot(ctx context.Context, userID int64, platform string) (PrincipalSnapshot, error) {
	if r == nil || r.db == nil {
		return PrincipalSnapshot{}, ErrPrincipalRepositoryNotConfigured
	}
	platform = strings.TrimSpace(platform)
	if userID <= 0 || platform == "" {
		return PrincipalSnapshot{}, ErrPrincipalNotFound
	}
	if platform != enum.PlatformAdmin {
		return PrincipalSnapshot{}, fmt.Errorf("unsupported principal platform %q", platform)
	}

	var identity principalIdentityRow
	err := r.db.WithContext(ctx).
		Table("users AS u").
		Select("u.id AS user_id, u.role_id, u.status AS user_status, u.is_del AS user_is_del, r.is_del AS role_is_del, COALESCE(v.version, 1) AS version").
		Joins("LEFT JOIN roles AS r ON r.id = u.role_id").
		Joins("LEFT JOIN authz_principal_versions AS v ON v.user_id = u.id AND v.platform = ?", platform).
		Where("u.id = ?", userID).
		Take(&identity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return PrincipalSnapshot{}, ErrPrincipalNotFound
	}
	if err != nil {
		return PrincipalSnapshot{}, err
	}

	snapshot := PrincipalSnapshot{
		UserID:      identity.UserID,
		RoleID:      identity.RoleID,
		Platform:    platform,
		Version:     identity.Version,
		UserActive:  identity.UserStatus == StatusActive && identity.UserIsDel == CommonNo,
		RoleActive:  identity.RoleID > 0 && identity.RoleIsDel != nil && *identity.RoleIsDel == CommonNo,
		RouteCodes:  []string{},
		ButtonCodes: []string{},
	}
	if snapshot.Version == 0 {
		snapshot.Version = 1
	}
	if !snapshot.UserActive || !snapshot.RoleActive {
		return snapshot, nil
	}

	grantedIDs, err := principalPermissionIDsByRole(ctx, r.db, identity.RoleID)
	if err != nil {
		return PrincipalSnapshot{}, err
	}
	if len(grantedIDs) == 0 {
		return snapshot, nil
	}
	permissions, err := principalActivePermissions(ctx, r.db)
	if err != nil {
		return PrincipalSnapshot{}, err
	}
	permissionMap := permissionMapByPlatform(permissions, platform)
	enabledIDs := resolveEnabledIDs(normalizeIDs(grantedIDs), permissionMap)
	permissionContext := buildContext(enabledIDs, permissionMap)
	snapshot.RouteCodes = append([]string(nil), permissionContext.RouteAccessCodes...)
	snapshot.ButtonCodes = append([]string(nil), permissionContext.ButtonCodes...)
	return snapshot, nil
}

func principalPermissionIDsByRole(ctx context.Context, db *gorm.DB, roleID int64) ([]int64, error) {
	var ids []int64
	err := db.WithContext(ctx).
		Table("role_permissions").
		Where("role_id = ?", roleID).
		Where("is_del = ?", CommonNo).
		Order("id ASC").
		Pluck("permission_id", &ids).Error
	return ids, err
}

func principalActivePermissions(ctx context.Context, db *gorm.DB) ([]Permission, error) {
	var rows []Permission
	err := db.WithContext(ctx).
		Where("is_del = ?", CommonNo).
		Where("status = ?", StatusActive).
		Order("parent_id ASC").
		Order("sort ASC").
		Order("id ASC").
		Find(&rows).Error
	return rows, err
}

func (r *GormPrincipalRepository) CurrentVersions(ctx context.Context, subjects []PrincipalSubject) ([]PrincipalVersion, error) {
	if r == nil || r.db == nil {
		return nil, ErrPrincipalRepositoryNotConfigured
	}
	return loadPrincipalVersions(ctx, r.db, subjects, false)
}

func (r *GormPrincipalRepository) AllVersions(ctx context.Context) ([]PrincipalVersion, error) {
	if r == nil || r.db == nil {
		return nil, ErrPrincipalRepositoryNotConfigured
	}
	var rows []PrincipalVersion
	err := r.db.WithContext(ctx).
		Table("users AS u").
		Select("u.id AS user_id, u.role_id, ? AS platform, COALESCE(v.version, 1) AS version", enum.PlatformAdmin).
		Joins("LEFT JOIN authz_principal_versions AS v ON v.user_id = u.id AND v.platform = ?", enum.PlatformAdmin).
		Order("u.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for index := range rows {
		if rows[index].Version == 0 {
			rows[index].Version = 1
		}
	}
	return normalizePrincipalVersions(rows), nil
}

func loadPrincipalVersions(ctx context.Context, db *gorm.DB, subjects []PrincipalSubject, lock bool) ([]PrincipalVersion, error) {
	subjects = normalizePrincipalSubjects(subjects)
	if len(subjects) == 0 {
		return []PrincipalVersion{}, nil
	}
	ids := make([]int64, 0, len(subjects))
	for _, subject := range subjects {
		if subject.Platform != enum.PlatformAdmin {
			return nil, fmt.Errorf("unsupported principal platform %q", subject.Platform)
		}
		ids = append(ids, subject.UserID)
	}
	query := db.WithContext(ctx).
		Table("users AS u").
		Select("u.id AS user_id, u.role_id, ? AS platform, COALESCE(v.version, 1) AS version", enum.PlatformAdmin).
		Joins("LEFT JOIN authz_principal_versions AS v ON v.user_id = u.id AND v.platform = ?", enum.PlatformAdmin).
		Where("u.id IN ?", ids).
		Order("u.id ASC")
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "u"}})
	}
	var rows []PrincipalVersion
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) != len(subjects) {
		return nil, ErrPrincipalNotFound
	}
	for index := range rows {
		if rows[index].Version == 0 {
			rows[index].Version = 1
		}
	}
	return normalizePrincipalVersions(rows), nil
}

func BumpPrincipalVersions(ctx context.Context, db *gorm.DB, subjects []PrincipalSubject) ([]PrincipalVersion, error) {
	if db == nil {
		return nil, ErrPrincipalRepositoryNotConfigured
	}
	subjects = normalizePrincipalSubjects(subjects)
	if len(subjects) == 0 {
		return []PrincipalVersion{}, nil
	}
	ids := make([]int64, 0, len(subjects))
	for _, subject := range subjects {
		if subject.Platform != enum.PlatformAdmin {
			return nil, fmt.Errorf("unsupported principal platform %q", subject.Platform)
		}
		ids = append(ids, subject.UserID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	var lockedIDs []int64
	if err := db.WithContext(ctx).
		Table("users").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id IN ?", ids).
		Order("id ASC").
		Pluck("id", &lockedIDs).Error; err != nil {
		return nil, err
	}
	if len(lockedIDs) != len(ids) {
		return nil, ErrPrincipalNotFound
	}

	now := time.Now().UTC()
	for _, userID := range ids {
		if err := db.WithContext(ctx).Exec(
			"INSERT INTO authz_principal_versions (user_id, platform, version, updated_at) VALUES (?, ?, 1, ?) ON DUPLICATE KEY UPDATE user_id = VALUES(user_id)",
			userID,
			enum.PlatformAdmin,
			now,
		).Error; err != nil {
			return nil, err
		}
	}
	if err := db.WithContext(ctx).
		Table("authz_principal_versions").
		Where("platform = ?", enum.PlatformAdmin).
		Where("user_id IN ?", ids).
		Updates(map[string]any{"version": gorm.Expr("version + 1"), "updated_at": now}).Error; err != nil {
		return nil, err
	}
	return loadPrincipalVersions(ctx, db, subjects, false)
}
