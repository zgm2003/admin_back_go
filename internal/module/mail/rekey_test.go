package mail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/secretbox"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDiagnosticRekeyCurrentOnlyIsANoOpWithZeroReferences(t *testing.T) {
	box, currentID, _ := newDiagnosticRekeyTestBox(t)
	currentCipher := encryptDiagnosticCode(t, box, currentID, "123456")
	locked := &memoryDiagnosticRekeyRepository{rows: []DiagnosticCipherRow{{ID: 8, KeyID: currentID, CodeEnc: currentCipher}}}
	owner := &diagnosticRekeyLockOwner{locked: locked}
	observerCalls := 0
	service := NewDiagnosticRekeyService(owner, box, "", func(uint64) error {
		observerCalls++
		return nil
	})

	result, err := service.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned fixed-error candidate: %v", err)
	}
	if result.CurrentKeyID != currentID || result.PreviousKeyID != "" || result.Scanned != 0 || result.Rekeyed != 0 || result.PreviousReferences != 0 || result.UnknownReferences != 0 {
		t.Fatalf("unexpected current-only result metadata")
	}
	if owner.name != DiagnosticRekeyLockName || locked.listCalls != 0 || len(locked.rewriteCalls) != 0 || observerCalls != 0 {
		t.Fatalf("current-only execution performed a previous-key operation")
	}
	if locked.unknownAllowedCalls != 1 || len(locked.lastUnknownAllowed) != 1 || locked.lastUnknownAllowed[0] != currentID || locked.previousCountCalls != 0 {
		t.Fatalf("current-only verification did not use the exact allowed key set")
	}
	if owner.unlockedCalls != 0 {
		t.Fatalf("service used the outer repository instead of the lock callback repository")
	}
}

func TestDiagnosticRekeyRejectsMalformedConfiguredKeyIDsBeforeLock(t *testing.T) {
	const malformedID = "mail-diagnostic-v1-short"
	const nonCanonicalID = "mail-diagnostic-v1-AAAAAAAAAAAAAAAAAAAAAB"
	const validID = "mail-diagnostic-v1-AAAAAAAAAAAAAAAAAAAAAA"
	tests := []struct {
		name       string
		currentID  string
		previousID string
		want       error
	}{
		{name: "current", currentID: malformedID, want: ErrDiagnosticRekeyCorruptCipher},
		{name: "previous", currentID: validID, previousID: malformedID, want: ErrDiagnosticRekeyUnknownKey},
		{name: "noncanonical current", currentID: nonCanonicalID, want: ErrDiagnosticRekeyCorruptCipher},
		{name: "noncanonical previous", currentID: validID, previousID: nonCanonicalID, want: ErrDiagnosticRekeyUnknownKey},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			keys := map[string][]byte{test.currentID: []byte("cccccccccccccccccccccccccccccccc")}
			if test.previousID != "" {
				keys[test.previousID] = []byte("pppppppppppppppppppppppppppppppp")
			}
			box, err := secretbox.NewVersioned(test.currentID, keys)
			if err != nil {
				t.Fatal("create malformed-ID diagnostic box")
			}
			locked := &memoryDiagnosticRekeyRepository{}
			owner := &diagnosticRekeyLockOwner{locked: locked}
			_, err = NewDiagnosticRekeyService(owner, box, test.previousID, nil).Run(context.Background())
			assertFixedDiagnosticRekeyError(t, err, test.want, malformedID)
			if owner.name != "" || locked.listCalls != 0 || len(locked.rewriteCalls) != 0 || locked.countCalls() != 0 {
				t.Fatalf("malformed configured key ID reached the repository")
			}
		})
	}
}

func TestIsCanonicalDiagnosticKeyIDRequiresStrictRawURL16ByteEncoding(t *testing.T) {
	tests := map[string]bool{
		"mail-diagnostic-v1-AAAAAAAAAAAAAAAAAAAAAA":  true,
		"mail-diagnostic-v1-AAAAAAAAAAAAAAAAAAAAAB":  false,
		"mail-diagnostic-v1-AAAAAAAAAAAAAAAAAAAAA":   false,
		"mail-diagnostic-v1-AAAAAAAAAAAAAAAAAAAAAA=": false,
	}
	for keyID, want := range tests {
		if got := IsCanonicalDiagnosticKeyID(keyID); got != want {
			t.Fatalf("IsCanonicalDiagnosticKeyID(%q) = %v, want %v", keyID, got, want)
		}
	}
}

