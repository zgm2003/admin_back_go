package userquickentry

import (
	"context"

	"admin_back_go/internal/apperror"
)

const maxQuickEntryCount = 6

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Save(ctx context.Context, userID int64, input SaveInput) (*SaveResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("userquickentry.repository_missing", nil, "快捷入口仓储未配置")
	}

	permissionIDs := normalizePermissionIDs(input.PermissionIDs)
	if len(permissionIDs) > maxQuickEntryCount {
		return nil, apperror.BadRequestKey("userquickentry.too_many", nil, "快捷入口最多保留6个")
	}

	activeIDs, err := s.repository.ActiveAdminPagePermissionIDs(ctx, permissionIDs)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "userquickentry.permission_query_failed", nil, "查询快捷入口权限失败", err)
	}
	for _, id := range permissionIDs {
		if _, ok := activeIDs[id]; !ok {
			return nil, apperror.BadRequestKey("userquickentry.invalid_permission", nil, "快捷入口仅支持启用的后台页面权限")
		}
	}

	entries, err := s.repository.ReplaceForUser(ctx, userID, permissionIDs)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "userquickentry.save_failed", nil, "保存快捷入口失败", err)
	}
	return &SaveResponse{QuickEntry: entries}, nil
}

func normalizePermissionIDs(ids []int64) []int64 {
	result := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
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
