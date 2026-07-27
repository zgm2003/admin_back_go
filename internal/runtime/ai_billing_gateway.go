package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/billing"
	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/module/ai/replycommand"
	airun "admin_back_go/internal/module/ai/run"
	aitext "admin_back_go/internal/module/ai/text"
	walletmodule "admin_back_go/internal/module/payment/wallet"
	modulerealtime "admin_back_go/internal/module/realtime"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type gormBillingTransaction struct{ db *gorm.DB }

// errPersistedPaidAttemptEvidence marks corrupt durable request evidence. It
// is never safe to retry by dispatching an altered request.
var errPersistedPaidAttemptEvidence = errors.New("persisted paid provider attempt evidence is invalid")

func (gormBillingTransaction) BillingTx() {}

func gatewayTransactionDB(tx aigateway.Transaction) (*gorm.DB, error) {
	value, ok := tx.(gormBillingTransaction)
	if !ok || value.db == nil {
		return nil, errors.New("AI billing transaction is invalid")
	}
	return value.db, nil
}

type gormGatewayTransactions struct{ db *gorm.DB }

func (runner gormGatewayTransactions) WithinTransaction(ctx context.Context, fn func(aigateway.Transaction) error) error {
	if runner.db == nil || fn == nil {
		return aigateway.ErrNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return runner.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(gormBillingTransaction{db: tx})
	})
}

type paidChatAttemptExecutor struct {
	db        *gorm.DB
	wallets   *walletmodule.GormRepository
	replies   *replycommand.GormRepository
	eventSink modulerealtime.TransactionalEventSink
	finalizer aigateway.Finalizer
}

func newPaidChatAttemptExecutor(client *database.Client, wallets *walletmodule.GormRepository, replies *replycommand.GormRepository, eventSink modulerealtime.TransactionalEventSink) *paidChatAttemptExecutor {
	if client == nil || client.Gorm == nil || wallets == nil || replies == nil {
		return nil
	}
	store := newGormGatewayFinalizationStore(client.Gorm, wallets, replies, eventSink)
	if store == nil {
		return nil
	}
	return &paidChatAttemptExecutor{db: client.Gorm, wallets: wallets, replies: replies, eventSink: eventSink, finalizer: aigateway.NewFinalizer(store, persistedSettlementPricer{})}
}

func (executor *paidChatAttemptExecutor) ExecutePaidChatAttempt(ctx context.Context, input aichat.PaidChatAttemptInput) (*aichat.PaidChatAttemptResult, error) {
	if executor == nil || executor.db == nil || executor.wallets == nil || executor.replies == nil || executor.finalizer == nil {
		return nil, aigateway.ErrNotConfigured
	}
	if executor.finalizationRetryPending(ctx, input) {
		return executor.finalizePaidAttempt(context.WithoutCancel(ctx), input, nil)
	}
	attemptNo, recoverPrepared, replayFinalization, recoveredToolResult, err := executor.nextAttempt(ctx, input.RunID, input.CommandID, input.ChatInput)
	if err != nil {
		return nil, err
	}
	if replayFinalization {
		return executor.finalizePaidAttempt(context.WithoutCancel(ctx), input, nil)
	}
	if recoveredToolResult != nil {
		return &aichat.PaidChatAttemptResult{ChatResult: recoveredToolResult}, nil
	}
	if providerAttemptLimitExceeded(attemptNo, input.CommandMaxAttempts) {
		return executor.FinalizePaidChatLocalFailure(context.WithoutCancel(ctx), input)
	}
	transport, ok := input.Engine.(aigateway.PreparedChatTransport)
	if !ok {
		return executor.finalizePreDispatchFailure(context.WithoutCancel(ctx), input)
	}
	if userStopped(input.DeliveryContext) {
		return executor.finalizePaidAttempt(context.WithoutCancel(ctx), input, nil)
	}
	provider := aigateway.NewPreparedChatProvider(transport, input.Sink, aichat.MarshalChatResultCandidate)
	provider.SetStopProbe(func() bool { return userStopped(input.DeliveryContext) })
	runs := gormGatewayRunStore{db: executor.db}
	attempts := gormGatewayAttemptStore{
		db: executor.db, repository: executor.replies,
		commandID: input.CommandID, owner: input.LeaseOwner, token: input.LeaseToken,
	}
	dependencies := aigateway.Dependencies{
		Assembler:    paidChatAssembler{transport: transport, input: input.ChatInput},
		Quotes:       aigateway.PersistedQuoteValidator{},
		Transactions: gormGatewayTransactions{db: executor.db},
		Runs:         runs,
		PriorUsage:   gormGatewayPriorUsagePricer{},
		Reserve:      gormGatewayReserveParticipant{wallets: executor.wallets},
		Failures:     gormGatewayReserveFailureRecorder{commandID: input.CommandID, owner: input.LeaseOwner, token: input.LeaseToken},
		Attempts:     attempts,
		Provider:     provider,
		Owner: gormGatewayOwnerGuard{
			commandID: input.CommandID, owner: input.LeaseOwner, token: input.LeaseToken, now: time.Now,
		},
	}
	gateway := aigateway.New(dependencies)
	var attempt aigateway.ProviderAttempt
	if recoverPrepared {
		attempt, err = gateway.ReserveAndPrepare(ctx, aigateway.ReserveAndPrepareInput{RunID: input.RunID, AttemptNo: attemptNo})
	} else {
		call, assembleErr := gateway.AssembleAndQuote(ctx, aigateway.RunRequest{
			UserID: input.RequestIdentity.UserID, RunID: input.RunID, RequestID: input.RequestID, Identity: input.RequestIdentity,
		})
		if assembleErr != nil {
			if isPermanentPreDispatchError(assembleErr) {
				return executor.finalizePreDispatchFailure(context.WithoutCancel(ctx), input)
			}
			return nil, mapPaidGatewayError(assembleErr)
		}
		attempt, err = gateway.ReserveAndPrepare(ctx, aigateway.ReserveAndPrepareInput{RunID: input.RunID, AttemptNo: attemptNo, NewCall: &call})
	}
	if err != nil {
		if isPaidInsufficientBalance(err) {
			return executor.finalizePaidAttempt(context.WithoutCancel(ctx), input, nil)
		}
		if isPermanentPreDispatchError(err) {
			return executor.finalizePreDispatchFailure(context.WithoutCancel(ctx), input)
		}
		return nil, mapPaidGatewayError(err)
	}
	if userStopped(input.DeliveryContext) {
		return executor.finalizePaidAttempt(context.WithoutCancel(ctx), input, nil)
	}
	dispatch, dispatchErr := gateway.Dispatch(ctx, attempt)
	if dispatchErr != nil {
		if executor.mustFinalizeProviderError(input, attempt, dispatch, dispatchErr) {
			if dispatch.TerminalState == "failed" {
				if markerErr := executor.markProviderFailure(context.WithoutCancel(ctx), input); markerErr != nil {
					return nil, errors.Join(dispatchErr, markerErr)
				}
			}
			return executor.finalizePaidAttempt(context.WithoutCancel(ctx), input, nil)
		}
		return nil, mapPaidGatewayError(dispatchErr)
	}
	result := provider.ChatResult()
	if result == nil {
		return nil, errors.New("paid AI Gateway returned no chat result")
	}
	if len(result.ToolCalls) == 0 || !result.Usage.Complete() || userStopped(input.DeliveryContext) {
		finalized, err := executor.finalizePaidAttempt(context.WithoutCancel(ctx), input, result)
		if err != nil {
			return nil, err
		}
		return finalized, nil
	}
	return &aichat.PaidChatAttemptResult{ChatResult: result}, nil
}

