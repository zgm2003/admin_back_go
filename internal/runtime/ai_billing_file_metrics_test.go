package runtime

import (
	"context"
	"errors"
	"testing"

	"admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/replycommand"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestRecordTerminalOutcomePersistsFileMetricsAtomically(t *testing.T) {
	db, mock, closeDB := newGatewayMetricsMockDB(t)
	defer closeDB()

	metrics := &ai.FileInputMetrics{COSHeadMS: 17, COSStreamMS: 23, MaterializedRequestBytes: 4096}
	outcome := gatewayMetricsOutcome(metrics)
	mock.ExpectBegin()
	expectDispatchedAttemptForMetrics(mock)
	mock.ExpectExec("UPDATE `ai_provider_attempts` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(seq\\), 0\\) FROM `ai_run_events` WHERE run_id = \\?").
		WithArgs(int64(71)).WillReturnRows(sqlmock.NewRows([]string{"COALESCE(MAX(seq), 0)"}).AddRow(2))
	mock.ExpectExec("INSERT INTO `ai_run_events`").
		WithArgs(int64(71), uint(3), "file_materialized_v1", `{"cos_head_ms":17,"cos_stream_ms":23,"materialized_request_bytes":4096}`, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectCommit()

	err := (gormGatewayTransactions{db: db}).WithinTransaction(context.Background(), func(tx aigateway.Transaction) error {
		_, err := (gormGatewayAttemptStore{}).RecordTerminalOutcome(context.Background(), tx, 71, 1, outcome)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordTerminalOutcomeRollsBackWhenFileMetricsEventFails(t *testing.T) {
	db, mock, closeDB := newGatewayMetricsMockDB(t)
	defer closeDB()

	mock.ExpectBegin()
	expectDispatchedAttemptForMetrics(mock)
	mock.ExpectExec("UPDATE `ai_provider_attempts` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(seq\\), 0\\) FROM `ai_run_events` WHERE run_id = \\?").
		WithArgs(int64(71)).WillReturnRows(sqlmock.NewRows([]string{"COALESCE(MAX(seq), 0)"}).AddRow(2))
	mock.ExpectExec("INSERT INTO `ai_run_events`").WillReturnError(errors.New("event unavailable"))
	mock.ExpectRollback()

	err := (gormGatewayTransactions{db: db}).WithinTransaction(context.Background(), func(tx aigateway.Transaction) error {
		_, err := (gormGatewayAttemptStore{}).RecordTerminalOutcome(context.Background(), tx, 71, 1, gatewayMetricsOutcome(&ai.FileInputMetrics{MaterializedRequestBytes: 4096}))
		return err
	})
	if err == nil || err.Error() != "event unavailable" {
		t.Fatalf("err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func newGatewayMetricsMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatal(err)
	}
	return db, mock, func() { _ = sqlDB.Close() }
}

func expectDispatchedAttemptForMetrics(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT .* FROM `ai_provider_attempts` WHERE run_id = \\? AND attempt_no = \\? ORDER BY `ai_provider_attempts`.`id` LIMIT \\? FOR UPDATE").
		WithArgs(int64(71), uint32(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_id", "attempt_no", "state"}).AddRow(91, 71, 1, replycommand.AttemptDispatched))
}

func gatewayMetricsOutcome(metrics *ai.FileInputMetrics) aigateway.DispatchResult {
	return aigateway.DispatchResult{
		DispatchState: "dispatched", TerminalState: "succeeded",
		Usage: ai.UsageSnapshot{Status: ai.UsageStatusComplete, Items: []ai.UsageItem{
			{Category: ai.UsageCategoryInput, Unit: "token", Quantity: 1},
			{Category: ai.UsageCategoryOutput, Unit: "token", Quantity: 1},
		}},
		FileInputMetrics: metrics,
	}
}