func TestDiagnosticRekeyProcessesPreviousRowsAscendingInBatchesOfAtMostOneHundred(t *testing.T) {
	box, currentID, previousID := newDiagnosticRekeyTestBox(t)
	rows := make([]DiagnosticCipherRow, 0, 205)
	for id := uint64(205); id > 0; id-- {
		rows = append(rows, DiagnosticCipherRow{ID: id, KeyID: previousID, CodeEnc: encryptDiagnosticCode(t, box, previousID, fmt.Sprintf("%06d", id))})
	}
	locked := &memoryDiagnosticRekeyRepository{rows: rows}
	owner := &diagnosticRekeyLockOwner{locked: locked}
	observed := make([]uint64, 0, len(rows))
	service := NewDiagnosticRekeyService(owner, box, previousID, func(id uint64) error {
		observed = append(observed, id)
		return nil
	})

	result, err := service.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned fixed-error candidate: %v", err)
	}
	if result.CurrentKeyID != currentID || result.PreviousKeyID != previousID || result.Scanned != 205 || result.Rekeyed != 205 || result.PreviousReferences != 0 || result.UnknownReferences != 0 {
		t.Fatalf("unexpected rekey result metadata")
	}
	if len(locked.listLimits) != 3 || len(locked.listAfterIDs) != 3 {
		t.Fatalf("unexpected batch scan count")
	}
	for _, limit := range locked.listLimits {
		if limit <= 0 || limit > DefaultDiagnosticRekeyBatchSize {
			t.Fatalf("batch limit exceeded the fixed bound")
		}
	}
	if locked.listAfterIDs[0] != 0 || locked.listAfterIDs[1] != 100 || locked.listAfterIDs[2] != 200 {
		t.Fatalf("batch cursor did not advance by committed row ID")
	}
	if len(observed) != 205 || len(locked.rewriteCalls) != 3 {
		t.Fatalf("committed rows were not observed exactly once")
	}
	for index, id := range observed {
		if id != uint64(index+1) {
			t.Fatalf("observer row IDs were not ascending")
		}
	}
	for _, row := range locked.rows {
		if row.KeyID != currentID || row.CodeEnc == "" {
			t.Fatalf("a committed row did not use the current diagnostic key")
		}
		plain, decryptErr := box.Decrypt(currentID, row.CodeEnc)
		if decryptErr != nil || len(plain) != 6 {
			t.Fatalf("a committed row did not contain a valid diagnostic code")
		}
	}
}

func TestDiagnosticRekeyRejectsUnknownOrEmptyKeyBeforeMutation(t *testing.T) {
	box, _, previousID := newDiagnosticRekeyTestBox(t)
	for _, unknownID := range []string{"", "marker-unknown-key"} {
		t.Run(fmt.Sprintf("key-length-%d", len(unknownID)), func(t *testing.T) {
			locked := &memoryDiagnosticRekeyRepository{distinctOverride: []string{unknownID}}
			owner := &diagnosticRekeyLockOwner{locked: locked}
			service := NewDiagnosticRekeyService(owner, box, previousID, nil)

			_, err := service.Run(context.Background())
			assertFixedDiagnosticRekeyError(t, err, ErrDiagnosticRekeyUnknownKey, "marker-unknown-key")
			if locked.listCalls != 0 || len(locked.rewriteCalls) != 0 || locked.countCalls() != 0 {
				t.Fatalf("unknown-key preflight performed scan, mutation, or final count")
			}
		})
	}
}

func TestDiagnosticRekeyCorruptCipherLeavesWholeBatchUnmodified(t *testing.T) {
	box, _, previousID := newDiagnosticRekeyTestBox(t)
	locked := &memoryDiagnosticRekeyRepository{rows: []DiagnosticCipherRow{
		{ID: 1, KeyID: previousID, CodeEnc: encryptDiagnosticCode(t, box, previousID, "123456")},
		{ID: 2, KeyID: previousID, CodeEnc: "marker-corrupt-cipher"},
	}}
	owner := &diagnosticRekeyLockOwner{locked: locked}
	service := NewDiagnosticRekeyService(owner, box, previousID, nil)

	result, err := service.Run(context.Background())
	assertFixedDiagnosticRekeyError(t, err, ErrDiagnosticRekeyCorruptCipher, "marker-corrupt-cipher", "123456")
	if result.Rekeyed != 0 || len(locked.rewriteCalls) != 0 {
		t.Fatalf("corrupt row allowed a partial batch mutation")
	}
	if locked.rows[0].KeyID != previousID || locked.rows[1].KeyID != previousID {
		t.Fatalf("corrupt batch changed in-memory rows")
	}
}

