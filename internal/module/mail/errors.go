package mail

import "errors"

var (
	ErrRepositoryNotConfigured = errors.New("mail repository is not configured")
	ErrSenderNotConfigured     = errors.New("mail sender is not configured")
)
