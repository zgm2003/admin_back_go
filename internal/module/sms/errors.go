package sms

import "errors"

var (
	ErrRepositoryNotConfigured = errors.New("sms repository is not configured")
	ErrSenderNotConfigured     = errors.New("sms sender is not configured")
)