func providerAttemptLimitExceeded(attemptNo uint32, maxAttempts uint) bool {
	return maxAttempts > 0 && uint(attemptNo) > maxAttempts
}

func (executor *paidChatAttemptExecutor) FinalizePaidChatAttempt(ctx context.Context, input aichat.PaidChatAttemptInput) (*aichat.PaidChatAttemptResult, error) {
	return executor.finalizePaidAttempt(ctx, input, nil)
}

func (executor *paidChatAttemptExecutor) FinalizePaidChatPreDispatchFailure(ctx context.Context, input aichat.PaidChatAttemptInput) (*aichat.PaidChatAttemptResult, error) {
	return executor.finalizePreDispatchFailure(ctx, input)
}

func (executor *paidChatAttemptExecutor) FinalizePaidChatLocalFailure(ctx context.Context, input aichat.PaidChatAttemptInput) (*aichat.PaidChatAttemptResult, error) {
	if err := executor.markFinalizationTrigger(ctx, input, "ai.local_failed", aigateway.TriggerLocalFailure); err != nil {
		return nil, err
	}
	return executor.finalizePaidAttempt(ctx, input, nil)
}

func (executor *paidChatAttemptExecutor) FinalizeOutcomeUnknown(ctx context.Context, commandID uint64) error {
	if executor == nil || executor.db == nil || executor.finalizer == nil || commandID == 0 {
		return aigateway.ErrNotConfigured
	}
	var command replycommand.Command
	if err := executor.db.WithContext(ctx).Where("id = ? AND state = ?", commandID, replycommand.StateOutcomeUnknown).First(&command).Error; err != nil {
		return err
	}
	var run airun.Run
	if err := executor.db.WithContext(ctx).Where("user_id = ? AND request_id = ?", command.UserID, command.RequestID).First(&run).Error; err != nil {
		return err
	}
	return executor.finalizer.Finalize(ctx, aigateway.FinalizeRequest{RunID: run.ID})
}

func (executor *paidChatAttemptExecutor) finalizePreDispatchFailure(ctx context.Context, input aichat.PaidChatAttemptInput) (*aichat.PaidChatAttemptResult, error) {
	if err := executor.markFinalizationTrigger(ctx, input, "ai.provider_pre_dispatch_failed", aigateway.TriggerPreDispatchFailed); err != nil {
		return nil, err
	}
	return executor.finalizePaidAttempt(ctx, input, nil)
}

func (executor *paidChatAttemptExecutor) markFinalizationTrigger(ctx context.Context, input aichat.PaidChatAttemptInput, code string, trigger aigateway.FinalizationTrigger) error {
	result := executor.db.WithContext(ctx).Model(&replycommand.Command{}).
		Where("id = ? AND state = ? AND lease_owner = ? AND lease_token = ?", input.CommandID, replycommand.StateRunning, input.LeaseOwner, input.LeaseToken).
		Updates(map[string]any{"last_error_code": code, "last_error_message": string(trigger), "updated_at": time.Now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return replycommand.ErrLeaseLost
	}
	return nil
}

func (executor *paidChatAttemptExecutor) finalizePaidAttempt(ctx context.Context, input aichat.PaidChatAttemptInput, result *infraai.ChatResult) (*aichat.PaidChatAttemptResult, error) {
	if executor == nil || executor.finalizer == nil || input.RunID <= 0 || input.CommandID == 0 {
		return nil, aigateway.ErrNotConfigured
	}
	if err := executor.promoteInvalidFinalCandidate(ctx, input); err != nil {
		return nil, err
	}
	if err := executor.finalizer.Finalize(ctx, aigateway.FinalizeRequest{RunID: input.RunID}); err != nil {
		if errors.Is(err, aigateway.ErrFinalizationPending) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", aichat.ErrPaidFinalizationRetry, err)
	}
	var command replycommand.Command
	if err := executor.db.WithContext(ctx).Where("id = ?", input.CommandID).First(&command).Error; err != nil {
		return nil, err
	}
	assistantID := int64(0)
	if command.AssistantMessageID != nil {
		assistantID = *command.AssistantMessageID
	}
	return &aichat.PaidChatAttemptResult{ChatResult: result, Finalized: true, AssistantMessageID: assistantID}, nil
}

func (executor *paidChatAttemptExecutor) finalizationRetryPending(ctx context.Context, input aichat.PaidChatAttemptInput) bool {
	if executor == nil || executor.db == nil || input.CommandID == 0 {
		return false
	}
	var command replycommand.Command
	if err := executor.db.WithContext(ctx).Select("last_error_code", "last_error_message").Where("id = ?", input.CommandID).First(&command).Error; err != nil {
		return false
	}
	if command.IsGenericFinalizationRetry() {
		return false
	}
	code, marker := strings.TrimSpace(command.LastErrorCode), aigateway.FinalizationTrigger(strings.TrimSpace(command.LastErrorMessage))
	switch code {
	case "ai.provider_failed":
		return marker == aigateway.TriggerProviderFailed
	case "ai.local_failed":
		return marker == aigateway.TriggerLocalFailure
	case "ai.provider_pre_dispatch_failed":
		return marker == aigateway.TriggerPreDispatchFailed
	case aigateway.ErrCodeInsufficientBalance:
		return marker == aigateway.TriggerInitialInsufficient || marker == aigateway.TriggerContinuationTopUpInsufficient
	default:
		return false
	}
}

func (executor *paidChatAttemptExecutor) promoteInvalidFinalCandidate(ctx context.Context, input aichat.PaidChatAttemptInput) error {
	var attempt replycommand.Attempt
	err := executor.db.WithContext(ctx).Where("run_id = ? AND command_id = ?", input.RunID, input.CommandID).Order("attempt_no DESC").First(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil || attempt.State != replycommand.AttemptSucceeded {
		return err
	}
	if !terminalCandidateRequiresLocalFailure(attempt) {
		return nil
	}
	return executor.markFinalizationTrigger(ctx, input, "ai.local_failed", aigateway.TriggerLocalFailure)
}

func terminalCandidateRequiresLocalFailure(attempt replycommand.Attempt) bool {
	usage, err := usageFromAttempt(attempt)
	if err != nil || !usage.Complete() {
		return false
	}
	if attempt.ResultCandidateJSON != nil {
		if _, err := aichat.FinalChatAnswerFromCandidate(*attempt.ResultCandidateJSON); err == nil {
			return false
		}
		if isToolCall, err := aichat.IsToolCallCandidate(*attempt.ResultCandidateJSON); err == nil && isToolCall {
			return false
		}
	}
	return true
}

func (executor *paidChatAttemptExecutor) mustFinalizeProviderError(input aichat.PaidChatAttemptInput, attempt aigateway.ProviderAttempt, dispatch aigateway.DispatchResult, err error) bool {
	if dispatch.TerminalState == "outcome_unknown" {
		return true
	}
	if input.CommandMaxAttempts > 0 && uint(attempt.AttemptNo) >= input.CommandMaxAttempts {
		return true
	}
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		return !appErr.Retryable()
	}
	if errors.Is(err, infraai.ErrRateLimited) || errors.Is(err, infraai.ErrUpstreamTimeout) || errors.Is(err, infraai.ErrUpstreamFailed) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, infraai.ErrUnauthorized) || errors.Is(err, infraai.ErrInvalidConfig) || errors.Is(err, infraai.ErrEngineDisabled) {
		return true
	}
	if outcome, ok := infraai.ProviderOutcomeFromError(err); ok {
		return outcome == infraai.ProviderOutcomeRejected || outcome == infraai.ProviderOutcomeNotDispatched
	}
	return false
}

