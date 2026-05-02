package user

import (
	"context"
	"encoding/json"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/platform/redisclient"
)

const defaultButtonCacheTTL = 30 * time.Minute

type PermissionBuilder interface {
	BuildContextByRole(ctx context.Context, roleID int64, platform string) (permission.Context, *apperror.Error)
}

type ButtonCache interface {
	Set(ctx context.Context, key string, values []string, ttl time.Duration) error
}

type RedisButtonCache struct {
	client *redisclient.Client
}

func NewRedisButtonCache(client *redisclient.Client) ButtonCache {
	if client == nil || client.Redis == nil {
		return nil
	}
	return &RedisButtonCache{client: client}
}

func (c *RedisButtonCache) Set(ctx context.Context, key string, values []string, ttl time.Duration) error {
	if c == nil || c.client == nil || c.client.Redis == nil {
		return nil
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return err
	}
	return c.client.Redis.Set(ctx, key, payload, ttl).Err()
}

type Service struct {
	repository        Repository
	permissionBuilder PermissionBuilder
	buttonCache       ButtonCache
	buttonCacheTTL    time.Duration
}

func NewService(repository Repository, permissionBuilder PermissionBuilder, buttonCache ButtonCache, buttonCacheTTL time.Duration) *Service {
	if buttonCacheTTL <= 0 {
		buttonCacheTTL = defaultButtonCacheTTL
	}
	return &Service{
		repository:        repository,
		permissionBuilder: permissionBuilder,
		buttonCache:       buttonCache,
		buttonCacheTTL:    buttonCacheTTL,
	}
}

func (s *Service) Init(ctx context.Context, input InitInput) (*InitResponse, *apperror.Error) {
	if input.UserID <= 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("用户仓储未配置")
	}
	if s.permissionBuilder == nil {
		return nil, apperror.Internal("权限服务未配置")
	}

	currentUser, err := s.repository.FindUser(ctx, input.UserID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询用户失败", err)
	}
	if currentUser == nil {
		return nil, apperror.NotFound("用户不存在")
	}

	profile, err := s.repository.FindProfile(ctx, currentUser.ID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询用户资料失败", err)
	}

	role, err := s.repository.FindRole(ctx, currentUser.RoleID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询角色失败", err)
	}

	perm, appErr := s.permissionBuilder.BuildContextByRole(ctx, currentUser.RoleID, input.Platform)
	if appErr != nil {
		return nil, appErr
	}

	quickEntry, err := s.repository.QuickEntries(ctx, currentUser.ID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询快捷入口失败", err)
	}

	if s.buttonCache != nil {
		_ = s.buttonCache.Set(ctx, permission.ButtonCacheKey(currentUser.ID, input.Platform), perm.ButtonCodes, s.buttonCacheTTL)
	}

	avatar := ""
	if profile != nil {
		avatar = profile.Avatar
	}
	roleName := ""
	if role != nil {
		roleName = role.Name
	}

	return &InitResponse{
		UserID:      currentUser.ID,
		Username:    currentUser.Username,
		Avatar:      avatar,
		RoleName:    roleName,
		Permissions: perm.Permissions,
		Router:      perm.Router,
		ButtonCodes: perm.ButtonCodes,
		QuickEntry:  quickEntry,
	}, nil
}
