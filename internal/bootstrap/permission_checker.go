package bootstrap

import (
	"context"
	"net/http"
	"strings"
	"time"

	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/shared/apperror"
)

const defaultPermissionCheckCacheTTL = 30 * time.Minute

type permissionUserRepository interface {
	FindUser(ctx context.Context, userID int64) (*user.User, error)
	FindRole(ctx context.Context, roleID int64) (*user.Role, error)
}

type permissionContextBuilder interface {
	BuildContextByRole(ctx context.Context, roleID int64, platform string) (permission.Context, *apperror.Error)
}

type permissionRouteAccessGrantCache interface {
	Get(ctx context.Context, key string) ([]string, bool, error)
	Set(ctx context.Context, key string, values []string, ttl time.Duration) error
}

// PermissionCheckerFor builds a fail-closed RBAC checker for protected route metadata.
func PermissionCheckerFor(repository permissionUserRepository, builder permissionContextBuilder, cache permissionRouteAccessGrantCache, cacheTTL time.Duration) middleware.PermissionChecker {
	if cacheTTL <= 0 {
		cacheTTL = defaultPermissionCheckCacheTTL
	}
	return func(ctx context.Context, input middleware.PermissionInput) *apperror.Error {
		if input.UserID <= 0 {
			return apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
		}
		if repository == nil {
			return apperror.Internal("用户仓储未配置")
		}
		if builder == nil {
			return apperror.Internal("权限服务未配置")
		}

		code := strings.TrimSpace(input.Code)
		if code == "" {
			return apperror.ForbiddenKey("permission.code_missing", nil, "权限标识未配置")
		}

		currentUser, err := repository.FindUser(ctx, input.UserID)
		if err != nil {
			return apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "查询用户失败", err)
		}
		if currentUser == nil {
			return apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
		}
		if currentUser.RoleID <= 0 {
			return apperror.ForbiddenKey("permission.api.denied", nil, "无接口权限")
		}

		role, err := repository.FindRole(ctx, currentUser.RoleID)
		if err != nil {
			return apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "查询角色失败", err)
		}
		if role == nil {
			return apperror.ForbiddenKey("permission.api.denied", nil, "无接口权限")
		}

		cacheKey := permission.RouteAccessCacheKey(currentUser.ID, input.Platform)
		routeAccessCodes, hit := cachedRouteAccessCodes(ctx, cache, cacheKey)
		if !hit {
			permissionContext, appErr := builder.BuildContextByRole(ctx, currentUser.RoleID, input.Platform)
			if appErr != nil {
				return appErr
			}
			routeAccessCodes = permissionContext.RouteAccessCodes
			if cache != nil {
				_ = cache.Set(ctx, cacheKey, routeAccessCodes, cacheTTL)
			}
		}

		for _, ownedCode := range routeAccessCodes {
			if ownedCode == code {
				return nil
			}
		}
		return apperror.ForbiddenKey("permission.api.denied", nil, "无接口权限")
	}
}

func cachedRouteAccessCodes(ctx context.Context, cache permissionRouteAccessGrantCache, key string) ([]string, bool) {
	if cache == nil {
		return nil, false
	}
	values, hit, err := cache.Get(ctx, key)
	if err != nil {
		return nil, false
	}
	return values, hit
}
