package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/billing"
	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/ai/contextengine"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/module/ai/replycommand"
	airun "admin_back_go/internal/module/ai/run"
	walletmodule "admin_back_go/internal/module/payment/wallet"
	modulerealtime "admin_back_go/internal/module/realtime"
	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPersistedSettlementPricingSelectsFrozenTierPerAttempt(t *testing.T) {
	rates := []pricing.Rate{
		{Category: pricing.InputTokens, Unit: "token", TierKey: "short_context", PriceUnits: 1, UnitScale: 1},
		{Category: pricing.InputTokens, Unit: "token", TierKey: "long_context", PriceUnits: 2, UnitScale: 1},
		{Category: pricing.OutputTokens, Unit: "token", TierKey: "short_context", PriceUnits: 3, UnitScale: 1},
		{Category: pricing.OutputTokens, Unit: "token", TierKey: "long_context", PriceUnits: 6, UnitScale: 1},
	}
	model := officialmodel.ResolvedModel{
		Model:          officialmodel.Model{CatalogVersion: "catalog-v3", CatalogVendor: "openai", ModelID: "gpt-tiered", ContextWindowTokens: 2000, MaxOutputTokens: 1000, ContextTierThresholdTokens: 50},
		EffectivePrice: pricing.PriceBook{ModelID: "gpt-tiered", ContextTierThresholdTokens: 50, Rates: rates}, PriceSource: officialmodel.PriceSourceOfficial,
		PriceSourceURL: "https://openai.com/pricing", PriceVerifiedAt: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
	}
	raw, err := aigateway.EncodePricingSnapshot(model, aigateway.PricingSnapshotInput{
		TransportEngine: "openai", RequestedModelID: model.Model.ModelID, EffectiveMaxOutputTokens: 100, MultiplierPPM: 1_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	usage := func(inputTokens int64) infraai.UsageSnapshot {
		t.Helper()
		snapshot, usageErr := infraai.NewUsageSnapshot(infraai.UsageStatusReported, []byte(`{"usage":"complete"}`), []infraai.UsageItem{
			{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: inputTokens},
			{Category: infraai.UsageCategoryOutput, Unit: "token", Quantity: 1},
		})
		if usageErr != nil {
			t.Fatal(usageErr)
		}
		return snapshot
	}
	quoted, err := (persistedSettlementPricer{}).PriceSettlement(context.Background(), aigateway.SettlementPricingInput{
		Run: aigateway.RunSnapshot{PricingSnapshotJSON: raw},
		Attempts: []aigateway.BillableAttempt{
			{ID: 11, AttemptNo: 1, Usage: usage(50)},
			{ID: 12, AttemptNo: 2, Usage: usage(51)},
		},
	})
	if err != nil || quoted.ActualUnits != 161 || len(quoted.Items) != 4 {
		t.Fatalf("settlement = %#v, %v", quoted, err)
	}
	for _, item := range quoted.Items {
		wantTier := "short_context"
		if item.AttemptID == 12 {
			wantTier = "long_context"
		}
		if item.TierKey != wantTier {
			t.Fatalf("attempt %d tier = %q, want %q", item.AttemptID, item.TierKey, wantTier)
		}
	}
}

func TestPersistedSettlementPricingOmitsZeroQuantityItems(t *testing.T) {
	rates := []pricing.Rate{
		{Category: pricing.InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1},
		{Category: pricing.OutputTokens, Unit: "token", PriceUnits: 3, UnitScale: 1},
		{Category: pricing.CacheWrite, Unit: "token", PriceUnits: 5, UnitScale: 1},
	}
	model := officialmodel.ResolvedModel{
		Model:          officialmodel.Model{CatalogVersion: "catalog-v3", CatalogVendor: "openai", ModelID: "gpt-zero-usage", ContextWindowTokens: 2000, MaxOutputTokens: 1000},
		EffectivePrice: pricing.PriceBook{ModelID: "gpt-zero-usage", Rates: rates}, PriceSource: officialmodel.PriceSourceOfficial,
		PriceSourceURL: "https://openai.com/pricing", PriceVerifiedAt: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
	}
	raw, err := aigateway.EncodePricingSnapshot(model, aigateway.PricingSnapshotInput{
		TransportEngine: "openai", RequestedModelID: model.Model.ModelID, EffectiveMaxOutputTokens: 100, MultiplierPPM: 1_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	usage, err := infraai.NewUsageSnapshot(infraai.UsageStatusReported, []byte(`{"usage":"complete"}`), []infraai.UsageItem{
		{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: 10},
		{Category: infraai.UsageCategoryOutput, Unit: "token", Quantity: 2},
		{Category: infraai.UsageCategoryCacheWrite, Unit: "token", Quantity: 0},
	})
	if err != nil {
		t.Fatal(err)
	}

	quoted, err := (persistedSettlementPricer{}).PriceSettlement(context.Background(), aigateway.SettlementPricingInput{
		Run:      aigateway.RunSnapshot{PricingSnapshotJSON: raw},
		Attempts: []aigateway.BillableAttempt{{ID: 11, AttemptNo: 1, Usage: usage}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if quoted.ActualUnits != 16 || len(quoted.Items) != 2 {
		t.Fatalf("settlement = %#v", quoted)
	}
	for _, item := range quoted.Items {
		if item.Quantity <= 0 {
			t.Fatalf("non-consumed usage became a charge item: %#v", item)
		}
	}
}

func TestFinalizationWritesSettledAtInTerminalTransaction(t *testing.T) {
	db, mock, closeDB := newFinalizerMockDB(t)
	defer closeDB()
	now := time.Date(2026, 7, 28, 11, 0, 3, 123456000, time.UTC)
	startedAt := now.Add(-time.Second)
	mock.ExpectExec("UPDATE `ai_runs` SET .*`settled_at`=\\?.* WHERE id = \\? AND status = \\? AND billing_status IN \\(\\?,\\?\\)").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `ai_usage_charges` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("(?is)SELECT EXISTS .* FROM ai_run_dashboard_facts").WillReturnRows(sqlmock.NewRows([]string{"fact_exists"}).AddRow(false))
	mock.ExpectExec("(?is)INSERT INTO ai_run_dashboard_facts .* WHERE r.id = \\?").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("(?is)INSERT INTO ai_run_dashboard_daily_facts").WillReturnResult(sqlmock.NewResult(0, 1))

	err := finalizeChatRunAndCharge(context.Background(), db,
		airun.Run{ID: 41, StartedAt: &startedAt},
		billing.UsageCharge{ID: 51},
		aigateway.FinalizationFacts{},
		aigateway.SettlementDecision{RunStatus: enum.AIRunStatusFailed, BillingStatus: billing.BillingStatusReleased, BillingReason: billing.BillingReasonReleasedProviderFailed, ChargeStatus: billing.ChargeStatusReleased},
		nil,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCanceledChatFinalizationWritesStoppedAssistantMessageID(t *testing.T) {
	db, mock, closeDB := newFinalizerMockDB(t)
	defer closeDB()
	now := time.Date(2026, 7, 28, 11, 0, 3, 123456000, time.UTC)
	startedAt := now.Add(-time.Second)
	mock.ExpectExec("UPDATE `ai_runs` SET").
		WithArgs(
			int64(97), billing.BillingReasonUnbilledUsageIncomplete, billing.BillingStatusUnbilled,
			uint(0), uint(1000), "用户停止生成", now, uint(0), now, enum.AIRunStatusCanceled, uint(0), now,
			int64(41), enum.AIRunStatusRunning, billing.BillingStatusPending, billing.BillingStatusHeld,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `ai_usage_charges` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("(?is)SELECT EXISTS .* FROM ai_run_dashboard_facts").WillReturnRows(sqlmock.NewRows([]string{"fact_exists"}).AddRow(true))

	err := finalizeChatRunAndCharge(context.Background(), db,
		airun.Run{ID: 41, StartedAt: &startedAt},
		billing.UsageCharge{ID: 51},
		aigateway.FinalizationFacts{},
		aigateway.SettlementDecision{
			RunStatus: enum.AIRunStatusCanceled, BillingStatus: billing.BillingStatusUnbilled,
			BillingReason: billing.BillingReasonUnbilledUsageIncomplete, ChargeStatus: billing.ChargeStatusUnbilled,
		},
		&replycommand.PaidCommandFinalizationResult{AssistantMessageID: 97},
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestChatCommandMatchesCanceledRunRequiresSameStoppedMessage(t *testing.T) {
	finishedAt := time.Date(2026, 7, 28, 11, 0, 3, 0, time.UTC)
	commandMessageID := int64(97)
	runMessageID := int64(97)
	command := replycommand.Command{
		UserID: 9, RequestID: "request-1", State: replycommand.StateCanceled,
		AssistantMessageID: &commandMessageID, FinishedAt: &finishedAt,
	}
	run := airun.Run{
		UserID: 9, RequestID: "request-1", Status: enum.AIRunStatusCanceled,
		AssistantMessageID: &runMessageID,
	}
	if !chatCommandMatchesRunTerminal(command, run) {
		t.Fatal("matching stopped assistant message was rejected")
	}
	runMessageID = 98
	if chatCommandMatchesRunTerminal(command, run) {
		t.Fatal("mismatched stopped assistant message was accepted")
	}
}

type noopFinalizationEventSink struct{}

func (noopFinalizationEventSink) AppendTx(context.Context, *gorm.DB, modulerealtime.AppendInput) (*modulerealtime.Event, error) {
	return nil, errors.New("unexpected realtime append during terminal replay")
}

func (noopFinalizationEventSink) PublishBestEffort(context.Context, *modulerealtime.Event) {}

type capturingFinalizationEventSink struct {
	input modulerealtime.AppendInput
	calls int
}

func (sink *capturingFinalizationEventSink) AppendTx(_ context.Context, _ *gorm.DB, input modulerealtime.AppendInput) (*modulerealtime.Event, error) {
	sink.calls++
	sink.input = input
	return &modulerealtime.Event{}, nil
}

func (*capturingFinalizationEventSink) PublishBestEffort(context.Context, *modulerealtime.Event) {}

type contextEnhancementPayloads struct {
	indexCalls  int
	memoryCalls int
}

func (stub *contextEnhancementPayloads) BuildConversationIndexPayload(context.Context, uint64, uint64) (contextengine.ContextConversationIndexV1, error) {
	stub.indexCalls++
	return contextengine.ContextConversationIndexV1{ProfileID: 1, ConversationID: 3, UserMessageID: 7, SourceSHA256: sha256.Sum256([]byte("turn"))}, nil
}

func (stub *contextEnhancementPayloads) BuildMemoryBuildPayload(context.Context, uint64, uint64) (contextengine.ContextMemoryBuildV1, bool, error) {
	stub.memoryCalls++
	return contextengine.ContextMemoryBuildV1{}, false, nil
}

type contextEnhancementEnqueuers struct{ indexCalls int }

func (stub *contextEnhancementEnqueuers) EnqueueConversationTurn(context.Context, contextengine.ContextConversationIndexV1) error {
	stub.indexCalls++
	return errors.New("index queue unavailable")
}

func (*contextEnhancementEnqueuers) EnqueueMemoryBuild(context.Context, contextengine.ContextMemoryBuildV1) error {
	return nil
}

func TestHistoricalAttachmentEnhancementsRemainIndependentAfterFinalization(t *testing.T) {
	payloads := &contextEnhancementPayloads{}
	enqueuers := &contextEnhancementEnqueuers{}
	store := &gormGatewayFinalizationStore{conversationPayloads: payloads, conversationEnqueuer: enqueuers,
		memoryPayloads: payloads, memoryEnqueuer: enqueuers}
	store.enqueueContextEnhancements(t.Context(), 3, 9, 7)
	if payloads.indexCalls != 1 || enqueuers.indexCalls != 1 || payloads.memoryCalls != 1 {
		t.Fatalf("index payloads=%d enqueues=%d memory payloads=%d", payloads.indexCalls, enqueuers.indexCalls, payloads.memoryCalls)
	}
}

func TestCanceledChatFinalizationPublishesV2WithStoppedAssistantMessage(t *testing.T) {
	sink := &capturingFinalizationEventSink{}
	result := &replycommand.PaidCommandFinalizationResult{AssistantMessageID: 97}
	_, err := appendChatRealtimeFinalization(
		context.Background(), &gorm.DB{}, sink,
		replycommand.Command{ConversationID: 3, UserID: 9, RequestID: "request-1"},
		result,
		replycommand.PaidCommandFinalizationInput{State: replycommand.StateCanceled},
		time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := sink.input.Payload.(modulerealtime.AIResponseCanceledPayload)
	if sink.input.Type != modulerealtime.TypeAIResponseCanceledV2 || !ok || payload.AssistantMessageID != 97 {
		t.Fatalf("event=%+v payload=%#v", sink.input, sink.input.Payload)
	}
}

func TestDeletedConversationFinalizationDoesNotPublishRealtimeTerminal(t *testing.T) {
	sink := &capturingFinalizationEventSink{}
	result := &replycommand.PaidCommandFinalizationResult{AssistantMessageID: 97, ConversationDeleted: true}
	event, err := appendChatRealtimeFinalization(
		context.Background(), &gorm.DB{}, sink,
		replycommand.Command{ConversationID: 3, UserID: 9, RequestID: "request-1"},
		result,
		replycommand.PaidCommandFinalizationInput{State: replycommand.StateCanceled},
		time.Date(2026, 8, 4, 14, 0, 0, 0, time.UTC),
	)
	if err != nil || event != nil || sink.calls != 0 {
		t.Fatalf("event=%+v calls=%d err=%v", event, sink.calls, err)
	}
}

func TestChatFinalizationCleanupRunsAfterCommitAndDoesNotUndoTerminalFacts(t *testing.T) {
	db, mock, closeDB := newFinalizerMockDB(t)
	defer closeDB()
	finishedAt := time.Date(2026, 7, 28, 11, 0, 3, 0, time.UTC)
	assistantID := int64(97)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `ai_runs`.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "request_id", "status", "billing_status", "assistant_message_id", "finished_at",
		}).AddRow(41, 9, "request-1", enum.AIRunStatusCanceled, billing.BillingStatusReleased, assistantID, finishedAt))
	mock.ExpectQuery("SELECT .* FROM `ai_usage_charges`.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "run_id", "user_id", "actual_units", "status", "finalized_at",
		}).AddRow(51, 41, 9, 0, billing.ChargeStatusReleased, finishedAt))
	mock.ExpectQuery("SELECT .* FROM `user_wallets`.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `wallet_holds`.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "request_id", "state", "assistant_message_id", "finished_at",
		}).AddRow(61, 9, "request-1", replycommand.StateCanceled, assistantID, finishedAt))
	mock.ExpectQuery("SELECT .* FROM `ai_provider_attempts`.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("(?is)SELECT EXISTS .* FROM ai_run_dashboard_facts").
		WillReturnRows(sqlmock.NewRows([]string{"fact_exists"}).AddRow(true))
	mock.ExpectCommit()
	mock.ExpectExec("DELETE FROM ai_reply_delivery_chunks").
		WithArgs(uint64(61), 256).
		WillReturnError(errors.New("temporary cleanup failure"))

	replies := replycommand.NewGormRepository(&database.Client{Gorm: db})
	store := newGormGatewayFinalizationStore(
		db,
		walletmodule.NewGormRepositoryFromDB(db),
		replies,
		noopFinalizationEventSink{},
	)
	result, err := store.WithLockedSettlement(context.Background(), 41, func(aigateway.FinalizationFacts) (aigateway.SettlementDecision, error) {
		t.Fatal("terminal replay must not decide settlement again")
		return aigateway.SettlementDecision{}, nil
	})
	if err != nil || result.Applied || !result.Replayed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func newFinalizerMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatal(err)
	}
	return db, mock, func() { _ = sqlDB.Close() }
}

func TestDeriveChatFinalizationTriggerUsesOnlyPersistedFacts(t *testing.T) {
	tests := []struct {
		name     string
		command  replycommand.Command
		attempts []replycommand.Attempt
		want     aigateway.FinalizationTrigger
		wantErr  error
	}{
		{
			name:    "cancel before any dispatch",
			command: replycommand.Command{CancelRequestedAt: nonNilTime()},
			want:    aigateway.TriggerUserStopBeforeDispatch,
		},
		{
			name:    "cancel after drained success",
			command: replycommand.Command{CancelRequestedAt: nonNilTime()},
			attempts: []replycommand.Attempt{{
				ID: 7, State: replycommand.AttemptSucceeded, DispatchState: "dispatched",
			}},
			want: aigateway.TriggerUserStop,
		},
		{
			name:    "initial insufficient",
			command: replycommand.Command{LastErrorCode: aigateway.ErrCodeInsufficientBalance, LastErrorMessage: string(aigateway.TriggerInitialInsufficient)},
			want:    aigateway.TriggerInitialInsufficient,
		},
		{
			name:    "pre dispatch failure",
			command: replycommand.Command{LastErrorCode: "ai.provider_pre_dispatch_failed", LastErrorMessage: string(aigateway.TriggerPreDispatchFailed)},
			want:    aigateway.TriggerPreDispatchFailed,
		},
		{
			name:    "orphaned failed command without provider attempt",
			command: replycommand.Command{State: replycommand.StateFailed, LastErrorCode: "internal.unknown", LastErrorMessage: "解密AI供应商API Key失败"},
			want:    aigateway.TriggerPreDispatchFailed,
		},
		{
			name:    "local failure",
			command: replycommand.Command{LastErrorCode: "ai.local_failed", LastErrorMessage: string(aigateway.TriggerLocalFailure)},
			attempts: []replycommand.Attempt{{
				ID: 7, State: replycommand.AttemptSucceeded, DispatchState: "dispatched",
			}},
			want: aigateway.TriggerLocalFailure,
		},
		{
			name:    "continuation insufficient",
			command: replycommand.Command{LastErrorCode: aigateway.ErrCodeInsufficientBalance, LastErrorMessage: string(aigateway.TriggerContinuationTopUpInsufficient)},
			attempts: []replycommand.Attempt{{
				ID: 7, State: replycommand.AttemptSucceeded, DispatchState: "dispatched",
			}},
			want: aigateway.TriggerContinuationTopUpInsufficient,
		},
		{
			name: "success",
			attempts: []replycommand.Attempt{{
				ID: 7, State: replycommand.AttemptSucceeded, DispatchState: "dispatched",
			}},
			want: aigateway.TriggerSuccess,
		},
		{
			name: "complete tool candidate remains pending for continuation",
			attempts: []replycommand.Attempt{{
				ID: 7, State: replycommand.AttemptSucceeded, DispatchState: "dispatched",
				UsageJSON:           `{"status":"reported","items":[{"category":"input","unit":"token","quantity":1}]}`,
				ResultCandidateJSON: stringPointer(`{"version":"ai_chat_result_v1","tool_calls":[{"id":"call-1","name":"lookup"}]}`),
			}},
			wantErr: aigateway.ErrFinalizationPending,
		},
		{
			name: "outcome unknown",
			attempts: []replycommand.Attempt{{
				ID: 7, State: replycommand.AttemptOutcomeUnknown, DispatchState: "unknown",
			}},
			want: aigateway.TriggerOutcomeUnknown,
		},
		{
			name:    "user stop beats outcome unknown",
			command: replycommand.Command{CancelRequestedAt: nonNilTime()},
			attempts: []replycommand.Attempt{{
				ID: 7, State: replycommand.AttemptOutcomeUnknown, DispatchState: "unknown",
			}},
			want: aigateway.TriggerUserStop,
		},
		{
			name:    "terminal provider failure marker",
			command: replycommand.Command{LastErrorCode: "ai.provider_failed", LastErrorMessage: string(aigateway.TriggerProviderFailed)},
			attempts: []replycommand.Attempt{{
				ID: 7, State: replycommand.AttemptFailed, DispatchState: "dispatched",
			}},
			want: aigateway.TriggerProviderFailed,
		},
		{
			name:    "retryable provider failure remains open",
			command: replycommand.Command{AttemptCount: 1, MaxAttempts: 3},
			attempts: []replycommand.Attempt{{
				ID: 7, AttemptNo: 1, State: replycommand.AttemptFailed, DispatchState: "dispatched",
			}},
			wantErr: aigateway.ErrFinalizationPending,
		},
		{
			name:    "third claim remains open after two provider failures",
			command: replycommand.Command{AttemptCount: 3, MaxAttempts: 3},
			attempts: []replycommand.Attempt{{
				ID: 7, AttemptNo: 2, State: replycommand.AttemptFailed, DispatchState: "dispatched",
			}},
			wantErr: aigateway.ErrFinalizationPending,
		},
		{
			name:    "third provider failure finalizes",
			command: replycommand.Command{AttemptCount: 3, MaxAttempts: 3},
			attempts: []replycommand.Attempt{{
				ID: 7, AttemptNo: 3, State: replycommand.AttemptFailed, DispatchState: "dispatched",
			}},
			want: aigateway.TriggerProviderFailed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := deriveChatFinalizationTrigger(test.command, test.attempts)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("derive trigger error=%v want=%v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("derive trigger=%q want=%q", got, test.want)
			}
		})
	}
}

func stringPointer(value string) *string { return &value }

func TestPaidAttemptRetryableRejectedProviderErrorDoesNotFinalize(t *testing.T) {
	executor := &paidChatAttemptExecutor{}
	input := aichat.PaidChatAttemptInput{CommandMaxAttempts: 3}
	attempt := aigateway.ProviderAttempt{AttemptNo: 1}
	err := infraai.NewProviderError(infraai.ProviderOutcomeRejected, "provider-request-1", infraai.ErrRateLimited)
	if executor.mustFinalizeProviderError(input, attempt, aigateway.DispatchResult{TerminalState: "failed"}, err) {
		t.Fatal("retryable rejected provider error finalized the held run")
	}
}

func TestPaidAttemptTerminalRejectedUpstreamFailureFinalizes(t *testing.T) {
	executor := &paidChatAttemptExecutor{}
	input := aichat.PaidChatAttemptInput{CommandMaxAttempts: 3}
	attempt := aigateway.ProviderAttempt{AttemptNo: 1}
	err := infraai.NewProviderError(
		infraai.ProviderOutcomeRejected,
		"provider-request-terminal",
		fmt.Errorf("%w: terminal Responses failure", infraai.ErrUpstreamFailed),
	)
	if !executor.mustFinalizeProviderError(input, attempt, aigateway.DispatchResult{TerminalState: "failed"}, err) {
		t.Fatal("terminal rejected Responses failure remained retryable")
	}
}

func TestPaidAttemptTerminalProviderErrorsFinalize(t *testing.T) {
	executor := &paidChatAttemptExecutor{}
	for _, tc := range []struct {
		name    string
		input   aichat.PaidChatAttemptInput
		attempt aigateway.ProviderAttempt
		err     error
	}{
		{"unauthorized", aichat.PaidChatAttemptInput{CommandMaxAttempts: 3}, aigateway.ProviderAttempt{AttemptNo: 1}, infraai.NewProviderError(infraai.ProviderOutcomeRejected, "provider-request-1", infraai.ErrUnauthorized)},
		{"max attempts", aichat.PaidChatAttemptInput{CommandMaxAttempts: 2}, aigateway.ProviderAttempt{AttemptNo: 2}, infraai.NewProviderError(infraai.ProviderOutcomeRejected, "provider-request-2", infraai.ErrRateLimited)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !executor.mustFinalizeProviderError(tc.input, tc.attempt, aigateway.DispatchResult{TerminalState: "failed"}, tc.err) {
				t.Fatal("terminal provider error remained retryable")
			}
		})
	}
}

func TestTerminalFinalAnswerCandidateReplaysFinalizationWithoutAnotherProviderAttempt(t *testing.T) {
	finalCandidate := `{"version":"ai_chat_result_v1","answer":"durable answer"}`
	if !replayFinalizationForTerminalAttempt(replycommand.Attempt{State: replycommand.AttemptSucceeded, ResultCandidateJSON: &finalCandidate}) {
		t.Fatal("terminal final answer was eligible for another provider attempt")
	}
	toolCandidate := `{"version":"ai_chat_result_v1","tool_calls":[{"id":"call-1","name":"lookup"}]}`
	usage, err := infraai.NewUsageSnapshot(infraai.UsageStatusReported, []byte(`{"usage":"complete"}`), []infraai.UsageItem{{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: 1}})
	if err != nil {
		t.Fatal(err)
	}
	usageJSON, err := json.Marshal(usage)
	if err != nil {
		t.Fatal(err)
	}
	if replayFinalizationForTerminalAttempt(replycommand.Attempt{State: replycommand.AttemptSucceeded, ResultCandidateJSON: &toolCandidate, UsageJSON: string(usageJSON)}) {
		t.Fatal("tool result blocked its required continuation")
	}
	unavailableUsage := `{"status":"unavailable"}`
	if !replayFinalizationForTerminalAttempt(replycommand.Attempt{State: replycommand.AttemptSucceeded, ResultCandidateJSON: &toolCandidate, UsageJSON: unavailableUsage}) {
		t.Fatal("usage-unavailable tool result was allowed to redispatch")
	}
	malformedCandidate := `{"version":"unknown","tool_calls":[{"id":"call-1","name":"lookup"}]}`
	if !replayFinalizationForTerminalAttempt(replycommand.Attempt{State: replycommand.AttemptSucceeded, ResultCandidateJSON: &malformedCandidate, UsageJSON: string(usageJSON)}) {
		t.Fatal("malformed tool candidate was allowed to redispatch")
	}
}

func TestNextAttemptRestoresPersistedToolCandidateBeforeCreatingContinuation(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	usage, err := infraai.NewUsageSnapshot(infraai.UsageStatusReported, []byte(`{"usage":"complete"}`), []infraai.UsageItem{{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: 1}})
	if err != nil {
		t.Fatal(err)
	}
	usageJSON, err := json.Marshal(usage)
	if err != nil {
		t.Fatal(err)
	}
	candidate := `{"version":"ai_chat_result_v1","tool_calls":[{"id":"call-1","name":"lookup"}]}`
	rows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "run_id", "command_id", "attempt_no", "state", "usage_json", "result_candidate_json"}).
			AddRow(91, 100, 41, 1, replycommand.AttemptSucceeded, string(usageJSON), candidate)
	}
	executor := &paidChatAttemptExecutor{db: db}
	mock.ExpectQuery("SELECT .* FROM `ai_provider_attempts`").WillReturnRows(rows())
	attemptNo, prepared, finalization, recovered, err := executor.nextAttempt(context.Background(), 100, 41, infraai.ChatInput{})
	if err != nil || attemptNo != 0 || prepared || finalization || recovered == nil || len(recovered.ToolCalls) != 1 || recovered.UsageStatus != infraai.UsageStatusReported || !recovered.Usage.Complete() || recovered.PromptTokens != 1 || recovered.TotalTokens != 1 {
		t.Fatalf("first recovery attempt=%d prepared=%v finalization=%v recovered=%+v err=%v", attemptNo, prepared, finalization, recovered, err)
	}
	mock.ExpectQuery("SELECT .* FROM `ai_provider_attempts`").WillReturnRows(rows())
	attemptNo, prepared, finalization, recovered, err = executor.nextAttempt(context.Background(), 100, 41, infraai.ChatInput{ToolCalls: recovered.ToolCalls, ToolOutputs: []infraai.ToolOutput{{CallID: "call-1", Name: "lookup", Output: "ok"}}})
	if err != nil || attemptNo != 2 || providerAttemptLimitExceeded(attemptNo, 3) || prepared || finalization || recovered != nil {
		t.Fatalf("continuation attempt=%d prepared=%v finalization=%v recovered=%+v err=%v", attemptNo, prepared, finalization, recovered, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProviderAttemptLimitExceeded(t *testing.T) {
	if providerAttemptLimitExceeded(2, 3) {
		t.Fatal("max-1 continuation was blocked")
	}
	if !providerAttemptLimitExceeded(3, 2) {
		t.Fatal("attempt beyond provider cap was allowed")
	}
}

func TestToolContinuationAtProviderAttemptLimitFinalizesWithoutProviderDispatch(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	usage, err := infraai.NewUsageSnapshot(infraai.UsageStatusReported, []byte(`{"usage":"complete"}`), []infraai.UsageItem{{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: 1}})
	if err != nil {
		t.Fatal(err)
	}
	usageJSON, err := json.Marshal(usage)
	if err != nil {
		t.Fatal(err)
	}
	candidate := `{"version":"ai_chat_result_v1","tool_calls":[{"id":"call-1","name":"lookup"}]}`
	rows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "run_id", "command_id", "attempt_no", "state", "usage_json", "result_candidate_json"}).
			AddRow(91, 100, 41, 2, replycommand.AttemptSucceeded, string(usageJSON), candidate)
	}
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"last_error_code", "last_error_message"}).AddRow("", ""))
	mock.ExpectQuery("SELECT .* FROM `ai_provider_attempts`").WillReturnRows(rows())
	mock.ExpectExec("UPDATE `ai_reply_commands` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM `ai_provider_attempts`").WillReturnRows(rows())
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(41))
	finalizer := &successfulPaidFinalizer{}
	executor := &paidChatAttemptExecutor{db: db, wallets: &walletmodule.GormRepository{}, replies: &replycommand.GormRepository{}, finalizer: finalizer}
	result, err := executor.ExecutePaidChatAttempt(context.Background(), aichat.PaidChatAttemptInput{
		RunID: 100, CommandID: 41, LeaseOwner: "worker-a", LeaseToken: 7, CommandMaxAttempts: 2,
		ChatInput: infraai.ChatInput{ToolCalls: []infraai.ToolCall{{ID: "call-1", Name: "lookup"}}, ToolOutputs: []infraai.ToolOutput{{CallID: "call-1", Name: "lookup", Output: "ok"}}},
	})
	if err != nil || result == nil || !result.Finalized || finalizer.calls != 1 {
		t.Fatalf("result=%+v calls=%d err=%v", result, finalizer.calls, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteTerminalCandidateNilOrMalformedRequiresLocalFailure(t *testing.T) {
	usage, err := infraai.NewUsageSnapshot(infraai.UsageStatusReported, []byte(`{"usage":"complete"}`), []infraai.UsageItem{{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: 1}})
	if err != nil {
		t.Fatal(err)
	}
	usageJSON, err := json.Marshal(usage)
	if err != nil {
		t.Fatal(err)
	}
	malformed := `{"version":"unknown","answer":"answer"}`
	for _, candidate := range []*string{nil, &malformed} {
		if !terminalCandidateRequiresLocalFailure(replycommand.Attempt{State: replycommand.AttemptSucceeded, UsageJSON: string(usageJSON), ResultCandidateJSON: candidate}) {
			t.Fatalf("candidate=%v did not require local failure", candidate)
		}
	}
	unavailable := `{"status":"unavailable"}`
	if terminalCandidateRequiresLocalFailure(replycommand.Attempt{State: replycommand.AttemptSucceeded, UsageJSON: unavailable}) {
		t.Fatal("usage unavailable was not left for unbilled finalization")
	}
}

type flakyPaidFinalizer struct{ calls int }

type successfulPaidFinalizer struct{ calls int }

func (f *successfulPaidFinalizer) Finalize(context.Context, aigateway.FinalizeRequest) error {
	f.calls++
	return nil
}

func (f *flakyPaidFinalizer) Finalize(context.Context, aigateway.FinalizeRequest) error {
	f.calls++
	if f.calls == 1 {
		return errors.New("first settlement commit failed")
	}
	return nil
}

func TestFinalizationRetryReentersFinalizerWithoutProviderAttempt(t *testing.T) {
	for _, tc := range []struct {
		name, code, marker string
	}{
		{"provider failure", "ai.provider_failed", string(aigateway.TriggerProviderFailed)},
		{"local tool failure", "ai.local_failed", string(aigateway.TriggerLocalFailure)},
		{"pre dispatch failure", "ai.provider_pre_dispatch_failed", string(aigateway.TriggerPreDispatchFailed)},
		{"initial insufficient", aigateway.ErrCodeInsufficientBalance, string(aigateway.TriggerInitialInsufficient)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })
			db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), SkipDefaultTransaction: true})
			if err != nil {
				t.Fatal(err)
			}
			markerRows := func() *sqlmock.Rows {
				return sqlmock.NewRows([]string{"last_error_code", "last_error_message"}).AddRow(tc.code, tc.marker)
			}
			mock.ExpectQuery("SELECT").WillReturnRows(markerRows())
			mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id"}))
			finalizer := &flakyPaidFinalizer{}
			executor := &paidChatAttemptExecutor{db: db, wallets: &walletmodule.GormRepository{}, replies: &replycommand.GormRepository{}, finalizer: finalizer}
			input := aichat.PaidChatAttemptInput{RunID: 100, CommandID: 41}
			if _, err := executor.ExecutePaidChatAttempt(context.Background(), input); !errors.Is(err, aichat.ErrPaidFinalizationRetry) {
				t.Fatalf("first execution err=%v", err)
			}
			mock.ExpectQuery("SELECT").WillReturnRows(markerRows())
			mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id"}))
			mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(41))
			result, err := executor.ExecutePaidChatAttempt(context.Background(), input)
			if err != nil || result == nil || !result.Finalized || finalizer.calls != 2 {
				t.Fatalf("result=%+v calls=%d err=%v", result, finalizer.calls, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBuildChatFinalizationFactsPreservesPaidEvidenceAndCurrentCandidate(t *testing.T) {
	rawUsage := []byte(`{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}`)
	usage, err := infraai.NewUsageSnapshot(infraai.UsageStatusReported, rawUsage, []infraai.UsageItem{
		{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: 2},
		{Category: infraai.UsageCategoryOutput, Unit: "token", Quantity: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	usageJSON, _ := json.Marshal(usage)
	responseHash := sha256.Sum256([]byte("provider response"))
	requestHash := sha256.Sum256([]byte(`{"model":"model-7"}`))
	candidate := `{"version":"ai_chat_result_v1","answer":"hello"}`
	run := airun.Run{
		ID: 44, UserID: 9, AgentID: 7, RequestID: "request-1", RequestFingerprint: make([]byte, sha256.Size),
		RequestIdentityStatus: "replayable", PricingSnapshotJSON: testPricingSnapshotJSON(),
		ModelID: "model-7", ModelDisplayName: "Model Seven", Status: enum.AIRunStatusRunning,
		BillingStatus: string(billing.BillingStatusHeld), BillingReason: string(billing.BillingReasonHeld),
	}
	charge := billing.UsageCharge{ID: 101, RunID: 44, UserID: 9, HeldUnits: 12, Status: billing.ChargeStatusOpen}
	hold := &walletmodule.Hold{ID: 102, WalletID: 103, RunID: 44, UserID: 9, HeldUnits: 12, Status: walletmodule.HoldActive}
	command := replycommand.Command{ID: 55, UserID: 9, RequestID: "request-1", AttemptCount: 1, MaxAttempts: 3}
	attempts := []replycommand.Attempt{{
		ID: 201, RunID: 44, AttemptNo: 1, State: replycommand.AttemptSucceeded,
		PreparedRequestJSON: `{"model":"model-7"}`, PreparedRequestSHA256: requestHash[:],
		QuoteJSON: `{"pricing_version":"catalog-v1","effective_max_output_tokens":10,"current_call_max_units":12,"target_hold_units":12}`,
		UsageJSON: string(usageJSON), UsageStatus: "complete", DispatchState: "dispatched",
		ProviderRequestID: "provider-1", ResponseSHA256: hex.EncodeToString(responseHash[:]), ResultCandidateJSON: &candidate,
	}}

	facts, err := buildChatFinalizationFacts(run, charge, hold, command, attempts)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Trigger != aigateway.TriggerSuccess || facts.CurrentAttemptID != 201 || facts.Candidate.AttemptID != 201 || facts.Candidate.JSON != candidate {
		t.Fatalf("facts identity=%+v", facts)
	}
	if len(facts.Attempts) != 1 || facts.Attempts[0].EvidenceKind != aigateway.AttemptEvidencePaid || facts.Attempts[0].Usage.Status != infraai.UsageStatusReported || facts.Attempts[0].ResponseSHA256 != responseHash {
		t.Fatalf("attempt facts=%+v", facts.Attempts)
	}

	corrupt := attempts[0]
	corrupt.State = replycommand.AttemptCanceled
	corrupt.DispatchState = infraai.DispatchStateNotDispatched
	corrupt.UsageJSON = `{"status":"unavailable"}`
	corrupt.QuoteJSON = `{}`
	corrupt.PreparedRequestSHA256 = []byte("corrupt")
	corrupt.ResultCandidateJSON = nil
	corrupt.ResponseSHA256 = ""
	command.LastErrorCode = "ai.provider_pre_dispatch_failed"
	command.LastErrorMessage = string(aigateway.TriggerPreDispatchFailed)
	facts, err = buildChatFinalizationFacts(run, charge, hold, command, []replycommand.Attempt{corrupt})
	if err != nil || facts.Trigger != aigateway.TriggerPreDispatchFailed || len(facts.Attempts) != 1 || facts.Attempts[0].State != billing.AttemptStateCanceled {
		t.Fatalf("corrupt prepared audit facts=%+v err=%v", facts, err)
	}

	malformedUsage := attempts[0]
	malformedUsage.UsageJSON = `{"status":"reported","items":[]}`
	malformedUsage.ResultCandidateJSON = nil
	command.LastErrorCode = ""
	command.LastErrorMessage = ""
	facts, err = buildChatFinalizationFacts(run, charge, hold, command, []replycommand.Attempt{malformedUsage})
	if err != nil || facts.Attempts[0].Usage.Status != infraai.UsageStatusUnavailable {
		t.Fatalf("malformed usage facts=%+v err=%v", facts, err)
	}
}

func testPricingSnapshotJSON() string {
	return `{"version":"catalog-v1","billable":true,"catalog_vendor":"test","transport_engine":"openai","requested_model_id":"model-7","canonical_model_id":"model-7","catalog_max_output_tokens":100,"effective_max_output_tokens":10,"multiplier_ppm":1000000,"source_url":"https://example.test/pricing","retrieved_at":"2026-07-26","rates":[{"category":"input","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000},{"category":"output","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000}]}`
}

func nonNilTime() *time.Time {
	value := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	return &value
}
