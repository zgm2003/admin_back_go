package role

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/module/permission"
)

const (
	minPageSize    = 1
	maxPageSize    = 50
	maxRoleNameLen = 64
	timeLayout     = "2006-01-02 15:04:05"
)

type PermissionDictionary interface {
	Init(ctx context.Context) (*permission.InitResponse, *apperror.Error)
}

type CacheInvalidator interface {
	Delete(ctx context.Context, key string) error
}

type Service struct {
	repository           Repository
	permissionDictionary PermissionDictionary
	cacheInvalidator     CacheInvalidator
	platforms            []string
}

func NewService(repository Repository, permissionDictionary PermissionDictionary, cacheInvalidator CacheInvalidator, platforms []string) *Service {
	if len(platforms) == 0 {
		platforms = []string{"admin", "app"}
	}
	return &Service{
		repository:           repository,
		permissionDictionary: permissionDictionary,
		cacheInvalidator:     cacheInvalidator,
		platforms:            normalizePlatforms(platforms),
	}
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	if s == nil || s.permissionDictionary == nil {
		return nil, apperror.Internal("权限字典服务未配置")
	}

	result, appErr := s.permissionDictionary.Init(ctx)
	if appErr != nil {
		return nil, appErr
	}
	if result == nil {
		return &InitResponse{}, nil
	}
	return &InitResponse{
		Dict: InitDict{
			PermissionTree:        result.Dict.PermissionTree,
			PermissionPlatformArr: result.Dict.PermissionPlatformArr,
		},
	}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("角色仓储未配置")
	}
	if appErr := validateListQuery(query); appErr != nil {
		return nil, appErr
	}

	query.Name = strings.TrimSpace(query.Name)
	rows, total, err := s.repository.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询角色失败", err)
	}

	roleIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		roleIDs = append(roleIDs, row.ID)
	}
	permissionMap, err := s.repository.PermissionIDsByRoleIDs(ctx, roleIDs)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询角色权限失败", err)
	}
	activePermissions, err := s.repository.AllActivePermissions(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询权限失败", err)
	}

	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, ListItem{
			ID:            row.ID,
			Name:          row.Name,
			PermissionIDs: normalizeAssignablePermissionIDs(permissionMap[row.ID], activePermissions),
			IsDefault:     row.IsDefault,
			CreatedAt:     formatTime(row.CreatedAt),
			UpdatedAt:     formatTime(row.UpdatedAt),
		})
	}

	return &ListResponse{
		List: list,
		Page: Page{
			PageSize:    query.PageSize,
			CurrentPage: query.CurrentPage,
			TotalPage:   totalPage(total, query.PageSize),
			Total:       total,
		},
	}, nil
}

func (s *Service) Create(ctx context.Context, input MutationInput) (int64, *apperror.Error) {
	if s == nil || s.repository == nil {
		return 0, apperror.Internal("角色仓储未配置")
	}
	input, appErr := s.normalizeMutation(ctx, input)
	if appErr != nil {
		return 0, appErr
	}

	deletedRole, err := s.repository.FindDeletedByName(ctx, input.Name)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "查询已删除角色失败", err)
	}
	exists, err := s.repository.ExistsByName(ctx, input.Name, 0)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "校验角色名失败", err)
	}
	if exists {
		return 0, apperror.BadRequest("角色名已存在")
	}

	var roleID int64
	if deletedRole != nil {
		err = s.repository.WithTx(ctx, func(tx Repository) error {
			if err := tx.RestoreDeleted(ctx, deletedRole.ID, Role{
				Name:      input.Name,
				IsDefault: permission.CommonNo,
				IsDel:     permission.CommonNo,
			}); err != nil {
				return err
			}
			roleID = deletedRole.ID
			return tx.SyncPermissions(ctx, deletedRole.ID, input.PermissionIDs)
		})
		if err != nil {
			return 0, apperror.Wrap(apperror.CodeInternal, 500, "恢复角色失败", err)
		}
		return roleID, nil
	}

	err = s.repository.WithTx(ctx, func(tx Repository) error {
		id, err := tx.Create(ctx, Role{
			Name:      input.Name,
			IsDefault: permission.CommonNo,
			IsDel:     permission.CommonNo,
		})
		if err != nil {
			return err
		}
		roleID = id
		return tx.SyncPermissions(ctx, id, input.PermissionIDs)
	})
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增角色失败", err)
	}
	return roleID, nil
}

