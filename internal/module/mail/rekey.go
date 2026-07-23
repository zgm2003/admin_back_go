package mail

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"admin_back_go/internal/infra/secretbox"
)

const (
	DefaultDiagnosticRekeyBatchSize = 100
	DiagnosticRekeyLockName         = "admin_go:mail-diagnostic-rekey:v1"
)

const diagnosticKeyIDPrefix = "mail-diagnostic-v1-"

type DiagnosticCipherRow struct {
	ID      uint64 `gorm:"column:id"`
	KeyID   string `gorm:"column:key_id"`
	CodeEnc string `gorm:"column:code_enc"`
}

type DiagnosticCipherRewrite struct {
	ID         uint64
	OldKeyID   string
	OldCodeEnc string
	NewKeyID   string
	NewCodeEnc string
}

type DiagnosticRekeyObserverFunc func(uint64) error

type DiagnosticRekeyRepository interface {
	WithDiagnosticRekeyLock(context.Context, string, func(DiagnosticRekeyRepository) error) error
	DistinctDiagnosticKeyIDs(context.Context) ([]string, error)
	ListDiagnosticCipherRows(context.Context, string, uint64, int) ([]DiagnosticCipherRow, error)
	RewriteDiagnosticCipherBatch(context.Context, []DiagnosticCipherRewrite) error
	CountDiagnosticKeyID(context.Context, string) (int64, error)
	CountUnknownDiagnosticKeyIDs(context.Context, []string) (int64, error)
}

type DiagnosticRekeyResult struct {
	CurrentKeyID       string
	PreviousKeyID      string
	Scanned            uint64
	Rekeyed            uint64
	PreviousReferences int64
	UnknownReferences  int64
}

type DiagnosticRekeyService struct {
	repository    DiagnosticRekeyRepository
	box           secretbox.VersionedBox
	previousKeyID string
	observer      DiagnosticRekeyObserverFunc
}

func NewDiagnosticRekeyService(repository DiagnosticRekeyRepository, box secretbox.VersionedBox, previousKeyID string, observer DiagnosticRekeyObserverFunc) *DiagnosticRekeyService {
	return &DiagnosticRekeyService{
		repository:    repository,
		box:           box,
		previousKeyID: previousKeyID,
		observer:      observer,
	}
}

func (s *DiagnosticRekeyService) Run(ctx context.Context) (DiagnosticRekeyResult, error) {
	if s == nil || s.repository == nil {
		return DiagnosticRekeyResult{}, ErrDiagnosticRekeyRepositoryNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}

	currentKeyID, allowedKeyIDs, err := s.validateKeys()
	result := DiagnosticRekeyResult{CurrentKeyID: currentKeyID, PreviousKeyID: s.previousKeyID}
	if err != nil {
		return result, err
	}

	err = s.repository.WithDiagnosticRekeyLock(ctx, DiagnosticRekeyLockName, func(locked DiagnosticRekeyRepository) error {
		if locked == nil {
			return ErrDiagnosticRekeyRepositoryNotConfigured
		}
		keyIDs, listErr := locked.DistinctDiagnosticKeyIDs(ctx)
		if listErr != nil {
			return fixedDiagnosticRekeyRepositoryError(listErr)
		}
		allowed := make(map[string]struct{}, len(allowedKeyIDs))
		for _, keyID := range allowedKeyIDs {
			allowed[keyID] = struct{}{}
		}
		for _, keyID := range keyIDs {
			if keyID == "" {
				return ErrDiagnosticRekeyUnknownKey
			}
			if _, ok := allowed[keyID]; !ok {
				return ErrDiagnosticRekeyUnknownKey
			}
		}

		if s.previousKeyID != "" {
			if runErr := s.rekeyPreviousRows(ctx, locked, &result); runErr != nil {
				return runErr
			}
			previousReferences, countErr := locked.CountDiagnosticKeyID(ctx, s.previousKeyID)
			if countErr != nil {
				return fixedDiagnosticRekeyRepositoryError(countErr)
			}
			result.PreviousReferences = previousReferences
		}

		unknownReferences, countErr := locked.CountUnknownDiagnosticKeyIDs(ctx, allowedKeyIDs)
		if countErr != nil {
			return fixedDiagnosticRekeyRepositoryError(countErr)
		}
		result.UnknownReferences = unknownReferences
		if result.PreviousReferences != 0 || result.UnknownReferences != 0 {
			return ErrDiagnosticRekeyIncomplete
		}
		return nil
	})
	if err != nil {
		return result, fixedDiagnosticRekeyError(err)
	}
	return result, nil
}

func (s *DiagnosticRekeyService) validateKeys() (string, []string, error) {
	currentKeyID := s.box.CurrentKeyID()
	if !IsCanonicalDiagnosticKeyID(currentKeyID) {
		return currentKeyID, nil, ErrDiagnosticRekeyCorruptCipher
	}
	keyID, ciphertext, err := s.box.Encrypt("000000")
	if err != nil || keyID != currentKeyID || ciphertext == "" || !isCanonicalDiagnosticCiphertext(ciphertext) {
		return currentKeyID, nil, ErrDiagnosticRekeyCorruptCipher
	}

	allowed := []string{currentKeyID}
	if s.previousKeyID == "" {
		return currentKeyID, allowed, nil
	}
	if !IsCanonicalDiagnosticKeyID(s.previousKeyID) || s.previousKeyID == currentKeyID {
		return currentKeyID, nil, ErrDiagnosticRekeyUnknownKey
	}
	if _, err := s.box.Decrypt(s.previousKeyID, ""); err != nil {
		return currentKeyID, nil, ErrDiagnosticRekeyUnknownKey
	}
	return currentKeyID, append(allowed, s.previousKeyID), nil
}

