package mail

import "errors"

var (
	ErrRepositoryNotConfigured       = errors.New("mail repository is not configured")
	ErrSenderNotConfigured           = errors.New("mail sender is not configured")
	ErrDiagnosticCipherNotConfigured = errors.New("mail diagnostic cipher is not configured")
	ErrInvalidDiagnosticSnapshot     = errors.New("mail diagnostic snapshot is invalid")
	ErrVerificationDeadlineElapsed   = errors.New("mail verification deadline elapsed")
	ErrLogFinalizationFailed         = errors.New("mail log finalization failed")
)
