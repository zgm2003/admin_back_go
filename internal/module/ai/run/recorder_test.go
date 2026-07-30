package airun

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRecorderStartsImageRunWithoutPolymorphicSourceFields(t *testing.T) {
	repo := &fakeRecorderRepository{nextID: 9}
	svc := NewRecorder(repo, func() time.Time { return time.Date(2026, 6, 7, 1, 2, 3, 0, time.UTC) })
	id, err := svc.Start(context.Background(), StartInput{Platform: enum.PlatformAdmin, RequestID: "image-77", UserID: 5, AgentID: 8, ProviderID: 9, ModelID: "gpt-image-1", ModelDisplayName: "GPT Image", InputSnapshot: "cat"})
	if err != nil || id != 9 {
		t.Fatalf("start failed id=%d err=%v", id, err)
	}
	if repo.started.Platform != enum.PlatformAdmin || repo.started.RequestID != "image-77" {
		t.Fatalf("bad start record: %#v", repo.started)
	}
}

func TestRecorderPreservesExplicitIdempotencyKey(t *testing.T) {
	repo := &fakeRecorderRepository{nextID: 10}
	svc := NewRecorder(repo, time.Now)
	_, err := svc.Start(context.Background(), StartInput{Platform: enum.PlatformAdmin, RequestID: "reply-1", IdempotencyKey: "reply-command:41", UserID: 5, AgentID: 8, ProviderID: 9, ModelID: "gpt-5.4", InputSnapshot: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if repo.started.IdempotencyKey != "reply-command:41" {
		t.Fatalf("start record=%+v", repo.started)
	}
}

func TestRecorderRunUsesReplayableRequestIdentity(t *testing.T) {
	run := runFromStartRecord(StartRecord{RequestID: "request-1"}, time.Now(), nil)
	if run.RequestIdentityStatus != "replayable" || run.RequestIdentityMarker != "" {
		t.Fatalf("request identity status=%q marker=%q", run.RequestIdentityStatus, run.RequestIdentityMarker)
	}
}

func TestRecorderRejectsMissingRequestID(t *testing.T) {
	svc := NewRecorder(&fakeRecorderRepository{}, time.Now)
	_, err := svc.Start(context.Background(), StartInput{Platform: enum.PlatformAdmin, UserID: 1, AgentID: 1, ProviderID: 1, ModelID: "m", InputSnapshot: "cat"})
	if err == nil {
		t.Fatalf("expected missing request id error")
	}
}

func TestRecorderRequestIDContractIs128Characters(t *testing.T) {
	repo := &fakeRecorderRepository{}
	svc := NewRecorder(repo, time.Now)
	base := StartInput{Platform: enum.PlatformAdmin, UserID: 1, AgentID: 1, ProviderID: 1, ModelID: "m", InputSnapshot: "input"}
	base.RequestID = strings.Repeat("界", 128)
	if _, err := svc.Start(context.Background(), base); err != nil {
		t.Fatalf("128-character request_id rejected: %v", err)
	}
	base.RequestID = strings.Repeat("界", 129)
	if _, err := svc.Start(context.Background(), base); !errors.Is(err, ErrRecorderInvalidInput) {
		t.Fatalf("129-character request_id error=%v", err)
	}
}

func TestRecorderCompleteStoresTokenCountsOnly(t *testing.T) {
	repo := &fakeRecorderRepository{}
	svc := NewRecorder(repo, func() time.Time { return time.Date(2026, 6, 7, 2, 0, 0, 0, time.UTC) })
	err := svc.Complete(context.Background(), CompleteInput{RunID: 9, PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7, DurationMS: 1200})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if repo.completed.TotalTokens != 7 {
		t.Fatalf("bad complete record: %#v", repo.completed)
	}
}

func TestRecorderTerminalUpdateIsIdempotent(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `ai_runs` SET .* WHERE id = \\? AND status = \\?").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(seq\\), 0\\) FROM `ai_run_events` WHERE run_id = \\?").WillReturnRows(sqlmock.NewRows([]string{"COALESCE(MAX(seq), 0)"}).AddRow(1))
	mock.ExpectExec("INSERT INTO `ai_run_events` .* VALUES .*").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectQuery("(?is)SELECT EXISTS .* FROM ai_run_dashboard_facts").WillReturnRows(sqlmock.NewRows([]string{"fact_exists"}).AddRow(false))
	mock.ExpectExec("(?is)INSERT INTO ai_run_dashboard_facts .* WHERE r.id = \\?").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("(?is)INSERT INTO ai_run_dashboard_daily_facts").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `ai_runs` SET .* WHERE id = \\? AND status = \\?").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("(?is)SELECT EXISTS .* FROM ai_run_dashboard_facts").WillReturnRows(sqlmock.NewRows([]string{"fact_exists"}).AddRow(true))
	mock.ExpectCommit()

	recorder := NewRecorder(&GormRepository{db: db}, func() time.Time { return time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC) })
	input := CompleteInput{RunID: 9, PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7, DurationMS: 1200}
	if err := recorder.Complete(context.Background(), input); err != nil {
		t.Fatalf("first completion failed: %v", err)
	}
	if err := recorder.Complete(context.Background(), input); err != nil {
		t.Fatalf("repeated completion must be idempotent: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("terminal update was not idempotent: %v", err)
	}
}

func TestStartRunRejectsTerminalIdempotencyReplay(t *testing.T) {
	tests := []string{
		enum.AIRunStatusFailed,
		enum.AIRunStatusCanceled,
		enum.AIRunStatusTimeout,
		enum.AIRunStatusOutcomeUnknown,
	}
	for _, status := range tests {
		t.Run(status, func(t *testing.T) {
			db, mock, _ := newDashboardRepositoryTestDB(t)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT \\* FROM `ai_runs` WHERE idempotency_key = \\? ORDER BY `ai_runs`.`id` LIMIT \\? FOR UPDATE").
				WithArgs("reply-command:41", 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(41, status))
			mock.ExpectRollback()

			id, err := (&GormRepository{db: db}).StartRun(context.Background(), StartRecord{
				IdempotencyKey: "reply-command:41",
				StartedAt:      time.Date(2026, 7, 30, 9, 0, 0, 0, time.Local),
			})
			if id != 0 || !errors.Is(err, ErrTerminalRunCannotRestart) {
				t.Fatalf("id=%d error=%v", id, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("terminal Run must remain immutable: %v", err)
			}
		})
	}
}

type fakeRecorderRepository struct {
	nextID    int64
	started   StartRecord
	completed CompleteRecord
	finished  FinishRecord
	startErr  error
}

func (f *fakeRecorderRepository) StartRun(ctx context.Context, input StartRecord) (int64, error) {
	f.started = input
	if f.startErr != nil {
		return 0, f.startErr
	}
	if f.nextID == 0 {
		return 1, nil
	}
	return f.nextID, nil
}

func (f *fakeRecorderRepository) CompleteRun(ctx context.Context, input CompleteRecord) error {
	f.completed = input
	return nil
}

func (f *fakeRecorderRepository) FinishRun(ctx context.Context, input FinishRecord) error {
	f.finished = input
	return nil
}

func TestRecorderReturnsRepositoryStartError(t *testing.T) {
	wantErr := errors.New("insert failed")
	svc := NewRecorder(&fakeRecorderRepository{startErr: wantErr}, time.Now)
	_, err := svc.Start(context.Background(), StartInput{Platform: enum.PlatformAdmin, RequestID: "image-7", UserID: 1, AgentID: 1, ProviderID: 1, ModelID: "m", InputSnapshot: "cat"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected repository error, got %v", err)
	}
}