func (s *Service) Update(ctx context.Context, id int64, input MutationInput) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的角色ID")
	}
	if s == nil || s.repository == nil {
		return apperror.Internal("角色仓储未配置")
	}

	role, err := s.repository.RoleByID(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询角色失败", err)
	}
	if role == nil {
		return apperror.NotFound("角色不存在")
	}

	input, appErr := s.normalizeMutation(ctx, input)
	if appErr != nil {
		return appErr
	}
	exists, err := s.repository.ExistsByName(ctx, input.Name, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验角色名失败", err)
	}
	if exists {
		return apperror.BadRequest("角色名已存在")
	}

	if err := s.repository.WithTx(ctx, func(tx Repository) error {
		if err := tx.Update(ctx, id, map[string]any{"name": input.Name}); err != nil {
			return err
		}
		return tx.SyncPermissions(ctx, id, input.PermissionIDs)
	}); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "更新角色失败", err)
	}

	if appErr := s.invalidateRoleUsers(ctx, []int64{id}); appErr != nil {
		return appErr
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, ids []int64) *apperror.Error {
	if s == nil || s.repository == nil {
		return apperror.Internal("角色仓储未配置")
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return apperror.BadRequest("请选择要删除的角色")
	}

	roles, err := s.repository.RolesByIDs(ctx, ids)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询角色失败", err)
	}
	if len(roles) == 0 {
		return apperror.NotFound("角色不存在")
	}
	if len(roles) != len(ids) {
		return apperror.BadRequest("包含不存在的角色")
	}
	hasDefault, err := s.repository.HasDefaultIn(ctx, ids)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验默认角色失败", err)
	}
	if hasDefault {
		return apperror.BadRequest("默认角色不能删除")
	}
	userCount, err := s.repository.CountUsersByRoleIDs(ctx, ids)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验角色用户失败", err)
	}
	if userCount > 0 {
		return apperror.BadRequest("角色已绑定用户，不能删除")
	}

	if err := s.repository.WithTx(ctx, func(tx Repository) error {
		if err := tx.Delete(ctx, ids); err != nil {
			return err
		}
		return tx.DeleteRolePermissionsByRoleIDs(ctx, ids)
	}); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除角色失败", err)
	}
	return nil
}

func (s *Service) SetDefault(ctx context.Context, id int64) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的角色ID")
	}
	if s == nil || s.repository == nil {
		return apperror.Internal("角色仓储未配置")
	}

	role, err := s.repository.RoleByID(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询角色失败", err)
	}
	if role == nil {
		return apperror.NotFound("角色不存在")
	}

	if err := s.repository.WithTx(ctx, func(tx Repository) error {
		if err := tx.ClearDefault(ctx); err != nil {
			return err
		}
		return tx.SetDefault(ctx, id)
	}); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "设置默认角色失败", err)
	}
	return nil
}

func (s *Service) normalizeMutation(ctx context.Context, input MutationInput) (MutationInput, *apperror.Error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return input, apperror.BadRequest("角色名不能为空")
	}
	if len([]rune(input.Name)) > maxRoleNameLen {
		return input, apperror.BadRequest("角色名不能超过64个字符")
	}

	activePermissions, err := s.repository.AllActivePermissions(ctx)
	if err != nil {
		return input, apperror.Wrap(apperror.CodeInternal, 500, "查询权限失败", err)
	}
	input.PermissionIDs = normalizeAssignablePermissionIDs(input.PermissionIDs, activePermissions)
	return input, nil
}

func (s *Service) invalidateRoleUsers(ctx context.Context, roleIDs []int64) *apperror.Error {
	if s.cacheInvalidator == nil {
		return nil
	}
	userIDs, err := s.repository.UserIDsByRoleIDs(ctx, roleIDs)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询角色用户失败", err)
	}
	for _, userID := range normalizeIDs(userIDs) {
		for _, platform := range s.platforms {
			if err := s.cacheInvalidator.Delete(ctx, permission.ButtonCacheKey(userID, platform)); err != nil {
				return apperror.Wrap(apperror.CodeInternal, 500, "清理权限缓存失败", err)
			}
		}
	}
	return nil
}

func validateListQuery(query ListQuery) *apperror.Error {
	if query.CurrentPage <= 0 {
		return apperror.BadRequest("当前页无效")
	}
	if query.PageSize < minPageSize || query.PageSize > maxPageSize {
		return apperror.BadRequest("每页数量无效")
	}
	return nil
}

func normalizeAssignablePermissionIDs(ids []int64, permissions []permission.Permission) []int64 {
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return []int64{}
	}

	permissionMap := make(map[int64]permission.Permission, len(permissions))
	for _, row := range permissions {
		if row.ID <= 0 || row.IsDel == permission.CommonYes || row.Status != permission.StatusActive {
			continue
		}
		permissionMap[row.ID] = row
	}

	result := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		row, ok := permissionMap[id]
		if !ok {
			continue
		}
		switch row.Type {
		case permission.TypePage:
			result[id] = struct{}{}
		case permission.TypeButton:
			result[id] = struct{}{}
			parent, ok := permissionMap[row.ParentID]
			if ok && parent.Type == permission.TypePage {
				result[parent.ID] = struct{}{}
			}
		}
	}

	normalized := make([]int64, 0, len(result))
	for id := range result {
		normalized = append(normalized, id)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i] < normalized[j]
	})
	return normalized
}

func normalizeIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func normalizePlatforms(platforms []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(platforms))
	for _, platform := range platforms {
		platform = strings.TrimSpace(platform)
		if platform == "" {
			continue
		}
		if _, ok := seen[platform]; ok {
			continue
		}
		seen[platform] = struct{}{}
		result = append(result, platform)
	}
	sort.Strings(result)
	return result
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}