func (executor *paidChatAttemptExecutor) markProviderFailure(ctx context.Context, input aichat.PaidChatAttemptInput) error {
	result := executor.db.WithContext(ctx).Model(&replycommand.Command{}).
		Where("id = ? AND state = ? AND lease_owner = ? AND lease_token = ?", input.CommandID, replycommand.StateRunning, input.LeaseOwner, input.LeaseToken).
		Updates(map[string]any{"last_error_code": "ai.provider_failed", "last_error_message": string(aigateway.TriggerProviderFailed), "updated_at": time.Now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return replycommand.ErrLeaseLost
	}
	return nil
}

func isPaidInsufficientBalance(err error) bool {
	var gatewayErr *aigateway.Error
	return errors.As(err, &gatewayErr) && gatewayErr.Code == aigateway.ErrCodeInsufficientBalance
}

func isPermanentPreDispatchError(err error) bool {
	if errors.Is(err, errPersistedPaidAttemptEvidence) {
		return true
	}
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		return !appErr.Retryable()
	}
	var gatewayErr *aigateway.Error
	return errors.As(err, &gatewayErr)
}

func userStopped(ctx context.Context) bool {
	return ctx != nil && errors.Is(context.Cause(ctx), infraai.ErrCanceled)
}

func (executor *paidChatAttemptExecutor) nextAttempt(ctx context.Context, runID int64, commandID uint64, chatInput infraai.ChatInput) (uint32, bool, bool, *infraai.ChatResult, error) {
	if runID <= 0 || commandID == 0 {
		return 0, false, false, nil, errors.New("paid AI attempt identity is invalid")
	}
	var latest replycommand.Attempt
	err := executor.db.WithContext(ctx).
		Where("run_id = ? AND command_id = ?", runID, commandID).
		Order("attempt_no DESC").
		First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 1, false, false, nil, nil
	}
	if err != nil {
		return 0, false, false, nil, err
	}
	if latest.AttemptNo == 0 || uint64(latest.AttemptNo) >= uint64(math.MaxUint32) {
		return 0, false, false, nil, errors.New("AI provider attempt number is exhausted")
	}
	switch latest.State {
	case replycommand.AttemptPrepared:
		return uint32(latest.AttemptNo), true, false, nil, nil
	case replycommand.AttemptSucceeded:
		if replayFinalizationForTerminalAttempt(latest) {
			return 0, false, true, nil, nil
		}
		if len(chatInput.ToolCalls) == 0 && len(chatInput.ToolOutputs) == 0 {
			result, err := aichat.ChatResultFromCandidate(*latest.ResultCandidateJSON)
			if err != nil {
				return 0, false, false, nil, err
			}
			usage, err := usageFromAttempt(latest)
			if err != nil {
				return 0, false, false, nil, err
			}
			prompt, completion, total, err := chatResultUsageTotals(usage)
			if err != nil {
				return 0, false, false, nil, err
			}
			result.Usage = usage
			result.UsageStatus = infraai.UsageStatusReported
			result.PromptTokens = prompt
			result.CompletionTokens = completion
			result.TotalTokens = total
			return 0, false, false, result, nil
		}
		return uint32(latest.AttemptNo + 1), false, false, nil, nil
	case replycommand.AttemptFailed:
		return uint32(latest.AttemptNo + 1), false, false, nil, nil
	case replycommand.AttemptDispatched, replycommand.AttemptOutcomeUnknown:
		if latest.State == replycommand.AttemptOutcomeUnknown {
			return 0, false, true, nil, nil
		}
		return 0, false, false, nil, infraai.NewProviderError(infraai.ProviderOutcomeUnknown, latest.ProviderRequestID, errors.New("paid provider attempt has no safely replayable terminal boundary"))
	case replycommand.AttemptCanceled:
		return 0, false, true, nil, nil
	default:
		return 0, false, false, nil, fmt.Errorf("unknown paid provider attempt state %q", latest.State)
	}
}

func chatResultUsageTotals(usage infraai.UsageSnapshot) (int, int, int, error) {
	if !usage.Complete() {
		return 0, 0, 0, errors.New("persisted tool candidate usage is incomplete")
	}
	maxInt := int64(^uint(0) >> 1)
	var prompt, completion int64
	for _, item := range usage.Items {
		switch item.Category {
		case infraai.UsageCategoryInput, infraai.UsageCategoryCacheRead, infraai.UsageCategoryCacheWrite:
			if item.Quantity > maxInt-prompt {
				return 0, 0, 0, errors.New("persisted tool candidate prompt usage overflows")
			}
			prompt += item.Quantity
		case infraai.UsageCategoryOutput:
			if item.Quantity > maxInt-completion {
				return 0, 0, 0, errors.New("persisted tool candidate completion usage overflows")
			}
			completion += item.Quantity
		}
	}
	if prompt > maxInt-completion {
		return 0, 0, 0, errors.New("persisted tool candidate total usage overflows")
	}
	return int(prompt), int(completion), int(prompt + completion), nil
}

// replayFinalizationForTerminalAttempt fences recovery at the persisted
// terminal boundary. A final-answer candidate never creates another provider
// request after an interrupted finalization; tool candidates may continue.
func replayFinalizationForTerminalAttempt(attempt replycommand.Attempt) bool {
	if attempt.State != replycommand.AttemptSucceeded || attempt.ResultCandidateJSON == nil {
		return true
	}
	usage, err := usageFromAttempt(attempt)
	if err != nil || !usage.Complete() {
		return true
	}
	if _, err := aichat.FinalChatAnswerFromCandidate(*attempt.ResultCandidateJSON); err == nil {
		return true
	}
	isToolCall, err := aichat.IsToolCallCandidate(*attempt.ResultCandidateJSON)
	return err != nil || !isToolCall
}

func mapPaidGatewayError(err error) error {
	if err == nil {
		return nil
	}
	var gatewayErr *aigateway.Error
	if errors.As(err, &gatewayErr) {
		category := apperror.CategoryInternal
		status := gatewayErr.Status
		if status == 0 {
			status = http.StatusInternalServerError
		}
		if gatewayErr.Code == aigateway.ErrCodeInsufficientBalance || status == http.StatusConflict {
			category = apperror.CategoryConflict
		}
		return apperror.Wrap(gatewayErr.Code, category, status, apperror.Permanent, "", nil, gatewayErr.Message, err)
	}
	return err
}

