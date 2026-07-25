package runtime

import (
	"context"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/database"
	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/ai/replycommand"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fakeReplyAttemptRepository struct {
	prepared replycommand.PrepareAttemptInput
	marked   replycommand.Attempt
	finished replycommand.FinishAttemptInput
}

func (f *fakeReplyAttemptRepository) PrepareLegacyAttempt(_ context.Context, input replycommand.PrepareAttemptInput) (*replycommand.Attempt, bool, error) {
	f.prepared = input
	return &replycommand.Attempt{ID: 91, RunID: input.RunID, IdempotencyKey: "attempt-key"}, true, nil
}

func (f *fakeReplyAttemptRepository) MarkAttemptDispatchedForRun(_ context.Context, runID int64, attemptID uint64, commandID uint64, owner string, token uint64, now time.Time) (bool, error) {
	f.marked = replycommand.Attempt{ID: attemptID, CommandID: &commandID, ProviderRequestID: owner, AttemptNo: uint(token), UpdatedAt: now}
	f.marked.RunID = runID
	return true, nil
}

func (f *fakeReplyAttemptRepository) FinishAttempt(_ context.Context, input replycommand.FinishAttemptInput) (bool, error) {
	f.finished = input
	return true, nil
}

func TestReplyAttemptRecorderMapsLifecycleWithoutFieldFallback(t *testing.T) {
	repository := &fakeReplyAttemptRepository{}
	recorder := replyAttemptRecorder{repository: repository}
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	attempt, err := recorder.PrepareProviderAttempt(context.Background(), aichat.ProviderAttemptPrepareInput{RunID: 100, CommandID: 41, Owner: "worker-a", Token: 7, Now: now})
	if err != nil || attempt.ID != 91 || attempt.IdempotencyKey != "attempt-key" {
		t.Fatalf("attempt=%+v err=%v", attempt, err)
	}
	if err := recorder.MarkProviderAttemptDispatched(context.Background(), aichat.ProviderAttemptMarkInput{RunID: 100, AttemptID: 91, CommandID: 41, Owner: "worker-a", Token: 7, Now: now}); err != nil {
		t.Fatal(err)
	}
	if err := recorder.FinishProviderAttempt(context.Background(), aichat.ProviderAttemptFinishInput{RunID: 100, AttemptID: 91, CommandID: 41, Owner: "worker-a", Token: 7, State: aichat.ProviderAttemptOutcomeUnknown, ProviderRequestID: "provider-request-1", ResponseSHA256: "hash", ErrorCode: "ai.provider_outcome_unknown", DispatchState: "unknown", UsageJSON: `{"status":"unavailable"}`, UsageStatus: "unavailable", Now: now}); err != nil {
		t.Fatal(err)
	}
	if repository.prepared.RunID != 100 || repository.marked.RunID != 100 || repository.finished.RunID != 100 || repository.finished.State != replycommand.AttemptOutcomeUnknown || repository.finished.ProviderRequestID != "provider-request-1" || repository.finished.ResponseSHA256 != "hash" || repository.finished.UsageStatus != "unavailable" || repository.finished.DispatchState != "unknown" {
		t.Fatalf("prepared=%+v marked=%+v finished=%+v", repository.prepared, repository.marked, repository.finished)
	}
}

func TestReplyAttemptRecorderPassesMappedFailuresThroughStrictRepository(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	gormDB, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := replyAttemptRecorder{repository: replycommand.NewGormRepository(&database.Client{Gorm: gormDB, SQL: sqlDB})}
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		state         aichat.ProviderAttemptState
		dispatchState string
		errorCode     string
	}{
		{name: "rejected", state: aichat.ProviderAttemptFailed, dispatchState: infraai.DispatchStateDispatched, errorCode: "ai.provider_failed"},
		{name: "not dispatched", state: aichat.ProviderAttemptFailed, dispatchState: infraai.DispatchStateNotDispatched, errorCode: "ai.provider_failed"},
		{name: "local cancel before dispatch", state: aichat.ProviderAttemptCanceled, dispatchState: infraai.DispatchStateNotDispatched, errorCode: "ai.provider_canceled"},
		{name: "unknown", state: aichat.ProviderAttemptOutcomeUnknown, dispatchState: infraai.DispatchStateUnknown, errorCode: "ai.provider_outcome_unknown"},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock.ExpectBegin()
			mock.ExpectExec("UPDATE .*ai_provider_attempts.*").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			err := recorder.FinishProviderAttempt(context.Background(), aichat.ProviderAttemptFinishInput{
				RunID:         100,
				AttemptID:     uint64(91 + i),
				CommandID:     41,
				Owner:         "worker-a",
				Token:         7,
				State:         tc.state,
				ErrorCode:     tc.errorCode,
				DispatchState: tc.dispatchState,
				UsageJSON:     `{"status":"unavailable"}`,
				UsageStatus:   infraai.UsageStatusUnavailable,
				Now:           now,
			})
			if err != nil {
				t.Fatalf("finish mapped terminal evidence: %v", err)
			}
		})
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