func TestDiagnosticRekeyRejectsInvalidRowShapeWithoutMutation(t *testing.T) {
	box, _, previousID := newDiagnosticRekeyTestBox(t)
	invalidPlain := encryptDiagnosticCode(t, box, previousID, "12345x")
	tests := []struct {
		name string
		rows []DiagnosticCipherRow
	}{
		{name: "zero id", rows: []DiagnosticCipherRow{{ID: 0, KeyID: previousID, CodeEnc: invalidPlain}}},
		{name: "wrong key", rows: []DiagnosticCipherRow{{ID: 1, KeyID: "marker-wrong-key", CodeEnc: invalidPlain}}},
		{name: "empty cipher", rows: []DiagnosticCipherRow{{ID: 1, KeyID: previousID, CodeEnc: ""}}},
		{name: "invalid plain", rows: []DiagnosticCipherRow{{ID: 1, KeyID: previousID, CodeEnc: invalidPlain}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			locked := &memoryDiagnosticRekeyRepository{rows: test.rows, distinctOverride: []string{previousID}, listOverride: test.rows}
			owner := &diagnosticRekeyLockOwner{locked: locked}
			_, err := NewDiagnosticRekeyService(owner, box, previousID, nil).Run(context.Background())
			assertFixedDiagnosticRekeyError(t, err, ErrDiagnosticRekeyCorruptCipher, "marker-wrong-key", "12345x")
			if len(locked.rewriteCalls) != 0 {
				t.Fatalf("invalid row shape caused a batch mutation")
			}
		})
	}
}

func TestDiagnosticRekeyObserverFailureHappensAfterCommitAndRerunResumes(t *testing.T) {
	box, currentID, previousID := newDiagnosticRekeyTestBox(t)
	locked := &memoryDiagnosticRekeyRepository{rows: []DiagnosticCipherRow{
		{ID: 3, KeyID: previousID, CodeEnc: encryptDiagnosticCode(t, box, previousID, "123456")},
		{ID: 4, KeyID: previousID, CodeEnc: encryptDiagnosticCode(t, box, previousID, "654321")},
	}}
	owner := &diagnosticRekeyLockOwner{locked: locked}
	observerCalls := 0
	service := NewDiagnosticRekeyService(owner, box, previousID, func(uint64) error {
		observerCalls++
		if observerCalls == 1 {
			return errors.New("marker-observer-provider-output")
		}
		return nil
	})

	result, err := service.Run(context.Background())
	assertFixedDiagnosticRekeyError(t, err, ErrDiagnosticRekeyOutputFailed, "marker-observer-provider-output", "123456", "654321")
	if result.Rekeyed != 2 || observerCalls != 1 || len(locked.rewriteCalls) != 1 {
		t.Fatalf("observer failure occurred before the batch commit was accounted")
	}
	for _, row := range locked.rows {
		if row.KeyID != currentID {
			t.Fatalf("observer failure rolled back a committed row")
		}
	}

	resumeOwner := &diagnosticRekeyLockOwner{locked: locked}
	resumedObserverCalls := 0
	resumed, err := NewDiagnosticRekeyService(resumeOwner, box, previousID, func(uint64) error {
		resumedObserverCalls++
		return nil
	}).Run(context.Background())
	if err != nil {
		t.Fatalf("resumed Run returned fixed-error candidate: %v", err)
	}
	if resumed.Scanned != 0 || resumed.Rekeyed != 0 || resumed.PreviousReferences != 0 || resumed.UnknownReferences != 0 || resumedObserverCalls != 0 {
		t.Fatalf("rerun did not resume from committed database state")
	}
}

func TestDiagnosticRekeyRerunIsIdempotent(t *testing.T) {
	box, _, previousID := newDiagnosticRekeyTestBox(t)
	locked := &memoryDiagnosticRekeyRepository{rows: []DiagnosticCipherRow{{ID: 11, KeyID: previousID, CodeEnc: encryptDiagnosticCode(t, box, previousID, "123456")}}}
	first, err := NewDiagnosticRekeyService(&diagnosticRekeyLockOwner{locked: locked}, box, previousID, nil).Run(context.Background())
	if err != nil || first.Rekeyed != 1 {
		t.Fatalf("first Run did not commit exactly one row")
	}
	beforeCalls := len(locked.rewriteCalls)
	second, err := NewDiagnosticRekeyService(&diagnosticRekeyLockOwner{locked: locked}, box, previousID, nil).Run(context.Background())
	if err != nil {
		t.Fatalf("idempotent Run returned fixed-error candidate: %v", err)
	}
	if second.Scanned != 0 || second.Rekeyed != 0 || second.PreviousReferences != 0 || second.UnknownReferences != 0 || len(locked.rewriteCalls) != beforeCalls {
		t.Fatalf("idempotent Run performed another mutation")
	}
}