type paidChatAssembler struct {
	transport aigateway.PreparedChatTransport
	input     infraai.ChatInput
}

func (assembler paidChatAssembler) AssembleAndQuote(ctx context.Context, run aigateway.RunSnapshot, _ aigateway.RunRequest) (aigateway.PreparedCall, error) {
	if assembler.transport == nil {
		return aigateway.PreparedCall{}, aigateway.ErrNotConfigured
	}
	snapshot, err := aigateway.ParsePricingSnapshot(run.PricingSnapshotJSON)
	if err != nil {
		return aigateway.PreparedCall{}, err
	}
	chatInput := clonePaidChatInput(assembler.input)
	if chatInput.Inputs == nil {
		chatInput.Inputs = map[string]any{}
	}
	chatInput.Inputs["model_id"] = snapshot.RequestedModelID
	chatInput.Inputs["max_tokens"] = snapshot.EffectiveMaxOutputTokens
	body, err := assembler.transport.PrepareChat(ctx, chatInput)
	if err != nil {
		return aigateway.PreparedCall{}, err
	}
	inputBound, err := infraai.SafeInputUpperBoundFromRequest(body)
	if err != nil {
		return aigateway.PreparedCall{}, err
	}
	items := []billing.UsageItem{
		{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: inputBound},
		{Category: billing.UsageCategoryOutputText, Unit: "token", Quantity: int64(snapshot.EffectiveMaxOutputTokens)},
	}
	quoted, err := quotePricingSnapshot(snapshot, items, "upper-bound")
	if err != nil {
		return aigateway.PreparedCall{}, err
	}
	if quoted.AmountUnits <= 0 {
		return aigateway.PreparedCall{}, errors.New("AI request upper-bound price must be positive")
	}
	return aigateway.PreparedCall{
		RequestBody:   append([]byte(nil), body...),
		RequestSHA256: sha256.Sum256(body),
		Quote: aigateway.QuoteEvidence{
			PricingVersion: snapshot.Version, EffectiveMaxOutputTokens: snapshot.EffectiveMaxOutputTokens,
			UpperBoundItems: items, CurrentCallMaxUnits: quoted.AmountUnits, TargetHoldUnits: quoted.AmountUnits,
		},
	}, nil
}

func clonePaidChatInput(input infraai.ChatInput) infraai.ChatInput {
	copy := input
	copy.Inputs = make(map[string]any, len(input.Inputs)+2)
	for key, value := range input.Inputs {
		copy.Inputs[key] = value
	}
	copy.Tools = append([]infraai.ToolDefinition(nil), input.Tools...)
	copy.ToolCalls = append([]infraai.ToolCall(nil), input.ToolCalls...)
	copy.ToolOutputs = append([]infraai.ToolOutput(nil), input.ToolOutputs...)
	return copy
}

func pricingModelFromSnapshot(snapshot aigateway.PricingSnapshot) pricing.ModelPrice {
	aliases := []string(nil)
	if snapshot.RequestedModelID != snapshot.CanonicalModelID {
		aliases = []string{snapshot.RequestedModelID}
	}
	return pricing.ModelPrice{
		Version: snapshot.Version, CatalogVersion: snapshot.CatalogVersion, OverrideVersion: snapshot.OverrideVersion,
		CatalogVendor: snapshot.CatalogVendor, ModelID: snapshot.CanonicalModelID, Aliases: aliases,
		MaxOutputTokens: snapshot.CatalogMaxOutputTokens, ContextTierThresholdTokens: snapshot.ContextTierThresholdTokens,
		PriceSource: snapshot.PriceSource, SourceURL: snapshot.SourceURL, RetrievedAt: snapshot.RetrievedAt,
		Rates: append([]pricing.Rate(nil), snapshot.Rates...),
	}
}

func quotePricingSnapshot(snapshot aigateway.PricingSnapshot, items []billing.UsageItem, keyPrefix string) (pricing.QuoteResult, error) {
	lines := make([]pricing.QuoteLine, len(items))
	for index, item := range items {
		lines[index] = pricing.QuoteLine{Key: keyPrefix + "-" + strconv.Itoa(index), Item: item}
	}
	model := pricingModelFromSnapshot(snapshot)
	selected, err := pricing.UpperBoundLines(model, lines)
	if err != nil {
		return pricing.QuoteResult{}, err
	}
	return pricing.Quote(model, selected, snapshot.MultiplierPPM)
}

type gormGatewayRunStore struct{ db *gorm.DB }

func (store gormGatewayRunStore) LoadRun(ctx context.Context, runID int64) (aigateway.RunSnapshot, error) {
	if store.db == nil || runID <= 0 {
		return aigateway.RunSnapshot{}, aigateway.ErrNotConfigured
	}
	var row airun.Run
	if err := store.db.WithContext(ctx).Where("id = ?", runID).First(&row).Error; err != nil {
		return aigateway.RunSnapshot{}, err
	}
	return gatewayRunSnapshot(row)
}

func (store gormGatewayRunStore) LockRunAndCharge(ctx context.Context, transaction aigateway.Transaction, runID int64) (aigateway.LockedRunCharge, error) {
	tx, err := gatewayTransactionDB(transaction)
	if err != nil {
		return aigateway.LockedRunCharge{}, err
	}
	var run airun.Run
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", runID).First(&run).Error; err != nil {
		return aigateway.LockedRunCharge{}, err
	}
	snapshot, err := gatewayRunSnapshot(run)
	if err != nil {
		return aigateway.LockedRunCharge{}, err
	}
	var charge billing.UsageCharge
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ?", runID).First(&charge).Error; err != nil {
		return aigateway.LockedRunCharge{}, err
	}
	if charge.UserID != run.UserID || charge.Status != billing.ChargeStatusOpen || charge.HeldUnits < 0 || charge.ActualUnits != 0 {
		return aigateway.LockedRunCharge{}, errors.New("AI Run and open usage charge are inconsistent")
	}
	if snapshot.BillingStatus != billing.BillingStatusPending && snapshot.BillingStatus != billing.BillingStatusHeld {
		return aigateway.LockedRunCharge{}, errors.New("AI Run is not open for billing")
	}
	return aigateway.LockedRunCharge{Run: snapshot, ChargeHeldAuditMax: charge.HeldUnits, HoldTargetUnits: charge.HeldUnits}, nil
}

func gatewayRunSnapshot(row airun.Run) (aigateway.RunSnapshot, error) {
	if row.ID <= 0 || row.UserID <= 0 || strings.TrimSpace(row.RequestID) == "" || len(row.RequestFingerprint) != sha256.Size || row.Status != enum.AIRunStatusRunning {
		return aigateway.RunSnapshot{}, errors.New("AI Run billing snapshot is incomplete")
	}
	var fingerprint [sha256.Size]byte
	copy(fingerprint[:], row.RequestFingerprint)
	return aigateway.RunSnapshot{
		RunID: row.ID, UserID: row.UserID, RequestID: strings.TrimSpace(row.RequestID), RequestFingerprint: fingerprint,
		PricingSnapshotJSON: strings.TrimSpace(row.PricingSnapshotJSON), BillingStatus: billing.BillingStatus(row.BillingStatus),
		BillingReason: billing.BillingReason(row.BillingReason), AgentID: row.AgentID, ModelID: strings.TrimSpace(row.ModelID),
		ModelDisplayName: strings.TrimSpace(row.ModelDisplayName),
	}, nil
}

