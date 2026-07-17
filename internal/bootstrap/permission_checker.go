package bootstrap

import (
	"context"

	"admin_back_go/internal/middleware"
	"admin_back_go/internal/shared/apperror"
)

type principalAuthorizer interface {
	Authorize(context.Context, int64, string, string) *apperror.Error
}

// PermissionCheckerFor keeps transport policy independent from the principal
// implementation while preserving the principal service's fail-closed errors.
func PermissionCheckerFor(authorizer principalAuthorizer) middleware.PermissionChecker {
	return func(ctx context.Context, input middleware.PermissionInput) *apperror.Error {
		if authorizer == nil {
			return apperror.InternalKey("permission.principal_service_missing", nil, "权限主体服务未配置")
		}
		return authorizer.Authorize(ctx, input.UserID, input.Platform, input.Code)
	}
}
