package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/billing"
	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/ai/contextengine"
	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/module/ai/requestidentity"
	airun "admin_back_go/internal/module/ai/run"
	walletmodule "admin_back_go/internal/module/payment/wallet"
	modulerealtime "admin_back_go/internal/module/realtime"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func deriveChatFinalizationTrigger(command replycommand.Command, attempts []replycommand.Attempt) (aigateway.FinalizationTrigger, error) {
	if command.CancelRequestedAt != nil {
		for _, attempt := range attempts {
			if strings.TrimSpace(attempt.DispatchState) == "dispatched" || strings.TrimSpace(attempt.DispatchState) == "unknown" {
				return aigateway.TriggerUserStop, nil
			}
		}
		return aigateway.TriggerUserStopBeforeDispatch, nil
	}
	if strings.TrimSpace(command.LastErrorCode) == "ai.provider_pre_dispatch_failed" &&
		strings.TrimSpace(command.LastErrorMessage) == string(aigateway.TriggerPreDispatchFailed) {
		return aigateway.TriggerPreDispatchFailed, nil
	}
	if strings.TrimSpace(command.LastErrorCode) == "ai.local_failed" &&
		strings.TrimSpace(command.LastErrorMessage) == string(aigateway.TriggerLocalFailure) {
		return aigateway.TriggerLocalFailure, nil
	}

	if strings.TrimSpace(command.LastErrorCode) == aigateway.ErrCodeInsufficientBalance {
		switch aigateway.FinalizationTrigger(strings.TrimSpace(command.LastErrorMessage)) {
		case aigateway.TriggerInitialInsufficient:
			return aigateway.TriggerInitialInsufficient, nil
		case aigateway.TriggerContinuationTopUpInsufficient:
			return aigateway.TriggerContinuationTopUpInsufficient, nil
		}
	}
	if len(attempts) == 0 {
		if command.State == replycommand.StateFailed {
			return aigateway.TriggerPreDispatchFailed, nil
		}
		return "", aigateway.ErrFinalizationPending
	}

	latest := attempts[len(attempts)-1]
	switch latest.State {
	case replycommand.AttemptSucceeded:
		if latest.ResultCandidateJSON != nil {
			if _, err := aichat.FinalChatAnswerFromCandidate(*latest.ResultCandidateJSON); err == nil {
				return aigateway.TriggerSuccess, nil
			}
			isToolCall, toolErr := aichat.IsToolCallCandidate(*latest.ResultCandidateJSON)
			if toolErr == nil && isToolCall {
				usage, usageErr := usageFromAttempt(latest)
				if usageErr == nil && usage.Complete() {
					return "", aigateway.ErrFinalizationPending
				}
			}
		}
		if terminalCandidateRequiresLocalFailure(latest) {
			return aigateway.TriggerLocalFailure, nil
		}
		return aigateway.TriggerSuccess, nil
	case replycommand.AttemptOutcomeUnknown:
		return aigateway.TriggerOutcomeUnknown, nil
	case replycommand.AttemptFailed, replycommand.AttemptCanceled:
		terminalMarker := strings.TrimSpace(command.LastErrorCode) == "ai.provider_failed" &&
			strings.TrimSpace(command.LastErrorMessage) == string(aigateway.TriggerProviderFailed)
		// A claim increments CommandAttempt before dispatch; only the persisted
		// provider attempt number proves a provider attempt was consumed.
		attemptsExhausted := command.MaxAttempts > 0 && latest.AttemptNo >= command.MaxAttempts
		if terminalMarker || attemptsExhausted {
			return aigateway.TriggerProviderFailed, nil
		}
	}
	return "", errors.Join(aigateway.ErrFinalizationPending, errors.New("chat command has no persisted terminal settlement trigger"))
}