type gormGatewayPriorUsagePricer struct{}

func (gormGatewayPriorUsagePricer) PricePriorSucceededUsage(ctx context.Context, transaction aigateway.Transaction, run aigateway.RunSnapshot, beforeAttemptNo uint32) (int64, error) {
	tx, err := gatewayTransactionDB(transaction)
	if err != nil {
		return 0, err
	}
	if beforeAttemptNo == 0 {
		return 0, errors.New("prior usage boundary is required")
	}
	var rows []replycommand.Attempt
	if err := tx.WithContext(ctx).
		Where("run_id = ? AND attempt_no < ? AND state = ?", run.RunID, beforeAttemptNo, replycommand.AttemptSucceeded).
		Order("attempt_no ASC").Find(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	snapshot, err := aigateway.ParsePricingSnapshot(run.PricingSnapshotJSON)
	if err != nil {
		return 0, err
	}
	lines := make([]pricing.QuoteLine, 0, len(rows)*2)
	for _, row := range rows {
		if _, err := paidQuoteFromAttempt(row); err != nil {
			return 0, aigateway.ErrUsageIncomplete
		}
		usage, err := usageFromAttempt(row)
		if err != nil || !completeRawUsage(usage) {
			return 0, aigateway.ErrUsageIncomplete
		}
		for index, item := range billingItemsFromProviderUsage(usage) {
			lines = append(lines, pricing.QuoteLine{
				Key:       fmt.Sprintf("attempt-%d-%d", row.ID, index),
				AttemptID: strconv.FormatUint(row.ID, 10),
				Item:      item,
			})
		}
	}
	model := pricingModelFromSnapshot(snapshot)
	selected, err := pricing.SettlementLines(model, lines)
	if err != nil {
		return 0, errors.Join(aigateway.ErrUsageIncomplete, err)
	}
	quote, err := pricing.Quote(model, selected, snapshot.MultiplierPPM)
	if err != nil {
		return 0, errors.Join(aigateway.ErrUsageIncomplete, err)
	}
	return quote.AmountUnits, nil
}

type persistedSettlementPricer struct{}

func (persistedSettlementPricer) PriceSettlement(ctx context.Context, input aigateway.SettlementPricingInput) (aigateway.SettlementQuote, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return aigateway.SettlementQuote{}, err
		}
	}
	snapshot, err := aigateway.ParsePricingSnapshot(input.Run.PricingSnapshotJSON)
	if err != nil {
		return aigateway.SettlementQuote{}, err
	}
	lines := make([]pricing.QuoteLine, 0, len(input.Attempts)*2)
	type itemIdentity struct {
		attemptID int64
		item      billing.UsageItem
	}
	identities := make(map[string]itemIdentity, len(input.Attempts)*2)
	for _, attempt := range input.Attempts {
		if attempt.ID <= 0 || !completeRawUsage(attempt.Usage) {
			return aigateway.SettlementQuote{}, aigateway.ErrUsageIncomplete
		}
		for index, item := range billingItemsFromProviderUsage(attempt.Usage) {
			if err := item.Validate(); err != nil {
				return aigateway.SettlementQuote{}, errors.Join(aigateway.ErrUsageIncomplete, err)
			}
			key := fmt.Sprintf("attempt-%d-%d", attempt.ID, index)
			lines = append(lines, pricing.QuoteLine{Key: key, AttemptID: strconv.FormatInt(attempt.ID, 10), Item: item})
			identities[key] = itemIdentity{attemptID: attempt.ID, item: item}
		}
	}
	model := pricingModelFromSnapshot(snapshot)
	selected, err := pricing.SettlementLines(model, lines)
	if err != nil {
		return aigateway.SettlementQuote{}, errors.Join(aigateway.ErrUsageIncomplete, err)
	}
	quote, err := pricing.Quote(model, selected, snapshot.MultiplierPPM)
	if err != nil {
		return aigateway.SettlementQuote{}, errors.Join(aigateway.ErrUsageIncomplete, err)
	}
	items := make([]billing.UsageChargeItem, 0, len(quote.Lines))
	for _, line := range quote.Lines {
		identity, ok := identities[line.Key]
		if !ok {
			return aigateway.SettlementQuote{}, errors.New("settlement allocation lost its usage identity")
		}
		items = append(items, billing.UsageChargeItem{
			AttemptID: identity.attemptID, Category: identity.item.Category, TierKey: line.Rate.TierKey,
			Quantity: identity.item.Quantity, Unit: identity.item.Unit, UnitPriceUnits: line.Rate.PriceUnits,
			UnitScale: line.Rate.UnitScale, AmountUnits: line.AmountUnits,
		})
	}
	return aigateway.SettlementQuote{ActualUnits: quote.AmountUnits, Items: items}, nil
}

func completeRawUsage(usage infraai.UsageSnapshot) bool {
	return usage.Complete() && len(usage.RawProviderJSON) > 0 && usage.ResponseSHA256 != ([32]byte{}) && sha256.Sum256(usage.RawProviderJSON) == usage.ResponseSHA256
}

func billingItemsFromProviderUsage(usage infraai.UsageSnapshot) []billing.UsageItem {
	items := make([]billing.UsageItem, 0, len(usage.Items))
	for _, item := range usage.Items {
		items = append(items, billing.UsageItem{
			Category: billing.UsageCategory(item.Category), Unit: strings.TrimSpace(item.Unit),
			TierKey: strings.TrimSpace(item.TierKey), Quantity: item.Quantity,
		})
	}
	return items
}

func usageFromAttempt(row replycommand.Attempt) (infraai.UsageSnapshot, error) {
	var usage infraai.UsageSnapshot
	if strings.TrimSpace(row.UsageJSON) == "" || json.Unmarshal([]byte(row.UsageJSON), &usage) != nil {
		return infraai.UsageSnapshot{}, errors.New("provider attempt usage is invalid")
	}
	if err := usage.Validate(); err != nil {
		return infraai.UsageSnapshot{}, err
	}
	return usage, nil
}

func paidQuoteFromAttempt(row replycommand.Attempt) (aigateway.QuoteEvidence, error) {
	var quote aigateway.QuoteEvidence
	if strings.TrimSpace(row.QuoteJSON) == "" || json.Unmarshal([]byte(row.QuoteJSON), &quote) != nil || quote.PricingVersion == "" || quote.CurrentCallMaxUnits <= 0 {
		return aigateway.QuoteEvidence{}, errors.New("provider attempt is not paid quote evidence")
	}
	return quote, nil
}

type gormGatewayReserveParticipant struct{ wallets *walletmodule.GormRepository }