func TestDiagnosticRekeyPropagatesFixedLockAndCompareFailures(t *testing.T) {
	box, _, previousID := newDiagnosticRekeyTestBox(t)
	t.Run("lock contention", func(t *testing.T) {
		owner := &diagnosticRekeyLockOwner{locked: &memoryDiagnosticRekeyRepository{}, lockErr: ErrDiagnosticRekeyLockUnavailable}
		_, err := NewDiagnosticRekeyService(owner, box, previousID, nil).Run(context.Background())
		assertFixedDiagnosticRekeyError(t, err, ErrDiagnosticRekeyLockUnavailable)
	})
	t.Run("optimistic compare", func(t *testing.T) {
		locked := &memoryDiagnosticRekeyRepository{
			rows:       []DiagnosticCipherRow{{ID: 1, KeyID: previousID, CodeEnc: encryptDiagnosticCode(t, box, previousID, "123456")}},
			rewriteErr: ErrDiagnosticRekeyOptimisticCompareFailed,
		}
		_, err := NewDiagnosticRekeyService(&diagnosticRekeyLockOwner{locked: locked}, box, previousID, nil).Run(context.Background())
		assertFixedDiagnosticRekeyError(t, err, ErrDiagnosticRekeyOptimisticCompareFailed)
		if locked.rows[0].KeyID != previousID {
			t.Fatalf("compare failure changed a row")
		}
	})
}

func TestDiagnosticRekeyFailsFixedWhenFinalReferencesRemain(t *testing.T) {
	box, _, previousID := newDiagnosticRekeyTestBox(t)
	for _, test := range []struct {
		name           string
		previousRemain *int64
		unknownRemain  *int64
	}{
		{name: "previous", previousRemain: int64Pointer(1)},
		{name: "unknown", unknownRemain: int64Pointer(1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			locked := &memoryDiagnosticRekeyRepository{previousCountOverride: test.previousRemain, unknownCountOverride: test.unknownRemain}
			result, err := NewDiagnosticRekeyService(&diagnosticRekeyLockOwner{locked: locked}, box, previousID, nil).Run(context.Background())
			assertFixedDiagnosticRekeyError(t, err, ErrDiagnosticRekeyIncomplete)
			if result.PreviousReferences == 0 && result.UnknownReferences == 0 {
				t.Fatalf("failed verification reported zero references")
			}
		})
	}
}

func TestDiagnosticRekeyRepositoryUsesPinnedConnectionForLockAndChildScan(t *testing.T) {
	repository, mock, sqlDB, closeDB := newDiagnosticRekeySQLMock(t)
	defer closeDB()
	sqlDB.SetMaxOpenConns(1)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT GET_LOCK(?, ?)")).WithArgs(DiagnosticRekeyLockName, 0).WillReturnRows(sqlmock.NewRows([]string{"lock_result"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT DISTINCT BINARY key_id AS key_id FROM mail_log_verification_codes ORDER BY BINARY key_id ASC")).WillReturnRows(sqlmock.NewRows([]string{"key_id"}).AddRow("current"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT RELEASE_LOCK(?)")).WithArgs(DiagnosticRekeyLockName).WillReturnRows(sqlmock.NewRows([]string{"release_result"}).AddRow(1))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var callbackRepository DiagnosticRekeyRepository
	err := repository.WithDiagnosticRekeyLock(ctx, DiagnosticRekeyLockName, func(locked DiagnosticRekeyRepository) error {
		callbackRepository = locked
		ids, listErr := locked.DistinctDiagnosticKeyIDs(ctx)
		if listErr != nil || len(ids) != 1 || ids[0] != "current" {
			t.Fatalf("locked child scan failed")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithDiagnosticRekeyLock returned fixed-error candidate: %v", err)
	}
	if callbackRepository == nil || callbackRepository == repository {
		t.Fatalf("lock callback did not receive a connection-bound repository")
	}
	assertDiagnosticRekeySQLExpectations(t, mock)
}

func TestDiagnosticRekeyRepositoryLockContentionSkipsCallback(t *testing.T) {
	repository, mock, _, closeDB := newDiagnosticRekeySQLMock(t)
	defer closeDB()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT GET_LOCK(?, ?)")).WithArgs(DiagnosticRekeyLockName, 0).WillReturnRows(sqlmock.NewRows([]string{"lock_result"}).AddRow(0))
	called := false
	err := repository.WithDiagnosticRekeyLock(context.Background(), DiagnosticRekeyLockName, func(DiagnosticRekeyRepository) error {
		called = true
		return nil
	})
	assertFixedDiagnosticRekeyError(t, err, ErrDiagnosticRekeyLockUnavailable)
	if called {
		t.Fatalf("lock contention invoked callback")
	}
	assertDiagnosticRekeySQLExpectations(t, mock)
}

func TestDiagnosticRekeyRepositoryTreatsNullLockResultAsRepositoryFailure(t *testing.T) {
	repository, mock, _, closeDB := newDiagnosticRekeySQLMock(t)
	defer closeDB()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT GET_LOCK(?, ?)")).WithArgs(DiagnosticRekeyLockName, 0).WillReturnRows(sqlmock.NewRows([]string{"lock_result"}).AddRow(nil))
	called := false
	err := repository.WithDiagnosticRekeyLock(context.Background(), DiagnosticRekeyLockName, func(DiagnosticRekeyRepository) error {
		called = true
		return nil
	})
	assertFixedDiagnosticRekeyError(t, err, ErrDiagnosticRekeyRepositoryFailure)
	if called {
		t.Fatalf("NULL lock result invoked callback")
	}
	assertDiagnosticRekeySQLExpectations(t, mock)
}

func TestDiagnosticRekeyRepositoryReleasesLockAfterCallbackPanic(t *testing.T) {
	repository, mock, _, closeDB := newDiagnosticRekeySQLMock(t)
	defer closeDB()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT GET_LOCK(?, ?)")).WithArgs(DiagnosticRekeyLockName, 0).WillReturnRows(sqlmock.NewRows([]string{"lock_result"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT RELEASE_LOCK(?)")).WithArgs(DiagnosticRekeyLockName).WillReturnRows(sqlmock.NewRows([]string{"release_result"}).AddRow(1))
	const panicMarker = "marker-diagnostic-rekey-callback-panic"
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = repository.WithDiagnosticRekeyLock(context.Background(), DiagnosticRekeyLockName, func(DiagnosticRekeyRepository) error {
			panic(panicMarker)
		})
	}()
	if recovered != panicMarker {
		t.Fatalf("callback panic was swallowed or changed")
	}
	assertDiagnosticRekeySQLExpectations(t, mock)
}

