package uploadconfig

import (
	"errors"

	"admin_back_go/internal/apperror"
)

var ErrRepositoryNotConfigured = errors.New("upload config repository is not configured")

const ErrRepositoryNotConfiguredMessage = "上传配置仓储未配置"

func repositoryNotConfigured() *apperror.Error {
	return apperror.Internal(ErrRepositoryNotConfiguredMessage)
}