func buildChatFinalizationFacts(run airun.Run, charge billing.UsageCharge, hold *walletmodule.Hold, command replycommand.Command, attempts []replycommand.Attempt) (aigateway.FinalizationFacts, error) {
	snapshot, err := gatewayRunSnapshot(run)
	if err != nil {
		return aigateway.FinalizationFacts{}, err
	}
	trigger, err := deriveChatFinalizationTrigger(command, attempts)
	if err != nil {
		return aigateway.FinalizationFacts{}, err
	}
	facts := aigateway.FinalizationFacts{
		Run: snapshot,
		Charge: aigateway.FinalizationCharge{
			ID: charge.ID, RunID: charge.RunID, UserID: charge.UserID, HeldUnits: charge.HeldUnits,
			HeldAuditMax: charge.HeldUnits, ActualUnits: charge.ActualUnits, Status: charge.Status,
		},
		Trigger: trigger,
	}
	if hold != nil {
		facts.Hold = aigateway.FinalizationHold{
			ID: hold.ID, WalletID: hold.WalletID, RunID: hold.RunID, UserID: hold.UserID,
			HeldUnits: hold.HeldUnits, HeldAuditMax: charge.HeldUnits, CapturedUnits: hold.CapturedUnits,
			Status: billing.HoldStatus(hold.Status),
		}
	}

	legacy := requestidentity.IdentityStatus(run.RequestIdentityStatus) == requestidentity.IdentityStatusLegacyNonReplayable
	facts.Attempts = make([]aigateway.FinalizationAttempt, 0, len(attempts))
	for _, row := range attempts {
		if row.ID == 0 || row.ID > math.MaxInt64 {
			return aigateway.FinalizationFacts{}, fmt.Errorf("attempt id %d is outside the billing identity range", row.ID)
		}
		attemptID := int64(row.ID)
		usage, usageErr := usageFromAttempt(row)
		if usageErr != nil {
			switch row.State {
			case replycommand.AttemptSucceeded, replycommand.AttemptFailed, replycommand.AttemptCanceled, replycommand.AttemptOutcomeUnknown:
				usage = infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
			default:
				return aigateway.FinalizationFacts{}, fmt.Errorf("load attempt %d usage: %w", row.ID, usageErr)
			}
		}
		evidenceKind := aigateway.AttemptEvidencePaid
		auditOnlyPrepared := (trigger == aigateway.TriggerPreDispatchFailed || trigger == aigateway.TriggerUserStopBeforeDispatch) &&
			row.State == replycommand.AttemptCanceled && strings.TrimSpace(row.DispatchState) == infraai.DispatchStateNotDispatched
		if legacy {
			evidenceKind = aigateway.AttemptEvidenceLegacyUnbillable
		} else if !auditOnlyPrepared {
			if _, quoteErr := paidQuoteFromAttempt(row); quoteErr != nil {
				return aigateway.FinalizationFacts{}, fmt.Errorf("load attempt %d quote: %w", row.ID, quoteErr)
			}
			if len(row.PreparedRequestSHA256) != sha256.Size || sha256.Sum256([]byte(row.PreparedRequestJSON)) != digestFromBytes(row.PreparedRequestSHA256) {
				return aigateway.FinalizationFacts{}, fmt.Errorf("attempt %d prepared request evidence is invalid", row.ID)
			}
		}
		responseHash, hashErr := decodeOptionalSHA256(row.ResponseSHA256)
		if hashErr != nil {
			return aigateway.FinalizationFacts{}, fmt.Errorf("load attempt %d response hash: %w", row.ID, hashErr)
		}
		facts.Attempts = append(facts.Attempts, aigateway.FinalizationAttempt{
			ID: attemptID, RunID: row.RunID, AttemptNo: uint32(row.AttemptNo), EvidenceKind: evidenceKind,
			State: billing.AttemptState(row.State), DispatchState: billing.DispatchState(row.DispatchState),
			Usage: usage, ProviderRequestID: strings.TrimSpace(row.ProviderRequestID), ResponseSHA256: responseHash,
		})
	}
	if len(attempts) > 0 {
		current := attempts[len(attempts)-1]
		currentID := int64(current.ID)
		facts.CurrentAttemptID = currentID
		if current.ResultCandidateJSON != nil {
			facts.Candidate = aigateway.FinalizationCandidate{AttemptID: currentID, JSON: strings.TrimSpace(*current.ResultCandidateJSON)}
		}
		if trigger == aigateway.TriggerUserStop && current.State == replycommand.AttemptCanceled && strings.TrimSpace(current.DispatchState) == "dispatched" {
			facts.StoppedAttemptID = currentID
		}
	}
	return facts, nil
}

func digestFromBytes(value []byte) [sha256.Size]byte {
	var digest [sha256.Size]byte
	copy(digest[:], value)
	return digest
}