func TestDiagnosticRekeyRepositoryReleaseFailureIsNotSwallowed(t *testing.T) {
	repository, mock, _, closeDB := newDiagnosticRekeySQLMock(t)
	defer closeDB()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT GET_LOCK(?, ?)")).WithArgs(DiagnosticRekeyLockName, 0).WillReturnRows(sqlmock.NewRows([]string{"lock_result"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT RELEASE_LOCK(?)")).WithArgs(DiagnosticRekeyLockName).WillReturnError(errors.New("marker-release-provider-error"))
	mock.ExpectClose()
	err := repository.WithDiagnosticRekeyLock(context.Background(), DiagnosticRekeyLockName, func(DiagnosticRekeyRepository) error {
		return ErrDiagnosticRekeyUnknownKey
	})
	assertFixedDiagnosticRekeyError(t, err, ErrDiagnosticRekeyRepositoryFailure, "marker-release-provider-error")
	assertDiagnosticRekeySQLExpectations(t, mock)
}

func TestDiagnosticRekeyRepositoryRedactsUnknownCallbackFailureAfterRelease(t *testing.T) {
	repository, mock, _, closeDB := newDiagnosticRekeySQLMock(t)
	defer closeDB()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT GET_LOCK(?, ?)")).WithArgs(DiagnosticRekeyLockName, 0).WillReturnRows(sqlmock.NewRows([]string{"lock_result"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT RELEASE_LOCK(?)")).WithArgs(DiagnosticRekeyLockName).WillReturnRows(sqlmock.NewRows([]string{"release_result"}).AddRow(1))
	err := repository.WithDiagnosticRekeyLock(context.Background(), DiagnosticRekeyLockName, func(DiagnosticRekeyRepository) error {
		return errors.New("marker-callback-provider-secret")
	})
	assertFixedDiagnosticRekeyError(t, err, ErrDiagnosticRekeyRepositoryFailure, "marker-callback-provider-secret")
	assertDiagnosticRekeySQLExpectations(t, mock)
}

func TestDiagnosticRekeyRepositoryScansChildTableWithoutParentFilter(t *testing.T) {
	repository, mock, _, closeDB := newDiagnosticRekeySQLMock(t)
	defer closeDB()
	query := "SELECT id,key_id,code_enc FROM mail_log_verification_codes WHERE key_id=BINARY ? AND id>? ORDER BY key_id ASC,id ASC LIMIT ?"
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs("previous", uint64(40), 100).WillReturnRows(
		sqlmock.NewRows([]string{"id", "key_id", "code_enc"}).AddRow(41, "previous", "cipher-for-soft-deleted-parent"),
	)
	rows, err := repository.ListDiagnosticCipherRows(context.Background(), "previous", 40, 100)
	if err != nil || len(rows) != 1 || rows[0].ID != 41 || rows[0].KeyID != "previous" {
		t.Fatalf("child-only scan did not return the expected row")
	}
	assertDiagnosticRekeySQLExpectations(t, mock)
}