func (participant gormGatewayReserveParticipant) ReserveOrTopUp(ctx context.Context, transaction aigateway.Transaction, runID, target int64) (aigateway.LockedBillingFacts, error) {
	tx, err := gatewayTransactionDB(transaction)
	if err != nil {
		return aigateway.LockedBillingFacts{}, err
	}
	if participant.wallets == nil || runID <= 0 || target <= 0 {
		return aigateway.LockedBillingFacts{}, aigateway.ErrNotConfigured
	}
	var run airun.Run
	if err := tx.WithContext(ctx).Select("id", "user_id", "billing_status", "billing_reason").Where("id = ?", runID).First(&run).Error; err != nil {
		return aigateway.LockedBillingFacts{}, err
	}
	hold, err := participant.wallets.ReserveHoldInTx(ctx, tx, walletmodule.ReserveHoldInput{UserID: run.UserID, RunID: runID, AmountUnits: target})
	if err != nil {
		return aigateway.LockedBillingFacts{}, err
	}
	if hold == nil || hold.Status != walletmodule.HoldActive || hold.HeldUnits != target {
		return aigateway.LockedBillingFacts{}, walletmodule.ErrHoldIntegrity
	}
	result := tx.WithContext(ctx).Model(&airun.Run{}).
		Where("id = ? AND billing_status IN ?", runID, []string{string(billing.BillingStatusPending), string(billing.BillingStatusHeld)}).
		Updates(map[string]any{"billing_status": billing.BillingStatusHeld, "billing_reason": billing.BillingReasonHeld, "updated_at": time.Now()})
	if result.Error != nil || result.RowsAffected != 1 {
		if result.Error != nil {
			return aigateway.LockedBillingFacts{}, result.Error
		}
		return aigateway.LockedBillingFacts{}, errors.New("AI Run hold transition was rejected")
	}
	result = tx.WithContext(ctx).Model(&billing.UsageCharge{}).
		Where("run_id = ? AND status = ?", runID, billing.ChargeStatusOpen).
		Update("held_units", gorm.Expr("GREATEST(held_units, ?)", target))
	if result.Error != nil || result.RowsAffected != 1 {
		if result.Error != nil {
			return aigateway.LockedBillingFacts{}, result.Error
		}
		return aigateway.LockedBillingFacts{}, errors.New("AI usage charge hold transition was rejected")
	}
	return lockedBillingFacts(ctx, tx, runID, target)
}

func (participant gormGatewayReserveParticipant) EnsureActiveHold(ctx context.Context, transaction aigateway.Transaction, runID, target int64) (aigateway.LockedBillingFacts, error) {
	tx, err := gatewayTransactionDB(transaction)
	if err != nil {
		return aigateway.LockedBillingFacts{}, err
	}
	if participant.wallets == nil || runID <= 0 || target <= 0 {
		return aigateway.LockedBillingFacts{}, aigateway.ErrNotConfigured
	}
	var run airun.Run
	if err := tx.WithContext(ctx).Select("id", "user_id").Where("id = ?", runID).First(&run).Error; err != nil {
		return aigateway.LockedBillingFacts{}, err
	}
	var ownerWallet walletmodule.Wallet
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND is_del = ?", run.UserID, enum.CommonNo).First(&ownerWallet).Error; err != nil {
		return aigateway.LockedBillingFacts{}, err
	}
	var hold walletmodule.Hold
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ?", runID).First(&hold).Error; err != nil {
		return aigateway.LockedBillingFacts{}, err
	}
	if hold.WalletID != ownerWallet.ID || hold.UserID != run.UserID || hold.Status != walletmodule.HoldActive || hold.HeldUnits != target || hold.CapturedUnits != 0 || ownerWallet.HeldUnits < hold.HeldUnits || ownerWallet.BalanceUnits < ownerWallet.HeldUnits {
		return aigateway.LockedBillingFacts{}, walletmodule.ErrHoldIntegrity
	}
	return lockedBillingFacts(ctx, tx, runID, target)
}

func lockedBillingFacts(ctx context.Context, tx *gorm.DB, runID, target int64) (aigateway.LockedBillingFacts, error) {
	var charge billing.UsageCharge
	if err := tx.WithContext(ctx).Where("run_id = ? AND status = ?", runID, billing.ChargeStatusOpen).First(&charge).Error; err != nil {
		return aigateway.LockedBillingFacts{}, err
	}
	if charge.HeldUnits != target || charge.HeldUnits < 0 {
		return aigateway.LockedBillingFacts{}, errors.New("AI charge and active Hold target differ")
	}
	return aigateway.LockedBillingFacts{
		RunID: runID, ChargeHeldUnits: charge.HeldUnits, ChargeHeldAuditMax: charge.HeldUnits,
		HoldTargetUnits: target, HoldActive: true,
	}, nil
}

type gormGatewayReserveFailureRecorder struct {
	commandID uint64
	owner     string
	token     uint64
}