func decodeOptionalSHA256(value string) ([sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	value = strings.TrimSpace(value)
	if value == "" {
		return digest, nil
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return digest, errors.New("response hash is not SHA-256")
	}
	copy(digest[:], decoded)
	return digest, nil
}

type gormGatewayFinalizationStore struct {
	db                   *gorm.DB
	wallets              *walletmodule.GormRepository
	replies              *replycommand.GormRepository
	eventSink            modulerealtime.TransactionalEventSink
	now                  func() time.Time
	conversationPayloads interface {
		BuildConversationIndexPayload(context.Context, uint64, uint64) (contextengine.ContextConversationIndexV1, error)
	}
	conversationEnqueuer interface {
		EnqueueConversationTurn(context.Context, contextengine.ContextConversationIndexV1) error
	}
	memoryPayloads interface {
		BuildMemoryBuildPayload(context.Context, uint64, uint64) (contextengine.ContextMemoryBuildV1, bool, error)
	}
	memoryEnqueuer interface {
		EnqueueMemoryBuild(context.Context, contextengine.ContextMemoryBuildV1) error
	}
}

type gatewayFinalizationOption func(*gormGatewayFinalizationStore)

func withConversationIndexPostCommit(payloads interface {
	BuildConversationIndexPayload(context.Context, uint64, uint64) (contextengine.ContextConversationIndexV1, error)
}, enqueuer interface {
	EnqueueConversationTurn(context.Context, contextengine.ContextConversationIndexV1) error
}) gatewayFinalizationOption {
	return func(store *gormGatewayFinalizationStore) {
		store.conversationPayloads = payloads
		store.conversationEnqueuer = enqueuer
	}
}

func withMemoryPostCommit(payloads interface {
	BuildMemoryBuildPayload(context.Context, uint64, uint64) (contextengine.ContextMemoryBuildV1, bool, error)
}, enqueuer interface {
	EnqueueMemoryBuild(context.Context, contextengine.ContextMemoryBuildV1) error
}) gatewayFinalizationOption {
	return func(store *gormGatewayFinalizationStore) {
		store.memoryPayloads = payloads
		store.memoryEnqueuer = enqueuer
	}
}

func (store *gormGatewayFinalizationStore) enqueueContextEnhancements(ctx context.Context, conversationID, userID, userMessageID uint64) {
	if store == nil || conversationID == 0 || userID == 0 || userMessageID == 0 {
		return
	}
	if store.conversationPayloads != nil && store.conversationEnqueuer != nil {
		payload, err := store.conversationPayloads.BuildConversationIndexPayload(ctx, conversationID, userMessageID)
		if err == nil {
			err = store.conversationEnqueuer.EnqueueConversationTurn(ctx, payload)
		}
		if err != nil {
			slog.WarnContext(ctx, "AI conversation index enqueue deferred to reconciler", "conversation_id", conversationID, "user_message_id", userMessageID, "error", err)
		}
	}
	if store.memoryPayloads != nil && store.memoryEnqueuer != nil {
		payload, ok, err := store.memoryPayloads.BuildMemoryBuildPayload(ctx, conversationID, userID)
		if err == nil && ok {
			err = store.memoryEnqueuer.EnqueueMemoryBuild(ctx, payload)
		}
		if err != nil {
			slog.WarnContext(ctx, "AI conversation memory enqueue deferred to reconciler", "conversation_id", conversationID, "error", err)
		}
	}
}

func newGormGatewayFinalizationStore(db *gorm.DB, wallets *walletmodule.GormRepository, replies *replycommand.GormRepository, eventSink modulerealtime.TransactionalEventSink, options ...gatewayFinalizationOption) *gormGatewayFinalizationStore {
	if db == nil || wallets == nil || replies == nil || eventSink == nil {
		return nil
	}
	store := &gormGatewayFinalizationStore{db: db, wallets: wallets, replies: replies, eventSink: eventSink, now: time.Now}
	for _, option := range options {
		if option != nil {
			option(store)
		}
	}
	return store
}

func (store *gormGatewayFinalizationStore) WithLockedSettlement(ctx context.Context, runID int64, decide func(aigateway.FinalizationFacts) (aigateway.SettlementDecision, error)) (aigateway.FinalizationApplyResult, error) {
	if store == nil || store.db == nil || store.wallets == nil || store.replies == nil || store.eventSink == nil || runID <= 0 || decide == nil {
		return aigateway.FinalizationApplyResult{}, aigateway.ErrNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	if store.now != nil {
		now = store.now()
	}
	var applied bool
	var replayed bool
	var commandID uint64
	var terminalUserID uint64
	var terminalConversationID uint64
	var terminalUserMessageID uint64
	var terminalState replycommand.State
	var terminalConversationDeleted bool
	var durableEvent *modulerealtime.Event
	err := store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		run, charge, wallet, hold, err := lockChatSettlementMoneyGraph(ctx, tx, runID)
		if err != nil {
			return err
		}
		command, attempts, err := lockChatSettlementBusinessGraph(ctx, tx, run)
		if err != nil {
			return err
		}
		commandID = command.ID
		terminalUserID = uint64(command.UserID)
		if command.ConversationID > 0 && command.UserMessageID > 0 {
			terminalConversationID = uint64(command.ConversationID)
			terminalUserMessageID = uint64(command.UserMessageID)
		}
		if terminalChatBilling(run, charge) {
			if err := validateChatFinalizationReplay(run, charge, wallet, hold, command, attempts); err != nil {
				return err
			}
			if err := airun.ProjectTerminalDashboardFacts(ctx, tx, run.ID); err != nil {
				return err
			}
			replayed = true
			return nil
		}
		if run.Status != enum.AIRunStatusRunning || charge.Status != billing.ChargeStatusOpen || charge.FinalizedAt != nil {
			return errors.New("AI chat settlement is neither open nor a valid terminal replay")
		}
		attempts, err = normalizeChatAttemptsForFinalization(ctx, tx, command, attempts, now)
		if err != nil {
			return err
		}
		facts, err := buildChatFinalizationFacts(run, charge, hold, command, attempts)
		if err != nil {
			return err
		}
		decision, err := decide(facts)
		if err != nil {
			return err
		}
		if err := applyChatMoneyDecision(ctx, tx, store.wallets, facts, decision); err != nil {
			return err
		}
		if err := insertSettledUsageItems(ctx, tx, charge.ID, decision, now); err != nil {
			return err
		}
		commandInput, err := paidCommandInputFromDecision(command, facts, decision, now)
		if err != nil {
			return err
		}
		terminalState = commandInput.State
		commandResult, err := store.replies.FinalizePaidCommandInTransaction(ctx, tx, commandInput)
		if err != nil {
			return err
		}
		terminalConversationDeleted = commandResult.ConversationDeleted
		if err := finalizeChatRunAndCharge(ctx, tx, run, charge, facts, decision, commandResult, now); err != nil {
			return err
		}
		if err := clearChatResultCandidates(ctx, tx, run.ID); err != nil {
			return err
		}
		if err := appendChatRunFinalizationEvents(ctx, tx, run.ID, facts, decision, now); err != nil {
			return err
		}
		durableEvent, err = appendChatRealtimeFinalization(ctx, tx, store.eventSink, command, commandResult, commandInput, now)
		if err != nil {
			return err
		}
		applied = true
		return nil
	})
	if err != nil {
		return aigateway.FinalizationApplyResult{}, err
	}
	if durableEvent != nil {
		store.eventSink.PublishBestEffort(context.WithoutCancel(ctx), durableEvent)
	}
	if commandID > 0 {
		cleanupCtx := context.WithoutCancel(ctx)
		if cleanupErr := replycommand.CleanupDeliveryChunks(cleanupCtx, store.replies, commandID, 4); cleanupErr != nil {
			slog.WarnContext(cleanupCtx, "AI reply delivery cleanup deferred to reconciler", "command_id", commandID, "error", cleanupErr)
		}
	}
	if applied && !terminalConversationDeleted && (terminalState == replycommand.StateSucceeded || terminalState == replycommand.StateCanceled) {
		store.enqueueContextEnhancements(context.WithoutCancel(ctx), terminalConversationID, terminalUserID, terminalUserMessageID)
	}
	return aigateway.FinalizationApplyResult{Applied: applied, Replayed: replayed}, nil
}