func TestDiagnosticRekeyRepositoryCommitsOptimisticBatch(t *testing.T) {
	repository, mock, _, closeDB := newDiagnosticRekeySQLMock(t)
	defer closeDB()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE mail_log_verification_codes SET key_id=?, code_enc=? WHERE id=? AND BINARY key_id=BINARY ? AND BINARY code_enc=BINARY ?")).
		WithArgs("current", "new-one", uint64(1), "previous", "old-one").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE mail_log_verification_codes SET key_id=?, code_enc=? WHERE id=? AND BINARY key_id=BINARY ? AND BINARY code_enc=BINARY ?")).
		WithArgs("current", "new-two", uint64(2), "previous", "old-two").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	err := repository.RewriteDiagnosticCipherBatch(context.Background(), []DiagnosticCipherRewrite{
		{ID: 1, OldKeyID: "previous", OldCodeEnc: "old-one", NewKeyID: "current", NewCodeEnc: "new-one"},
		{ID: 2, OldKeyID: "previous", OldCodeEnc: "old-two", NewKeyID: "current", NewCodeEnc: "new-two"},
	})
	if err != nil {
		t.Fatalf("RewriteDiagnosticCipherBatch returned fixed-error candidate: %v", err)
	}
	assertDiagnosticRekeySQLExpectations(t, mock)
}

func TestDiagnosticRekeyRepositoryRollsBackWholeBatchOnCompareMismatch(t *testing.T) {
	repository, mock, _, closeDB := newDiagnosticRekeySQLMock(t)
	defer closeDB()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE mail_log_verification_codes SET key_id=?, code_enc=? WHERE id=? AND BINARY key_id=BINARY ? AND BINARY code_enc=BINARY ?")).
		WithArgs("current", "new-one", uint64(1), "previous", "old-one").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE mail_log_verification_codes SET key_id=?, code_enc=? WHERE id=? AND BINARY key_id=BINARY ? AND BINARY code_enc=BINARY ?")).
		WithArgs("current", "new-two", uint64(2), "previous", "old-two").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	err := repository.RewriteDiagnosticCipherBatch(context.Background(), []DiagnosticCipherRewrite{
		{ID: 1, OldKeyID: "previous", OldCodeEnc: "old-one", NewKeyID: "current", NewCodeEnc: "new-one"},
		{ID: 2, OldKeyID: "previous", OldCodeEnc: "old-two", NewKeyID: "current", NewCodeEnc: "new-two"},
	})
	assertFixedDiagnosticRekeyError(t, err, ErrDiagnosticRekeyOptimisticCompareFailed, "old-one", "old-two", "new-one", "new-two")
	assertDiagnosticRekeySQLExpectations(t, mock)
}

func TestDiagnosticRekeyRepositoryCountsPreviousAndCaseVariantUnknownReferences(t *testing.T) {
	repository, mock, _, closeDB := newDiagnosticRekeySQLMock(t)
	defer closeDB()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM mail_log_verification_codes WHERE key_id=BINARY ?")).WithArgs("previous").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM mail_log_verification_codes WHERE BINARY key_id NOT IN (BINARY ?,BINARY ?)")).WithArgs("current", "previous").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	previous, err := repository.CountDiagnosticKeyID(context.Background(), "previous")
	if err != nil || previous != 0 {
		t.Fatalf("previous-key count was not zero")
	}
	unknown, err := repository.CountUnknownDiagnosticKeyIDs(context.Background(), []string{"current", "previous"})
	if err != nil || unknown != 1 {
		t.Fatalf("case-variant unknown key was not counted")
	}
	assertDiagnosticRekeySQLExpectations(t, mock)
}

func TestDiagnosticRekeyRepositoryIsNilSafeAndRedactsProviderErrors(t *testing.T) {
	var repository *GormDiagnosticRekeyRepository
	_, err := repository.DistinctDiagnosticKeyIDs(context.Background())
	assertFixedDiagnosticRekeyError(t, err, ErrDiagnosticRekeyRepositoryNotConfigured)

	repository, mock, _, closeDB := newDiagnosticRekeySQLMock(t)
	defer closeDB()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT DISTINCT BINARY key_id AS key_id FROM mail_log_verification_codes ORDER BY BINARY key_id ASC")).WillReturnError(errors.New("marker-database-provider-secret"))
	_, err = repository.DistinctDiagnosticKeyIDs(context.Background())
	assertFixedDiagnosticRekeyError(t, err, ErrDiagnosticRekeyRepositoryFailure, "marker-database-provider-secret")
	assertDiagnosticRekeySQLExpectations(t, mock)
}

