package authplatform

import "admin_back_go/internal/apperror"

const ErrManagementRepositoryNotConfiguredMessage = "认证平台管理仓储未配置"

func managementRepositoryNotConfigured() *apperror.Error {
	return apperror.InternalKey("authplatform.repository_missing", nil, ErrManagementRepositoryNotConfiguredMessage)
}
