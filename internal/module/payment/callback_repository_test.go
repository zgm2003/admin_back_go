package payment

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGormRepositoryResolveCallbackEventRollsBackSettlementWhenAuditUpdateFails(t *testing.T) {
	participant := validFakePaymentParticipant()
	repo, mock, closeDB := newPaymentMockRepositoryWithParticipant(t, participant)
	defer closeDB()

	now := time.Date(2026, 8, 4, 14, 0, 0, 0, time.UTC)
	paidAt := now.Add(-time.Second)
	dedupeKey := make([]byte, 32)
	for index := range dedupeKey {
		dedupeKey[index] = byte(index + 1)
	}
	auditErr := errors.New("callback audit update failed")

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `payment_callback_events` WHERE id = ? AND dedupe_key = ? ORDER BY `payment_callback_events`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(30), dedupeKey, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "provider", "dedupe_key", "notify_id", "out_trade_no", "trade_no", "trade_status", "app_id",
			"total_amount_cents", "signature_valid", "process_status", "process_message", "raw_payload_json",
			"received_at", "processed_at", "is_del", "created_at", "updated_at",
		}).AddRow(
			int64(30), providerAlipay, dedupeKey, "notify-1", "PAY20260726115900000001", "202607262200", "TRADE_SUCCESS", "2026000000000000",
			int64(500), enum.CommonNo, callbackProcessPending, "", `{}`, now, nil, enum.CommonNo, now, now,
		))
	expectLockedPaymentOrder(mock, now, orderStatusPaying, nil)
	expectLockedRecharge(mock, now, rechargeStatusPaying, nil, nil)
	mock.ExpectExec(`UPDATE ` + "`payment_orders`" + ` SET .*` + "`alipay_trade_no`" + `=.*` + "`alipay_trade_no_identity`" + `=.* WHERE`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `payment_recharges` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `payment_callback_events` SET")).
		WillReturnError(auditErr)
	mock.ExpectRollback()

	result, err := repo.ResolveCallbackEvent(context.Background(), CallbackEventResolution{
		EventID:        30,
		DedupeKey:      dedupeKey,
		SignatureValid: enum.CommonYes,
		ProcessStatus:  callbackProcessSuccess,
		ProcessMessage: "credited",
		ProcessedAt:    now,
		PaidOrderID:    20,
		AlipayTradeNo:  "202607262200",
		PaidAt:         paidAt,
	})
	if result != nil || !errors.Is(err, auditErr) {
		t.Fatalf("audit update failure must roll back the whole callback settlement, result=%#v err=%v", result, err)
	}
	if participant.creditCalls != 1 {
		t.Fatalf("test must reach wallet participation before the forced audit failure, calls=%d", participant.creditCalls)
	}
	assertPaymentMockExpectations(t, mock)
}