func TestDiagnosticRekeyRepositorySuppressesGormSQLLogging(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal("create diagnostic rekey logger sql mock")
	}
	defer sqlDB.Close()
	probe := &diagnosticRekeyProbeLogger{}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               probe,
	})
	if err != nil {
		t.Fatal("open diagnostic rekey logger gorm mock")
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT DISTINCT BINARY key_id AS key_id FROM mail_log_verification_codes ORDER BY BINARY key_id ASC")).WillReturnError(errors.New("marker-provider-must-not-be-logged"))
	repository := NewGormDiagnosticRekeyRepository(&database.Client{Gorm: db, SQL: sqlDB})
	_, err = repository.DistinctDiagnosticKeyIDs(context.Background())
	assertFixedDiagnosticRekeyError(t, err, ErrDiagnosticRekeyRepositoryFailure, "marker-provider-must-not-be-logged")
	if probe.traces != 0 {
		t.Fatalf("diagnostic rekey repository allowed GORM SQL logging")
	}
	assertDiagnosticRekeySQLExpectations(t, mock)
}

type diagnosticRekeyLockOwner struct {
	locked        DiagnosticRekeyRepository
	lockErr       error
	name          string
	unlockedCalls int
}

func (r *diagnosticRekeyLockOwner) WithDiagnosticRekeyLock(_ context.Context, name string, callback func(DiagnosticRekeyRepository) error) error {
	r.name = name
	if r.lockErr != nil {
		return r.lockErr
	}
	return callback(r.locked)
}

func (r *diagnosticRekeyLockOwner) DistinctDiagnosticKeyIDs(context.Context) ([]string, error) {
	r.unlockedCalls++
	return nil, errors.New("outer repository used")
}

func (r *diagnosticRekeyLockOwner) ListDiagnosticCipherRows(context.Context, string, uint64, int) ([]DiagnosticCipherRow, error) {
	r.unlockedCalls++
	return nil, errors.New("outer repository used")
}

func (r *diagnosticRekeyLockOwner) RewriteDiagnosticCipherBatch(context.Context, []DiagnosticCipherRewrite) error {
	r.unlockedCalls++
	return errors.New("outer repository used")
}

func (r *diagnosticRekeyLockOwner) CountDiagnosticKeyID(context.Context, string) (int64, error) {
	r.unlockedCalls++
	return 0, errors.New("outer repository used")
}

func (r *diagnosticRekeyLockOwner) CountUnknownDiagnosticKeyIDs(context.Context, []string) (int64, error) {
	r.unlockedCalls++
	return 0, errors.New("outer repository used")
}

type memoryDiagnosticRekeyRepository struct {
	rows                  []DiagnosticCipherRow
	distinctOverride      []string
	listOverride          []DiagnosticCipherRow
	rewriteErr            error
	previousCountOverride *int64
	unknownCountOverride  *int64
	listCalls             int
	listLimits            []int
	listAfterIDs          []uint64
	rewriteCalls          [][]DiagnosticCipherRewrite
	previousCountCalls    int
	unknownAllowedCalls   int
	lastUnknownAllowed    []string
}

func (r *memoryDiagnosticRekeyRepository) WithDiagnosticRekeyLock(_ context.Context, _ string, callback func(DiagnosticRekeyRepository) error) error {
	return callback(r)
}