func lockChatSettlementMoneyGraph(ctx context.Context, tx *gorm.DB, runID int64) (airun.Run, billing.UsageCharge, *walletmodule.Wallet, *walletmodule.Hold, error) {
	var run airun.Run
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", runID).First(&run).Error; err != nil {
		return airun.Run{}, billing.UsageCharge{}, nil, nil, err
	}
	var charge billing.UsageCharge
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ?", runID).First(&charge).Error; err != nil {
		return airun.Run{}, billing.UsageCharge{}, nil, nil, err
	}
	var wallet walletmodule.Wallet
	walletErr := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND is_del = ?", run.UserID, enum.CommonNo).First(&wallet).Error
	if walletErr != nil && !errors.Is(walletErr, gorm.ErrRecordNotFound) {
		return airun.Run{}, billing.UsageCharge{}, nil, nil, walletErr
	}
	var walletFact *walletmodule.Wallet
	if walletErr == nil {
		walletFact = &wallet
	}
	var hold walletmodule.Hold
	holdErr := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ?", runID).First(&hold).Error
	if holdErr != nil && !errors.Is(holdErr, gorm.ErrRecordNotFound) {
		return airun.Run{}, billing.UsageCharge{}, nil, nil, holdErr
	}
	var holdFact *walletmodule.Hold
	if holdErr == nil {
		holdFact = &hold
		if walletFact == nil || hold.WalletID != wallet.ID || hold.UserID != run.UserID {
			return airun.Run{}, billing.UsageCharge{}, nil, nil, walletmodule.ErrHoldOwnerMismatch
		}
	}
	return run, charge, walletFact, holdFact, nil
}

func lockChatSettlementBusinessGraph(ctx context.Context, tx *gorm.DB, run airun.Run) (replycommand.Command, []replycommand.Attempt, error) {
	var command replycommand.Command
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND request_id = ?", run.UserID, run.RequestID).First(&command).Error; err != nil {
		return replycommand.Command{}, nil, err
	}
	var attempts []replycommand.Attempt
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("run_id = ?", run.ID).Order("attempt_no ASC").Find(&attempts).Error; err != nil {
		return replycommand.Command{}, nil, err
	}
	return command, attempts, nil
}

