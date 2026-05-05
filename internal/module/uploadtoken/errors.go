package uploadtoken

import "errors"

var ErrRepositoryNotConfigured = errors.New("upload token repository is not configured")

const ErrRepositoryNotConfiguredMessage = "上传运行时仓储未配置"