func (r *memoryDiagnosticRekeyRepository) DistinctDiagnosticKeyIDs(context.Context) ([]string, error) {
	if r.distinctOverride != nil {
		return append([]string(nil), r.distinctOverride...), nil
	}
	set := make(map[string]struct{})
	for _, row := range r.rows {
		set[row.KeyID] = struct{}{}
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func (r *memoryDiagnosticRekeyRepository) ListDiagnosticCipherRows(_ context.Context, keyID string, afterID uint64, limit int) ([]DiagnosticCipherRow, error) {
	r.listCalls++
	r.listLimits = append(r.listLimits, limit)
	r.listAfterIDs = append(r.listAfterIDs, afterID)
	if r.listOverride != nil {
		return append([]DiagnosticCipherRow(nil), r.listOverride...), nil
	}
	rows := make([]DiagnosticCipherRow, 0)
	for _, row := range r.rows {
		if row.KeyID == keyID && row.ID > afterID {
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return append([]DiagnosticCipherRow(nil), rows...), nil
}

func (r *memoryDiagnosticRekeyRepository) RewriteDiagnosticCipherBatch(_ context.Context, rewrites []DiagnosticCipherRewrite) error {
	cloned := append([]DiagnosticCipherRewrite(nil), rewrites...)
	r.rewriteCalls = append(r.rewriteCalls, cloned)
	if r.rewriteErr != nil {
		return r.rewriteErr
	}
	indexes := make([]int, len(rewrites))
	for rewriteIndex, rewrite := range rewrites {
		found := -1
		for rowIndex := range r.rows {
			row := r.rows[rowIndex]
			if row.ID == rewrite.ID && row.KeyID == rewrite.OldKeyID && row.CodeEnc == rewrite.OldCodeEnc {
				found = rowIndex
				break
			}
		}
		if found < 0 {
			return ErrDiagnosticRekeyOptimisticCompareFailed
		}
		indexes[rewriteIndex] = found
	}
	for index, rewrite := range rewrites {
		r.rows[indexes[index]].KeyID = rewrite.NewKeyID
		r.rows[indexes[index]].CodeEnc = rewrite.NewCodeEnc
	}
	return nil
}

func (r *memoryDiagnosticRekeyRepository) CountDiagnosticKeyID(_ context.Context, keyID string) (int64, error) {
	r.previousCountCalls++
	if r.previousCountOverride != nil {
		return *r.previousCountOverride, nil
	}
	var count int64
	for _, row := range r.rows {
		if row.KeyID == keyID {
			count++
		}
	}
	return count, nil
}

func (r *memoryDiagnosticRekeyRepository) CountUnknownDiagnosticKeyIDs(_ context.Context, allowed []string) (int64, error) {
	r.unknownAllowedCalls++
	r.lastUnknownAllowed = append([]string(nil), allowed...)
	if r.unknownCountOverride != nil {
		return *r.unknownCountOverride, nil
	}
	known := make(map[string]struct{}, len(allowed))
	for _, id := range allowed {
		known[id] = struct{}{}
	}
	var count int64
	for _, row := range r.rows {
		if _, ok := known[row.KeyID]; !ok {
			count++
		}
	}
	return count, nil
}

func (r *memoryDiagnosticRekeyRepository) countCalls() int {
	return r.previousCountCalls + r.unknownAllowedCalls
}

func newDiagnosticRekeyTestBox(t *testing.T) (secretbox.VersionedBox, string, string) {
	t.Helper()
	const currentID = "mail-diagnostic-v1-AAAAAAAAAAAAAAAAAAAAAA"
	const previousID = "mail-diagnostic-v1-BBBBBBBBBBBBBBBBBBBBBQ"
	box, err := secretbox.NewVersioned(currentID, map[string][]byte{
		currentID:  []byte("cccccccccccccccccccccccccccccccc"),
		previousID: []byte("pppppppppppppppppppppppppppppppp"),
	})
	if err != nil {
		t.Fatal("create diagnostic test box")
	}
	return box, currentID, previousID
}

func encryptDiagnosticCode(t *testing.T, box secretbox.VersionedBox, keyID string, code string) string {
	t.Helper()
	if keyID == box.CurrentKeyID() {
		gotID, ciphertext, err := box.Encrypt(code)
		if err != nil || gotID != keyID || ciphertext == "" {
			t.Fatal("encrypt diagnostic code with current key")
		}
		return ciphertext
	}
	key := []byte("pppppppppppppppppppppppppppppppp")
	ciphertext, err := secretbox.New(key).Encrypt(code)
	if err != nil || ciphertext == "" {
		t.Fatal("encrypt diagnostic code with previous key")
	}
	return ciphertext
}

func assertFixedDiagnosticRekeyError(t *testing.T, got error, want error, forbidden ...string) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("error did not match fixed diagnostic rekey sentinel")
	}
	if got.Error() != want.Error() {
		t.Fatalf("diagnostic rekey error was not fixed")
	}
	for _, marker := range forbidden {
		if marker != "" && strings.Contains(got.Error(), marker) {
			t.Fatalf("diagnostic rekey error exposed a forbidden marker")
		}
	}
}

func int64Pointer(value int64) *int64 { return &value }

func newDiagnosticRekeySQLMock(t *testing.T) (*GormDiagnosticRekeyRepository, sqlmock.Sqlmock, *sql.DB, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal("create diagnostic rekey sql mock")
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatal("open diagnostic rekey gorm mock")
	}
	client := &database.Client{Gorm: db, SQL: sqlDB}
	return NewGormDiagnosticRekeyRepository(client), mock, sqlDB, func() { _ = sqlDB.Close() }
}

func assertDiagnosticRekeySQLExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal("diagnostic rekey SQL expectations were not met")
	}
}

type diagnosticRekeyProbeLogger struct {
	traces int
}

func (l *diagnosticRekeyProbeLogger) LogMode(logger.LogLevel) logger.Interface { return l }
func (*diagnosticRekeyProbeLogger) Info(context.Context, string, ...any)       {}
func (*diagnosticRekeyProbeLogger) Warn(context.Context, string, ...any)       {}
func (*diagnosticRekeyProbeLogger) Error(context.Context, string, ...any)      {}
func (l *diagnosticRekeyProbeLogger) Trace(context.Context, time.Time, func() (string, int64), error) {
	l.traces++
}