func normalizeChatAttemptsForFinalization(ctx context.Context, tx *gorm.DB, command replycommand.Command, attempts []replycommand.Attempt, now time.Time) ([]replycommand.Attempt, error) {
	if len(attempts) == 0 {
		return attempts, nil
	}
	latest := &attempts[len(attempts)-1]
	updates := map[string]any(nil)
	preDispatchFailure := strings.TrimSpace(command.LastErrorCode) == "ai.provider_pre_dispatch_failed" &&
		strings.TrimSpace(command.LastErrorMessage) == string(aigateway.TriggerPreDispatchFailed)
	if (command.CancelRequestedAt != nil || preDispatchFailure) && latest.State == replycommand.AttemptPrepared && strings.TrimSpace(latest.DispatchState) == infraai.DispatchStateNotDispatched {
		errorCode := "ai.user_stopped"
		if preDispatchFailure {
			errorCode = "ai.provider_pre_dispatch_failed"
		}
		updates = map[string]any{
			"state": replycommand.AttemptCanceled, "dispatch_state": infraai.DispatchStateNotDispatched,
			"usage_status": billing.UsageStatusUnavailable, "usage_json": `{"status":"unavailable"}`,
			"result_candidate_json": nil, "provider_request_id": "", "response_sha256": "",
			"error_code": errorCode, "finished_at": now, "updated_at": now,
		}
	} else if command.State == replycommand.StateOutcomeUnknown && latest.State == replycommand.AttemptDispatched {
		updates = map[string]any{
			"state": replycommand.AttemptOutcomeUnknown, "dispatch_state": infraai.DispatchStateUnknown,
			"usage_status": billing.UsageStatusUnavailable, "usage_json": `{"status":"unavailable"}`,
			"result_candidate_json": nil, "response_sha256": "", "error_code": "ai.provider_outcome_unknown",
			"finished_at": now, "updated_at": now,
		}
	}
	if updates == nil {
		return attempts, nil
	}
	result := tx.WithContext(ctx).Model(&replycommand.Attempt{}).Where("id = ? AND state = ?", latest.ID, latest.State).Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, replycommand.ErrAttemptTerminalConflict
	}
	if err := tx.WithContext(ctx).Where("id = ?", latest.ID).First(latest).Error; err != nil {
		return nil, err
	}
	return attempts, nil
}

func terminalChatBilling(run airun.Run, charge billing.UsageCharge) bool {
	runTerminal := billing.BillingStatus(run.BillingStatus) == billing.BillingStatusSettled || billing.BillingStatus(run.BillingStatus) == billing.BillingStatusReleased || billing.BillingStatus(run.BillingStatus) == billing.BillingStatusUnbilled
	chargeTerminal := charge.Status == billing.ChargeStatusSettled || charge.Status == billing.ChargeStatusReleased || charge.Status == billing.ChargeStatusUnbilled
	return runTerminal && chargeTerminal
}

func validateChatFinalizationReplay(run airun.Run, charge billing.UsageCharge, wallet *walletmodule.Wallet, hold *walletmodule.Hold, command replycommand.Command, attempts []replycommand.Attempt) error {
	if charge.FinalizedAt == nil || charge.RunID != run.ID || charge.UserID != run.UserID || charge.ActualUnits < 0 {
		return errors.New("terminal AI charge replay facts are invalid")
	}
	switch billing.BillingStatus(run.BillingStatus) {
	case billing.BillingStatusSettled:
		if charge.Status != billing.ChargeStatusSettled || hold == nil || wallet == nil || hold.Status != walletmodule.HoldCaptured || hold.HeldUnits != 0 || hold.CapturedUnits != charge.ActualUnits {
			return errors.New("settled AI replay facts are inconsistent")
		}
	case billing.BillingStatusReleased:
		if charge.Status != billing.ChargeStatusReleased || charge.ActualUnits != 0 || (hold != nil && (hold.Status != walletmodule.HoldReleased || hold.HeldUnits != 0 || hold.CapturedUnits != 0)) {
			return errors.New("released AI replay facts are inconsistent")
		}
	case billing.BillingStatusUnbilled:
		if charge.Status != billing.ChargeStatusUnbilled || charge.ActualUnits != 0 || hold == nil || hold.Status != walletmodule.HoldReleased || hold.HeldUnits != 0 || hold.CapturedUnits != 0 {
			return errors.New("unbilled AI replay facts are inconsistent")
		}
	default:
		return errors.New("AI replay billing status is not terminal")
	}
	if !chatCommandMatchesRunTerminal(command, run) {
		return errors.New("terminal AI command replay facts are inconsistent")
	}
	for _, attempt := range attempts {
		if attempt.ResultCandidateJSON != nil {
			return errors.New("terminal AI replay retained a result candidate")
		}
	}
	return nil
}

func chatCommandMatchesRunTerminal(command replycommand.Command, run airun.Run) bool {
	if command.UserID != run.UserID || command.RequestID != run.RequestID || command.FinishedAt == nil {
		return false
	}
	switch run.Status {
	case enum.AIRunStatusSuccess:
		return command.State == replycommand.StateSucceeded && command.AssistantMessageID != nil && run.AssistantMessageID != nil && *command.AssistantMessageID == *run.AssistantMessageID
	case enum.AIRunStatusCanceled:
		return command.State == replycommand.StateCanceled && command.AssistantMessageID != nil && run.AssistantMessageID != nil && *command.AssistantMessageID == *run.AssistantMessageID
	case enum.AIRunStatusOutcomeUnknown:
		return command.State == replycommand.StateOutcomeUnknown && command.AssistantMessageID == nil && run.AssistantMessageID == nil
	case enum.AIRunStatusFailed:
		return command.State == replycommand.StateFailed && command.AssistantMessageID == nil && run.AssistantMessageID == nil
	default:
		return false
	}
}

