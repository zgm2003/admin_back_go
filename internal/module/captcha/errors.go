package captcha

import "errors"

var (
	ErrEngineNotConfigured = errors.New("captcha engine is not configured")
	ErrStoreNotConfigured  = errors.New("captcha store is not configured")
)
