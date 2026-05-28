package systemsetting

import "admin_back_go/internal/shared/apperror"

const ErrRepositoryNotConfiguredMessage = "系统设置仓储未配置"

func repositoryNotConfigured() *apperror.Error {
	return apperror.InternalKey("systemsetting.repository_missing", nil, ErrRepositoryNotConfiguredMessage)
}
