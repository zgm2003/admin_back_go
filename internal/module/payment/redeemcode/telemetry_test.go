package redeemcode

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/telemetry"
)

func TestTelemetryRecordsControlledRedeemCodeMetrics(t *testing.T) {
	memory := telemetry.NewMemoryRecorder()
	metrics := newMetrics(memory)
	metrics.batch("ok", "created")
	metrics.codes(3, "unused")
	metrics.transition(2, "voided", "admin_void")
	metrics.redemption("ok", "created", 250*time.Millisecond)
	metrics.conflict("generate", "request")

	events := memory.Events()
	wantNames := []string{
		"payment.redeem_code.batches",
		"payment.redeem_code.codes",
		"payment.redeem_code.state_transitions",
		"payment.redeem_code.redemptions",
		"payment.redeem_code.redemption_latency_seconds",
		"payment.redeem_code.conflicts",
	}
	if len(events) != len(wantNames) {
		t.Fatalf("events=%+v", events)
	}
	for index, want := range wantNames {
		if events[index].Name != want {
			t.Fatalf("event[%d].Name=%q want %q", index, events[index].Name, want)
		}
		for key := range events[index].Attributes {
			if key != "operation" && key != "outcome" && key != "state" && key != "reason" {
				t.Fatalf("event[%d] retained forbidden attribute %q", index, key)
			}
		}
	}
	if events[1].Value != 3 || events[2].Value != 2 || events[4].Value != 0.25 {
		t.Fatalf("metric values=%+v", events)
	}
}

func TestTelemetryServiceRedeemRecordsUsedAndSuccess(t *testing.T) {
	recorder := &captureRecorder{}
	repository := &fakeRepository{redeemFact: validTelemetryRedemptionFact()}
	service := NewService(repository, WithTelemetry(recorder), WithAttemptLimiter(newAllowAttemptLimiter()))

	response, appErr := service.Redeem(context.Background(), 7, "admin", "ZHR-2345-6789-ABCD-EFGH-JKMN")
	if appErr != nil || response == nil || response.Replayed {
		t.Fatalf("Redeem=(%+v,%+v)", response, appErr)
	}
	assertCapturedMetric(t, recorder.events, "payment.redeem_code.codes", map[string]any{
		"operation": "inventory", "state": "used",
	})
	assertCapturedMetric(t, recorder.events, "payment.redeem_code.redemptions", map[string]any{
		"operation": "redeem", "outcome": "ok", "reason": "created",
	})
	assertOnlyControlledTelemetryAttributes(t, recorder.events)
}

func TestTelemetryServiceRedeemRecordsExpiredUnavailable(t *testing.T) {
	recorder := &captureRecorder{}
	repository := &fakeRepository{redeemErr: ErrExpired}
	service := NewService(repository, WithTelemetry(recorder), WithAttemptLimiter(newAllowAttemptLimiter()))

	response, appErr := service.Redeem(context.Background(), 7, "admin", "ZHR-2345-6789-ABCD-EFGH-JKMN")
	if response != nil || appErr == nil || appErr.Code != ErrorWalletUnavailable {
		t.Fatalf("Redeem=(%+v,%+v)", response, appErr)
	}
	assertCapturedMetric(t, recorder.events, "payment.redeem_code.codes", map[string]any{
		"operation": "inventory", "state": "expired",
	})
	assertCapturedMetric(t, recorder.events, "payment.redeem_code.redemptions", map[string]any{
		"operation": "redeem", "outcome": "unavailable", "reason": "expired",
	})
	assertOnlyControlledTelemetryAttributes(t, recorder.events)
}

func TestTelemetryPassesOnlyBoundedAttributeNamesAndReasons(t *testing.T) {
	recorder := &captureRecorder{}
	metrics := newMetrics(recorder)
	metrics.redemption("error", "wallet_overflow", time.Second)
	metrics.conflict("redeem", "source_unique")

	if len(recorder.events) != 3 {
		t.Fatalf("events=%+v", recorder.events)
	}
	for _, event := range recorder.events {
		for key := range event.attributes {
			if key != "operation" && key != "outcome" && key != "state" && key != "reason" {
				t.Fatalf("forbidden telemetry attribute %q", key)
			}
		}
		if _, found := event.attributes["user_id"]; found {
			t.Fatal("telemetry must not include user identity")
		}
	}
	if recorder.events[0].attributes["reason"] != "wallet_overflow" || recorder.events[2].attributes["reason"] != "source_unique" {
		t.Fatalf("controlled reasons=%+v", recorder.events)
	}
}

type capturedMetric struct {
	name       string
	attributes telemetry.Attributes
}

type captureRecorder struct{ events []capturedMetric }

func (recorder *captureRecorder) Count(name string, _ float64, attributes telemetry.Attributes) {
	recorder.events = append(recorder.events, capturedMetric{name: name, attributes: attributes})
}

func (recorder *captureRecorder) Observe(name string, _ float64, attributes telemetry.Attributes) {
	recorder.events = append(recorder.events, capturedMetric{name: name, attributes: attributes})
}

func (recorder *captureRecorder) Start(ctx context.Context, _ string, _ telemetry.Attributes) (context.Context, func(error)) {
	return ctx, func(error) {}
}

func validTelemetryRedemptionFact() *RedemptionFact {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	return &RedemptionFact{
		AmountCents: 100,
		Transaction: &wallet.Transaction{
			ID: 9, TransactionNo: "WLT1", WalletID: 4, UserID: 7, Direction: wallet.DirectionIn,
			AmountUnits: 100_000_000, BalanceBeforeUnits: 0, BalanceAfterUnits: 100_000_000,
			SourceType: wallet.SourceRedeemCode, SourceID: 20, Remark: "RCB1", IsDel: enum.CommonNo, CreatedAt: now,
		},
		Wallet: &wallet.Wallet{ID: 4, UserID: 7, BalanceUnits: 100_000_000, TotalRechargeUnits: 100_000_000, IsDel: enum.CommonNo},
	}
}

func assertCapturedMetric(t *testing.T, events []capturedMetric, name string, want map[string]any) {
	t.Helper()
	for _, event := range events {
		if event.name != name {
			continue
		}
		matches := true
		for key, value := range want {
			if event.attributes[key] != value {
				matches = false
				break
			}
		}
		if matches {
			return
		}
	}
	t.Fatalf("metric %q with attributes %+v not found in %+v", name, want, events)
}

func assertOnlyControlledTelemetryAttributes(t *testing.T, events []capturedMetric) {
	t.Helper()
	for _, event := range events {
		for key := range event.attributes {
			if key != "operation" && key != "outcome" && key != "state" && key != "reason" {
				t.Fatalf("metric %q has forbidden attribute %q", event.name, key)
			}
		}
	}
}
