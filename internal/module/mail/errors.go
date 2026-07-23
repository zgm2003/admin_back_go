package mail

import "errors"

var (
	ErrRepositoryNotConfigured       = errors.New("mail repository is not configured")
	ErrSenderNotConfigured           = errors.New("mail sender is not configured")
	ErrDiagnosticCipherNotConfigured = errors.New("mail diagnostic cipher is not configured")
	ErrInvalidDiagnosticSnapshot     = errors.New("mail diagnostic snapshot is invalid")
	ErrVerificationDeadlineElapsed   = errors.New("mail verification deadline elapsed")
	ErrLogFinalizationFailed         = errors.New("mail log finalization failed")

	ErrDiagnosticRekeyRepositoryNotConfigured = errors.New("mail diagnostic rekey repository is not configured")
	ErrDiagnosticRekeyRepositoryFailure       = errors.New("mail diagnostic rekey repository failed")
	ErrDiagnosticRekeyLockUnavailable         = errors.New("mail diagnostic rekey lock is unavailable")
	ErrDiagnosticRekeyUnknownKey              = errors.New("mail diagnostic rekey key is unknown")
	ErrDiagnosticRekeyCorruptCipher           = errors.New("mail diagnostic rekey cipher is corrupt")
	ErrDiagnosticRekeyOptimisticCompareFailed = errors.New("mail diagnostic rekey optimistic compare failed")
	ErrDiagnosticRekeyOutputFailed            = errors.New("mail diagnostic rekey output failed")
	ErrDiagnosticRekeyIncomplete              = errors.New("mail diagnostic rekey verification failed")
)