func applyChatMoneyDecision(ctx context.Context, tx *gorm.DB, wallets *walletmodule.GormRepository, facts aigateway.FinalizationFacts, decision aigateway.SettlementDecision) error {
	switch decision.MoneyAction {
	case aigateway.SettlementMoneyCapture:
		if facts.Hold.ID <= 0 {
			return errors.New("AI capture decision has no authoritative Hold")
		}
		_, ledger, err := wallets.CaptureHoldInTx(ctx, tx, walletmodule.CaptureHoldInput{
			UserID: facts.Run.UserID, RunID: facts.Run.RunID, ActualUnits: decision.ActualUnits, SourceSummary: decision.LedgerSummary,
		})
		if err != nil {
			return err
		}
		if (decision.ActualUnits == 0 && ledger != nil) || (decision.ActualUnits > 0 && ledger == nil) {
			return walletmodule.ErrHoldIntegrity
		}
		return nil
	case aigateway.SettlementMoneyRelease:
		if facts.Hold.ID == 0 {
			if facts.Trigger != aigateway.TriggerInitialInsufficient && facts.Trigger != aigateway.TriggerUserStopBeforeDispatch && facts.Trigger != aigateway.TriggerPreDispatchFailed {
				return errors.New("AI release decision has no authoritative Hold")
			}
			return nil
		}
		_, err := wallets.ReleaseHoldInTx(ctx, tx, walletmodule.ReleaseHoldInput{UserID: facts.Run.UserID, RunID: facts.Run.RunID})
		return err
	default:
		return errors.New("AI settlement money action is invalid")
	}
}

func insertSettledUsageItems(ctx context.Context, tx *gorm.DB, chargeID int64, decision aigateway.SettlementDecision, now time.Time) error {
	if decision.ChargeStatus != billing.ChargeStatusSettled {
		return nil
	}
	for _, raw := range decision.Items {
		item := raw
		item.ID = 0
		item.ChargeID = chargeID
		item.CreatedAt = now
		if err := tx.WithContext(ctx).Create(&item).Error; err != nil {
			return err
		}
	}
	return nil
}

func paidCommandInputFromDecision(command replycommand.Command, facts aigateway.FinalizationFacts, decision aigateway.SettlementDecision, now time.Time) (replycommand.PaidCommandFinalizationInput, error) {
	input := replycommand.PaidCommandFinalizationInput{CommandID: command.ID, UserID: command.UserID, RequestID: command.RequestID, Now: now}
	switch decision.RunStatus {
	case enum.AIRunStatusSuccess:
		if decision.CandidateAction != aigateway.SettlementCandidatePublish {
			return input, errors.New("successful AI settlement did not publish its candidate")
		}
		answer, err := aichat.FinalChatAnswerFromCandidate(decision.Candidate.JSON)
		if err != nil {
			return input, err
		}
		input.State = replycommand.StateSucceeded
		input.Content = answer
	case enum.AIRunStatusCanceled:
		input.State = replycommand.StateCanceled
	case enum.AIRunStatusOutcomeUnknown:
		input.State = replycommand.StateOutcomeUnknown
		input.ErrorCode, input.ErrorMessage = chatFinalizationFailure(facts, decision)
	case enum.AIRunStatusFailed:
		input.State = replycommand.StateFailed
		input.ErrorCode, input.ErrorMessage = chatFinalizationFailure(facts, decision)
	default:
		return input, errors.New("AI settlement run status is invalid")
	}
	return input, nil
}

func chatFinalizationFailure(facts aigateway.FinalizationFacts, decision aigateway.SettlementDecision) (string, string) {
	switch {
	case facts.Trigger == aigateway.TriggerInitialInsufficient || facts.Trigger == aigateway.TriggerContinuationTopUpInsufficient:
		return aigateway.ErrCodeInsufficientBalance, "余额不足，请充值后重试"
	case decision.BillingReason == billing.BillingReasonUnbilledOverHold:
		return "ai.billing.over_hold", "AI用量超过冻结上限，本次未扣费"
	case decision.BillingReason == billing.BillingReasonUnbilledUsageIncomplete:
		return "ai.billing.usage_incomplete", "AI用量不完整，本次未扣费"
	case facts.Trigger == aigateway.TriggerOutcomeUnknown:
		return "ai.provider_outcome_unknown", "上游结果无法确认，本次未扣费"
	default:
		return "ai.provider_failed", "AI生成失败，本次未扣费"
	}
}

