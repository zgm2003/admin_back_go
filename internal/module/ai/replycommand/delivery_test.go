package replycommand

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAppendDeliveryChunkCommitsFencedRunningCommandAndPreservesBytes(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	delta := "  你\n"

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`.*FOR UPDATE").
		WithArgs(uint64(41), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "state", "lease_owner", "lease_token", "lease_expires_at", "cancel_requested_at", "delivery_seq",
		}).AddRow(41, StateRunning, "worker-a", 7, now.Add(time.Minute), nil, 3))
	mock.ExpectExec("UPDATE `ai_reply_commands` SET `delivery_seq`=\\? WHERE id = \\?").
		WithArgs(uint32(4), uint64(41)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `ai_reply_delivery_chunks`").
		WithArgs(uint64(41), uint32(4), delta, now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := repository.AppendDeliveryChunk(context.Background(), AppendDeliveryChunkInput{
		CommandID: 41,
		Owner:     "worker-a",
		Token:     7,
		Delta:     delta,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Committed || result.DeliverySeq != 4 {
		t.Fatalf("result=%+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAppendDeliveryChunkRejectsUnfencedOrCanceledCommand(t *testing.T) {
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	canceledAt := now.Add(-time.Second)
	cases := []struct {
		name       string
		state      State
		owner      string
		token      uint64
		leaseUntil time.Time
		cancelAt   *time.Time
	}{
		{name: "not running", state: StateClaimed, owner: "worker-a", token: 7, leaseUntil: now.Add(time.Minute)},
		{name: "wrong owner", state: StateRunning, owner: "worker-b", token: 7, leaseUntil: now.Add(time.Minute)},
		{name: "wrong token", state: StateRunning, owner: "worker-a", token: 8, leaseUntil: now.Add(time.Minute)},
		{name: "expired", state: StateRunning, owner: "worker-a", token: 7, leaseUntil: now},
		{name: "canceled", state: StateRunning, owner: "worker-a", token: 7, leaseUntil: now.Add(time.Minute), cancelAt: &canceledAt},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, _, mock, closeDB := newAttemptMockRepository(t)
			defer closeDB()
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`.*FOR UPDATE").
				WithArgs(uint64(41), 1).
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "state", "lease_owner", "lease_token", "lease_expires_at", "cancel_requested_at", "delivery_seq",
				}).AddRow(41, tc.state, tc.owner, tc.token, tc.leaseUntil, tc.cancelAt, 3))
			mock.ExpectCommit()

			result, err := repository.AppendDeliveryChunk(context.Background(), AppendDeliveryChunkInput{
				CommandID: 41, Owner: "worker-a", Token: 7, Delta: "x", Now: now,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Committed || result.DeliverySeq != 0 {
				t.Fatalf("result=%+v", result)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAppendDeliveryChunkRejectsInvalidDelta(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	for _, delta := range []string{"", string([]byte{0xff}), strings.Repeat("a", MaxDeliveryChunkBytes+1)} {
		if _, err := repository.AppendDeliveryChunk(context.Background(), AppendDeliveryChunkInput{
			CommandID: 41, Owner: "worker-a", Token: 7, Delta: delta, Now: now,
		}); err == nil {
			t.Fatalf("invalid delta with %d bytes was accepted", len(delta))
		}
	}
	if !validDeliveryDelta(strings.Repeat("a", MaxDeliveryChunkBytes)) {
		t.Fatal("exactly 16 KiB must be accepted")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAppendDeliveryChunkRollsBackSequenceWhenInsertFails(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`.*FOR UPDATE").
		WithArgs(uint64(41), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "state", "lease_owner", "lease_token", "lease_expires_at", "cancel_requested_at", "delivery_seq",
		}).AddRow(41, StateRunning, "worker-a", 7, now.Add(time.Minute), nil, 3))
	mock.ExpectExec("UPDATE `ai_reply_commands` SET `delivery_seq`=\\? WHERE id = \\?").
		WithArgs(uint32(4), uint64(41)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `ai_reply_delivery_chunks`").
		WithArgs(uint64(41), uint32(4), "x", now).
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	result, err := repository.AppendDeliveryChunk(context.Background(), AppendDeliveryChunkInput{
		CommandID: 41, Owner: "worker-a", Token: 7, Delta: "x", Now: now,
	})
	if err == nil || result.Committed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReadDeliveryPrefixPreservesContinuousBytes(t *testing.T) {
	repository, db, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT `delivery_seq`,`delta` FROM `ai_reply_delivery_chunks` WHERE command_id = \\? AND delivery_seq <= \\? ORDER BY delivery_seq ASC").
		WithArgs(uint64(41), uint32(3)).
		WillReturnRows(sqlmock.NewRows([]string{"delivery_seq", "delta"}).
			AddRow(1, "  ").
			AddRow(2, "你").
			AddRow(3, "\n"))
	mock.ExpectRollback()

	tx := db.Begin()
	prefix, err := repository.ReadDeliveryPrefixTx(context.Background(), tx, 41, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !prefix.Consistent || prefix.StopDeliverySeq != 3 || prefix.Content != "  你\n" {
		t.Fatalf("prefix=%+v", prefix)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReadDeliveryPrefixDowngradesGapToZero(t *testing.T) {
	repository, db, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT `delivery_seq`,`delta` FROM `ai_reply_delivery_chunks` WHERE command_id = \\? AND delivery_seq <= \\? ORDER BY delivery_seq ASC").
		WithArgs(uint64(41), uint32(3)).
		WillReturnRows(sqlmock.NewRows([]string{"delivery_seq", "delta"}).
			AddRow(1, "1").
			AddRow(3, "3"))
	mock.ExpectRollback()

	tx := db.Begin()
	prefix, err := repository.ReadDeliveryPrefixTx(context.Background(), tx, 41, 3)
	if err != nil {
		t.Fatal(err)
	}
	if prefix.Consistent || prefix.StopDeliverySeq != 0 || prefix.Content != "" {
		t.Fatalf("prefix=%+v", prefix)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteDeliveryChunksClampsBatchTo256(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	mock.ExpectExec("DELETE FROM ai_reply_delivery_chunks WHERE command_id = \\? ORDER BY delivery_seq ASC LIMIT \\?").
		WithArgs(uint64(41), 256).
		WillReturnResult(sqlmock.NewResult(0, 256))

	deleted, err := repository.DeleteDeliveryChunks(context.Background(), 41, 1000)
	if err != nil || deleted != 256 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