func (s *DiagnosticRekeyService) rekeyPreviousRows(ctx context.Context, repository DiagnosticRekeyRepository, result *DiagnosticRekeyResult) error {
	var afterID uint64
	for {
		rows, err := repository.ListDiagnosticCipherRows(ctx, s.previousKeyID, afterID, DefaultDiagnosticRekeyBatchSize)
		if err != nil {
			return fixedDiagnosticRekeyRepositoryError(err)
		}
		if len(rows) == 0 {
			return nil
		}
		if len(rows) > DefaultDiagnosticRekeyBatchSize {
			return ErrDiagnosticRekeyCorruptCipher
		}

		rewrites := make([]DiagnosticCipherRewrite, 0, len(rows))
		lastID := afterID
		for _, row := range rows {
			if row.ID == 0 || row.ID <= lastID || row.KeyID != s.previousKeyID || row.CodeEnc == "" || !isCanonicalDiagnosticCiphertext(row.CodeEnc) {
				return ErrDiagnosticRekeyCorruptCipher
			}
			plain, decryptErr := s.box.Decrypt(s.previousKeyID, row.CodeEnc)
			if decryptErr != nil || !verifyCodePattern.MatchString(plain) {
				return ErrDiagnosticRekeyCorruptCipher
			}
			newKeyID, newCiphertext, encryptErr := s.box.Encrypt(plain)
			if encryptErr != nil || newKeyID != result.CurrentKeyID || newCiphertext == "" || !isCanonicalDiagnosticCiphertext(newCiphertext) {
				return ErrDiagnosticRekeyCorruptCipher
			}
			rewrites = append(rewrites, DiagnosticCipherRewrite{
				ID: row.ID, OldKeyID: row.KeyID, OldCodeEnc: row.CodeEnc,
				NewKeyID: newKeyID, NewCodeEnc: newCiphertext,
			})
			lastID = row.ID
		}

		if err := repository.RewriteDiagnosticCipherBatch(ctx, rewrites); err != nil {
			if errors.Is(err, ErrDiagnosticRekeyOptimisticCompareFailed) {
				return ErrDiagnosticRekeyOptimisticCompareFailed
			}
			return fixedDiagnosticRekeyRepositoryError(err)
		}
		result.Scanned += uint64(len(rows))
		result.Rekeyed += uint64(len(rows))
		afterID = lastID
		if s.observer != nil {
			for _, row := range rows {
				if err := s.observer(row.ID); err != nil {
					return ErrDiagnosticRekeyOutputFailed
				}
			}
		}
		if len(rows) < DefaultDiagnosticRekeyBatchSize {
			return nil
		}
	}
}

func isCanonicalDiagnosticCiphertext(ciphertext string) bool {
	decoded, err := base64.StdEncoding.Strict().DecodeString(ciphertext)
	return err == nil && base64.StdEncoding.EncodeToString(decoded) == ciphertext
}

// IsCanonicalDiagnosticKeyID accepts only IDs produced by secretkey's 16-byte
// digest encoding, including strict rejection of non-zero base64 padding bits.
func IsCanonicalDiagnosticKeyID(keyID string) bool {
	suffix, ok := strings.CutPrefix(keyID, diagnosticKeyIDPrefix)
	if !ok || len(suffix) != 22 {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(suffix)
	return err == nil && len(decoded) == 16 && base64.RawURLEncoding.EncodeToString(decoded) == suffix
}

func fixedDiagnosticRekeyRepositoryError(err error) error {
	if errors.Is(err, ErrDiagnosticRekeyRepositoryNotConfigured) {
		return ErrDiagnosticRekeyRepositoryNotConfigured
	}
	if errors.Is(err, ErrDiagnosticRekeyLockUnavailable) {
		return ErrDiagnosticRekeyLockUnavailable
	}
	if errors.Is(err, ErrDiagnosticRekeyOptimisticCompareFailed) {
		return ErrDiagnosticRekeyOptimisticCompareFailed
	}
	return ErrDiagnosticRekeyRepositoryFailure
}

func fixedDiagnosticRekeyError(err error) error {
	for _, sentinel := range []error{
		ErrDiagnosticRekeyRepositoryNotConfigured,
		ErrDiagnosticRekeyRepositoryFailure,
		ErrDiagnosticRekeyLockUnavailable,
		ErrDiagnosticRekeyUnknownKey,
		ErrDiagnosticRekeyCorruptCipher,
		ErrDiagnosticRekeyOptimisticCompareFailed,
		ErrDiagnosticRekeyOutputFailed,
		ErrDiagnosticRekeyIncomplete,
	} {
		if errors.Is(err, sentinel) {
			return sentinel
		}
	}
	return ErrDiagnosticRekeyRepositoryFailure
}