func finalizeChatRunAndCharge(ctx context.Context, tx *gorm.DB, run airun.Run, charge billing.UsageCharge, facts aigateway.FinalizationFacts, decision aigateway.SettlementDecision, commandResult *replycommand.PaidCommandFinalizationResult, now time.Time) error {
	prompt, completion, total, err := finalizationTokenTotals(facts)
	if err != nil {
		return err
	}
	updates := map[string]any{
		"status": decision.RunStatus, "billing_status": decision.BillingStatus, "billing_reason": decision.BillingReason,
		"prompt_tokens": prompt, "completion_tokens": completion, "total_tokens": total,
		"finished_at": now, "settled_at": now, "duration_ms": finalizationDurationMS(run.StartedAt, now), "updated_at": now,
	}
	if decision.RunStatus == enum.AIRunStatusSuccess {
		if commandResult == nil || commandResult.AssistantMessageID <= 0 {
			return errors.New("successful AI settlement has no assistant message")
		}
		updates["assistant_message_id"] = commandResult.AssistantMessageID
		updates["error_message"] = ""
	} else {
		if decision.RunStatus == enum.AIRunStatusCanceled {
			if commandResult == nil || commandResult.AssistantMessageID <= 0 {
				return errors.New("canceled AI settlement has no assistant message")
			}
			updates["assistant_message_id"] = commandResult.AssistantMessageID
		} else {
			updates["assistant_message_id"] = nil
		}
		_, message := chatFinalizationFailure(facts, decision)
		if decision.RunStatus == enum.AIRunStatusCanceled {
			message = "用户停止生成"
		}
		if decision.BillingAnomaly != "" {
			message = strings.TrimSpace(message + ": " + decision.BillingAnomaly)
		}
		updates["error_message"] = truncateFinalizationMessage(message)
	}
	runUpdate := tx.WithContext(ctx).Model(&airun.Run{}).
		Where("id = ? AND status = ? AND billing_status IN ?", run.ID, enum.AIRunStatusRunning, []billing.BillingStatus{billing.BillingStatusPending, billing.BillingStatusHeld}).
		Updates(updates)
	if runUpdate.Error != nil {
		return runUpdate.Error
	}
	if runUpdate.RowsAffected != 1 {
		return errors.New("AI Run terminal compare-and-set was rejected")
	}
	actualUnits := int64(0)
	if decision.ChargeStatus == billing.ChargeStatusSettled {
		actualUnits = decision.ActualUnits
	}
	chargeUpdate := tx.WithContext(ctx).Model(&billing.UsageCharge{}).
		Where("id = ? AND run_id = ? AND status = ? AND finalized_at IS NULL", charge.ID, run.ID, billing.ChargeStatusOpen).
		Updates(map[string]any{"actual_units": actualUnits, "status": decision.ChargeStatus, "finalized_at": now, "updated_at": now})
	if chargeUpdate.Error != nil {
		return chargeUpdate.Error
	}
	if chargeUpdate.RowsAffected != 1 {
		return errors.New("AI usage charge terminal compare-and-set was rejected")
	}
	return airun.ProjectTerminalDashboardFacts(ctx, tx, run.ID)
}

func finalizationTokenTotals(facts aigateway.FinalizationFacts) (uint, uint, uint, error) {
	var prompt int64
	var completion int64
	for _, attempt := range facts.Attempts {
		billable := attempt.State == billing.AttemptStateSucceeded || attempt.ID == facts.StoppedAttemptID
		if !billable || !attempt.Usage.Complete() {
			continue
		}
		for _, item := range attempt.Usage.Items {
			switch strings.TrimSpace(item.Category) {
			case infraai.UsageCategoryInput, infraai.UsageCategoryCacheRead, infraai.UsageCategoryCacheWrite:
				if item.Quantity > math.MaxInt64-prompt {
					return 0, 0, 0, errors.New("AI prompt token total overflows")
				}
				prompt += item.Quantity
			case infraai.UsageCategoryOutput:
				if item.Quantity > math.MaxInt64-completion {
					return 0, 0, 0, errors.New("AI completion token total overflows")
				}
				completion += item.Quantity
			}
		}
	}
	if prompt < 0 || completion < 0 || prompt > math.MaxInt64-completion || uint64(prompt) > uint64(^uint(0)) || uint64(completion) > uint64(^uint(0)) || uint64(prompt+completion) > uint64(^uint(0)) {
		return 0, 0, 0, errors.New("AI token totals exceed Run storage")
	}
	return uint(prompt), uint(completion), uint(prompt + completion), nil
}

func finalizationDurationMS(startedAt *time.Time, finishedAt time.Time) uint {
	if startedAt == nil || startedAt.IsZero() || finishedAt.Before(*startedAt) {
		return 0
	}
	value := finishedAt.Sub(*startedAt).Milliseconds()
	if value <= 0 {
		return 0
	}
	if uint64(value) > uint64(^uint(0)) {
		return ^uint(0)
	}
	return uint(value)
}

func clearChatResultCandidates(ctx context.Context, tx *gorm.DB, runID int64) error {
	return tx.WithContext(ctx).Model(&replycommand.Attempt{}).Where("run_id = ? AND result_candidate_json IS NOT NULL", runID).Update("result_candidate_json", nil).Error
}