func (g gormGatewayReserveFailureRecorder) RecordReserveFailure(ctx context.Context, transaction aigateway.Transaction, runID int64, trigger aigateway.FinalizationTrigger) error {
	tx, err := gatewayTransactionDB(transaction)
	if err != nil {
		return err
	}
	if trigger != aigateway.TriggerInitialInsufficient && trigger != aigateway.TriggerContinuationTopUpInsufficient {
		return errors.New("unsupported AI reserve failure trigger")
	}
	if g.commandID == 0 || strings.TrimSpace(g.owner) == "" || g.token == 0 {
		return aigateway.ErrNotConfigured
	}
	result := tx.WithContext(ctx).Model(&replycommand.Command{}).
		Where("id = ? AND state = ? AND lease_owner = ? AND lease_token = ? AND EXISTS (SELECT 1 FROM ai_runs r WHERE r.id = ? AND r.user_id = ai_reply_commands.user_id AND r.request_id = ai_reply_commands.request_id)", g.commandID, replycommand.StateRunning, strings.TrimSpace(g.owner), g.token, runID).
		Updates(map[string]any{
			"last_error_code": aigateway.ErrCodeInsufficientBalance, "last_error_message": string(trigger), "updated_at": time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return replycommand.ErrLeaseLost
	}
	return nil
}

type gormGatewayAttemptStore struct {
	db         *gorm.DB
	repository *replycommand.GormRepository
	commandID  uint64
	textTaskID uint64
	owner      string
	token      uint64
}

func (store gormGatewayAttemptStore) PutPrepared(ctx context.Context, transaction aigateway.Transaction, attempt aigateway.ProviderAttempt) (aigateway.PreparedWriteResult, error) {
	tx, err := gatewayTransactionDB(transaction)
	if err != nil {
		return aigateway.PreparedWriteResult{}, err
	}
	quoteJSON, err := json.Marshal(attempt.Quote)
	if err != nil {
		return aigateway.PreparedWriteResult{}, err
	}
	var row *replycommand.Attempt
	if store.commandID > 0 {
		if store.repository == nil || strings.TrimSpace(store.owner) == "" || store.token == 0 {
			return aigateway.PreparedWriteResult{}, aigateway.ErrNotConfigured
		}
		prepared, ok, prepareErr := store.repository.PreparePaidAttemptInTransaction(ctx, tx, replycommand.PrepareAttemptInput{
			RunID: attempt.RunID, CommandID: store.commandID, AttemptNo: uint(attempt.AttemptNo),
			Owner: store.owner, Token: store.token, Now: time.Now(), IdempotencyKey: attempt.IdempotencyKey,
			PreparedRequestJSON: string(attempt.PreparedRequest), PreparedRequestSHA256: attempt.RequestSHA256, QuoteJSON: string(quoteJSON),
		})
		if prepareErr != nil {
			return aigateway.PreparedWriteResult{}, prepareErr
		}
		if !ok || prepared == nil {
			return aigateway.PreparedWriteResult{}, replycommand.ErrLeaseLost
		}
		row = prepared
	} else {
		if store.textTaskID == 0 {
			return aigateway.PreparedWriteResult{}, aigateway.ErrNotConfigured
		}
		var task aitext.TextTask
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND run_id = ? AND status = ?", store.textTaskID, attempt.RunID, aitext.StatusRunning).
			First(&task).Error; err != nil {
			return aigateway.PreparedWriteResult{}, err
		}
		now := time.Now()
		prepared := &replycommand.Attempt{
			RunID: attempt.RunID, CommandID: nil, AttemptNo: uint(attempt.AttemptNo),
			IdempotencyKey: attempt.IdempotencyKey, State: replycommand.AttemptPrepared,
			PreparedRequestJSON: string(attempt.PreparedRequest), PreparedRequestSHA256: append([]byte(nil), attempt.RequestSHA256[:]...),
			QuoteJSON: string(quoteJSON), UsageJSON: `{"status":"unavailable"}`, UsageStatus: string(billing.UsageStatusUnavailable),
			DispatchState: infraai.DispatchStateNotDispatched, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.WithContext(ctx).Create(prepared).Error; err != nil {
			return aigateway.PreparedWriteResult{}, err
		}
		row = prepared
	}
	persisted, err := gatewayAttemptFromRow(*row)
	if err != nil {
		return aigateway.PreparedWriteResult{}, err
	}
	return aigateway.PreparedWriteResult{Attempt: persisted, Inserted: true}, nil
}

func (store gormGatewayAttemptStore) GetPreparedForUpdate(ctx context.Context, transaction aigateway.Transaction, runID int64, attemptNo uint32) (aigateway.ProviderAttempt, error) {
	return store.getAttemptForUpdate(ctx, transaction, runID, attemptNo, []replycommand.AttemptState{replycommand.AttemptPrepared})
}

func (store gormGatewayAttemptStore) GetDispatchedForUpdate(ctx context.Context, transaction aigateway.Transaction, runID int64, attemptNo uint32) (aigateway.ProviderAttempt, error) {
	return store.getAttemptForUpdate(ctx, transaction, runID, attemptNo, []replycommand.AttemptState{replycommand.AttemptDispatched})
}

func (store gormGatewayAttemptStore) getAttemptForUpdate(ctx context.Context, transaction aigateway.Transaction, runID int64, attemptNo uint32, states []replycommand.AttemptState) (aigateway.ProviderAttempt, error) {
	tx, err := gatewayTransactionDB(transaction)
	if err != nil {
		return aigateway.ProviderAttempt{}, err
	}
	var row replycommand.Attempt
	query := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("run_id = ? AND attempt_no = ? AND state IN ?", runID, attemptNo, states)
	if store.commandID > 0 {
		query = query.Where("command_id = ?", store.commandID)
	}
	if err := query.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return aigateway.ProviderAttempt{}, aigateway.ErrNotFound
		}
		return aigateway.ProviderAttempt{}, err
	}
	return gatewayAttemptFromRow(row)
}

func (store gormGatewayAttemptStore) MarkDispatched(ctx context.Context, transaction aigateway.Transaction, runID int64, attemptNo uint32) (bool, error) {
	tx, err := gatewayTransactionDB(transaction)
	if err != nil {
		return false, err
	}
	now := time.Now()
	query := tx.WithContext(ctx).Model(&replycommand.Attempt{}).
		Where("run_id = ? AND attempt_no = ? AND state = ?", runID, attemptNo, replycommand.AttemptPrepared)
	if store.commandID > 0 {
		query = query.Where("command_id = ?", store.commandID)
	}
	result := query.Updates(map[string]any{
		"state": replycommand.AttemptDispatched, "dispatch_state": infraai.DispatchStateDispatched,
		"dispatched_at": now, "updated_at": now,
	})
	return result.RowsAffected == 1, result.Error
}

func (store gormGatewayAttemptStore) GetTerminalOutcome(ctx context.Context, transaction aigateway.Transaction, runID int64, attemptNo uint32) (aigateway.DispatchResult, error) {
	tx, err := gatewayTransactionDB(transaction)
	if err != nil {
		return aigateway.DispatchResult{}, err
	}
	var row replycommand.Attempt
	query := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("run_id = ? AND attempt_no = ? AND state IN ?", runID, attemptNo, []replycommand.AttemptState{
			replycommand.AttemptSucceeded, replycommand.AttemptFailed, replycommand.AttemptCanceled, replycommand.AttemptOutcomeUnknown,
		})
	if store.commandID > 0 {
		query = query.Where("command_id = ?", store.commandID)
	}
	if err := query.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return aigateway.DispatchResult{}, aigateway.ErrNotFound
		}
		return aigateway.DispatchResult{}, err
	}
	return gatewayOutcomeFromRow(row)
}

func (store gormGatewayAttemptStore) RecordTerminalOutcome(ctx context.Context, transaction aigateway.Transaction, runID int64, attemptNo uint32, outcome aigateway.DispatchResult) (aigateway.TerminalOutcomeWriteResult, error) {
	tx, err := gatewayTransactionDB(transaction)
	if err != nil {
		return aigateway.TerminalOutcomeWriteResult{}, err
	}
	var row replycommand.Attempt
	query := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ? AND attempt_no = ?", runID, attemptNo)
	if store.commandID > 0 {
		query = query.Where("command_id = ?", store.commandID)
	}
	if err := query.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return aigateway.TerminalOutcomeWriteResult{}, aigateway.ErrNotFound
		}
		return aigateway.TerminalOutcomeWriteResult{}, err
	}
	if row.State != replycommand.AttemptDispatched {
		persisted, err := gatewayOutcomeFromRow(row)
		if err != nil {
			return aigateway.TerminalOutcomeWriteResult{}, err
		}
		return aigateway.TerminalOutcomeWriteResult{Outcome: persisted, Replayed: true}, nil
	}
	usageJSON, err := json.Marshal(outcome.Usage)
	if err != nil {
		return aigateway.TerminalOutcomeWriteResult{}, err
	}
	state, err := gatewayAttemptState(outcome.TerminalState)
	if err != nil {
		return aigateway.TerminalOutcomeWriteResult{}, err
	}
	usageStatus := billing.UsageStatusUnavailable
	if outcome.Usage.Complete() {
		usageStatus = billing.UsageStatusComplete
	}
	now := time.Now()
	responseHash := ""
	if outcome.ResponseSHA256 != ([32]byte{}) {
		responseHash = hex.EncodeToString(outcome.ResponseSHA256[:])
	}
	result := tx.WithContext(ctx).Model(&replycommand.Attempt{}).
		Where("id = ? AND state = ?", row.ID, replycommand.AttemptDispatched).
		Updates(map[string]any{
			"state": state, "provider_request_id": strings.TrimSpace(outcome.ProviderRequestID),
			"response_sha256": responseHash, "error_code": gatewayAttemptErrorCode(state),
			"dispatch_state": strings.TrimSpace(outcome.DispatchState), "usage_json": string(usageJSON),
			"usage_status": usageStatus, "result_candidate_json": outcome.ResultCandidateJSON,
			"finished_at": now, "updated_at": now,
		})
	if result.Error != nil {
		return aigateway.TerminalOutcomeWriteResult{}, result.Error
	}
	if result.RowsAffected != 1 {
		return aigateway.TerminalOutcomeWriteResult{}, replycommand.ErrAttemptTerminalConflict
	}
	return aigateway.TerminalOutcomeWriteResult{Outcome: cloneGatewayOutcome(outcome)}, nil
}

