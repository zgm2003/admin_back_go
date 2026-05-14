package systemsetting

import "admin_back_go/internal/apperror"

const ErrRepositoryNotConfiguredMessage = "系统设置仓储未配置"

func repositoryNotConfigured() *apperror.Error {
	return apperror.InternalKey("systemsetting.repository_missing", nil, ErrRepositoryNotConfiguredMessage)
}