func appendChatRunFinalizationEvents(ctx context.Context, tx *gorm.DB, runID int64, facts aigateway.FinalizationFacts, decision aigateway.SettlementDecision, now time.Time) error {
	var maxSeq uint
	if err := tx.WithContext(ctx).Model(&airun.RunEvent{}).Where("run_id = ?", runID).Select("COALESCE(MAX(seq), 0)").Scan(&maxSeq).Error; err != nil {
		return err
	}
	events := make([]airun.RunEvent, 0, 3)
	if hasCompleteFinalizationUsage(facts) {
		events = append(events, airun.RunEvent{EventType: enum.AIRunEventUsageRecorded, Message: enum.AIRunEventLabels[enum.AIRunEventUsageRecorded]})
	}
	businessType := enum.AIRunEventFailed
	switch decision.RunStatus {
	case enum.AIRunStatusSuccess:
		businessType = enum.AIRunEventCompleted
	case enum.AIRunStatusCanceled:
		businessType = enum.AIRunEventCanceled
	case enum.AIRunStatusOutcomeUnknown:
		businessType = enum.AIRunEventOutcomeUnknown
	}
	events = append(events, airun.RunEvent{EventType: businessType, Message: enum.AIRunEventLabels[businessType]})
	billingType := enum.AIRunEventReleased
	switch decision.BillingStatus {
	case billing.BillingStatusSettled:
		billingType = enum.AIRunEventSettled
	case billing.BillingStatusUnbilled:
		billingType = enum.AIRunEventUnbilled
	}
	billingMessage := enum.AIRunEventLabels[billingType]
	if decision.BillingAnomaly != "" {
		billingMessage = truncateFinalizationMessage(billingMessage + ": " + decision.BillingAnomaly)
	}
	events = append(events, airun.RunEvent{EventType: billingType, Message: billingMessage})
	for index := range events {
		events[index].RunID = runID
		events[index].Seq = maxSeq + uint(index) + 1
		events[index].CreatedAt = now
		if err := tx.WithContext(ctx).Create(&events[index]).Error; err != nil {
			return err
		}
	}
	return nil
}

func hasCompleteFinalizationUsage(facts aigateway.FinalizationFacts) bool {
	for _, attempt := range facts.Attempts {
		if attempt.Usage.Complete() {
			return true
		}
	}
	return false
}

func appendChatRealtimeFinalization(ctx context.Context, tx *gorm.DB, sink modulerealtime.TransactionalEventSink, command replycommand.Command, result *replycommand.PaidCommandFinalizationResult, input replycommand.PaidCommandFinalizationInput, now time.Time) (*modulerealtime.Event, error) {
	if result != nil && result.ConversationDeleted {
		return nil, nil
	}
	eventInput := modulerealtime.AppendInput{RequestID: command.RequestID, UserID: command.UserID, OccurredAt: now}
	switch input.State {
	case replycommand.StateSucceeded:
		if result == nil || result.AssistantMessageID <= 0 {
			return nil, errors.New("completed realtime event has no assistant message")
		}
		eventInput.Type = modulerealtime.TypeAIResponseCompletedV1
		eventInput.Payload = modulerealtime.AIResponseCompletedPayload{ConversationID: command.ConversationID, RequestID: command.RequestID, AssistantMessageID: result.AssistantMessageID}
	case replycommand.StateCanceled:
		if result == nil || result.AssistantMessageID <= 0 {
			return nil, errors.New("canceled realtime event has no assistant message")
		}
		eventInput.Type = modulerealtime.TypeAIResponseCanceledV2
		eventInput.Payload = modulerealtime.AIResponseCanceledPayload{
			ConversationID:     command.ConversationID,
			RequestID:          command.RequestID,
			AssistantMessageID: result.AssistantMessageID,
		}
	case replycommand.StateFailed, replycommand.StateOutcomeUnknown:
		var walletPath *string
		var rechargePath *string
		if input.ErrorCode == aigateway.ErrCodeInsufficientBalance {
			wallet := "/profile/wallet"
			recharge := "/payment/recharge"
			walletPath, rechargePath = &wallet, &recharge
		}
		eventInput.Type = modulerealtime.TypeAIResponseFailedV1
		eventInput.Payload = modulerealtime.AIResponseFailedPayload{
			ConversationID: command.ConversationID, RequestID: command.RequestID, Msg: input.ErrorMessage,
			ErrorCode: input.ErrorCode, WalletPath: walletPath, RechargePath: rechargePath,
		}
	default:
		return nil, errors.New("AI command realtime terminal state is invalid")
	}
	return sink.AppendTx(ctx, tx, eventInput)
}

func truncateFinalizationMessage(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 1024 {
		return string(runes[:1024])
	}
	return value
}

var _ aigateway.FinalizationStore = (*gormGatewayFinalizationStore)(nil)