func gatewayAttemptFromRow(row replycommand.Attempt) (aigateway.ProviderAttempt, error) {
	if row.RunID <= 0 || row.AttemptNo == 0 || strings.TrimSpace(row.IdempotencyKey) == "" || len(row.PreparedRequestSHA256) != sha256.Size {
		return aigateway.ProviderAttempt{}, fmt.Errorf("%w: attempt identity is incomplete", errPersistedPaidAttemptEvidence)
	}
	request := []byte(row.PreparedRequestJSON)
	if !json.Valid(request) {
		return aigateway.ProviderAttempt{}, fmt.Errorf("%w: prepared request JSON is invalid", errPersistedPaidAttemptEvidence)
	}
	if persistedHash := sha256.Sum256(request); !bytes.Equal(persistedHash[:], row.PreparedRequestSHA256) {
		return aigateway.ProviderAttempt{}, fmt.Errorf("%w: prepared request hash does not match", errPersistedPaidAttemptEvidence)
	}
	quote, err := paidQuoteFromAttempt(row)
	if err != nil {
		return aigateway.ProviderAttempt{}, fmt.Errorf("%w: %v", errPersistedPaidAttemptEvidence, err)
	}
	var requestHash [sha256.Size]byte
	copy(requestHash[:], row.PreparedRequestSHA256)
	return aigateway.ProviderAttempt{
		RunID: row.RunID, AttemptNo: uint32(row.AttemptNo), IdempotencyKey: strings.TrimSpace(row.IdempotencyKey),
		PreparedRequest: []byte(row.PreparedRequestJSON), RequestSHA256: requestHash, Quote: quote,
	}, nil
}

func gatewayOutcomeFromRow(row replycommand.Attempt) (aigateway.DispatchResult, error) {
	state, err := gatewayTerminalState(row.State)
	if err != nil {
		return aigateway.DispatchResult{}, err
	}
	usage, err := usageFromAttempt(row)
	if err != nil {
		return aigateway.DispatchResult{}, err
	}
	var responseHash [sha256.Size]byte
	if value := strings.TrimSpace(row.ResponseSHA256); value != "" {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != sha256.Size {
			return aigateway.DispatchResult{}, errors.New("provider response hash is invalid")
		}
		copy(responseHash[:], decoded)
	}
	var candidate *string
	if row.ResultCandidateJSON != nil {
		value := *row.ResultCandidateJSON
		candidate = &value
	}
	return aigateway.DispatchResult{
		ProviderRequestID: strings.TrimSpace(row.ProviderRequestID), ResponseSHA256: responseHash,
		DispatchState: strings.TrimSpace(row.DispatchState), TerminalState: state, Usage: usage,
		ResultCandidateJSON: candidate,
	}, nil
}

func gatewayAttemptState(state string) (replycommand.AttemptState, error) {
	switch strings.TrimSpace(state) {
	case "succeeded":
		return replycommand.AttemptSucceeded, nil
	case "failed":
		return replycommand.AttemptFailed, nil
	case "canceled":
		return replycommand.AttemptCanceled, nil
	case "outcome_unknown":
		return replycommand.AttemptOutcomeUnknown, nil
	default:
		return "", fmt.Errorf("unsupported provider terminal state %q", state)
	}
}

func gatewayTerminalState(state replycommand.AttemptState) (string, error) {
	switch state {
	case replycommand.AttemptSucceeded, replycommand.AttemptFailed, replycommand.AttemptCanceled, replycommand.AttemptOutcomeUnknown:
		return string(state), nil
	default:
		return "", fmt.Errorf("provider attempt %q is not terminal", state)
	}
}

func gatewayAttemptErrorCode(state replycommand.AttemptState) string {
	switch state {
	case replycommand.AttemptFailed:
		return "ai.provider_failed"
	case replycommand.AttemptCanceled:
		return "ai.provider_canceled"
	case replycommand.AttemptOutcomeUnknown:
		return "ai.provider_outcome_unknown"
	default:
		return ""
	}
}

func cloneGatewayOutcome(outcome aigateway.DispatchResult) aigateway.DispatchResult {
	outcome.Usage.RawProviderJSON = append([]byte(nil), outcome.Usage.RawProviderJSON...)
	outcome.Usage.Items = append([]infraai.UsageItem(nil), outcome.Usage.Items...)
	if outcome.ResultCandidateJSON != nil {
		value := *outcome.ResultCandidateJSON
		outcome.ResultCandidateJSON = &value
	}
	return outcome
}

type gormGatewayOwnerGuard struct {
	commandID uint64
	owner     string
	token     uint64
	now       func() time.Time
}

func (guard gormGatewayOwnerGuard) EnsureRunnable(ctx context.Context, transaction aigateway.Transaction, runID int64) error {
	tx, err := gatewayTransactionDB(transaction)
	if err != nil {
		return err
	}
	if guard.commandID == 0 || strings.TrimSpace(guard.owner) == "" || guard.token == 0 || runID <= 0 {
		return aigateway.ErrNotConfigured
	}
	now := time.Now()
	if guard.now != nil {
		now = guard.now()
	}
	var command replycommand.Command
	err = tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND lease_owner = ? AND lease_token = ? AND state = ? AND cancel_requested_at IS NULL AND lease_expires_at > ?", guard.commandID, strings.TrimSpace(guard.owner), guard.token, replycommand.StateRunning, now).
		Where("EXISTS (SELECT 1 FROM ai_runs r WHERE r.id = ? AND r.user_id = ai_reply_commands.user_id AND r.request_id = ai_reply_commands.request_id)", runID).
		First(&command).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return replycommand.ErrLeaseLost
	}
	return err
}

var (
	_ aigateway.TransactionRunner      = gormGatewayTransactions{}
	_ aigateway.RunStore               = gormGatewayRunStore{}
	_ aigateway.PriorUsagePricer       = gormGatewayPriorUsagePricer{}
	_ aigateway.ReserveParticipant     = gormGatewayReserveParticipant{}
	_ aigateway.ReserveFailureRecorder = gormGatewayReserveFailureRecorder{}
	_ aigateway.AttemptStore           = gormGatewayAttemptStore{}
	_ aigateway.OwnerGuard             = gormGatewayOwnerGuard{}
)

var _ aichat.PaidChatAttemptExecutor = (*paidChatAttemptExecutor)(nil)
