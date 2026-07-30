package runtime

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/billing"
	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/module/ai/requestidentity"
	airun "admin_back_go/internal/module/ai/run"
	aitext "admin_back_go/internal/module/ai/text"
	aitool "admin_back_go/internal/module/ai/tool"
	walletmodule "admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type textAttemptGateway interface {
	AssembleAndQuote(context.Context, aigateway.RunRequest) (aigateway.PreparedCall, error)
	ReserveAndPrepare(context.Context, aigateway.ReserveAndPrepareInput) (aigateway.ProviderAttempt, error)
	Dispatch(context.Context, aigateway.ProviderAttempt) (aigateway.DispatchResult, error)
}

func runTextGatewayAttempt(ctx context.Context, gateway textAttemptGateway, request aigateway.RunRequest, attemptNo uint32, recoverPrepared bool) (aigateway.DispatchResult, error) {
	var (
		attempt aigateway.ProviderAttempt
		err     error
	)
	if recoverPrepared {
		attempt, err = gateway.ReserveAndPrepare(ctx, aigateway.ReserveAndPrepareInput{
			RunID: request.RunID, AttemptNo: attemptNo,
		})
	} else {
		call, assembleErr := gateway.AssembleAndQuote(ctx, request)
		if assembleErr != nil {
			return aigateway.DispatchResult{}, assembleErr
		}
		attempt, err = gateway.ReserveAndPrepare(ctx, aigateway.ReserveAndPrepareInput{
			RunID: request.RunID, AttemptNo: attemptNo, NewCall: &call,
		})
	}
	if err != nil {
		return aigateway.DispatchResult{}, err
	}
	result, err := gateway.Dispatch(ctx, attempt)
	if err != nil {
		var gatewayErr *aigateway.Error
		if errors.As(err, &gatewayErr) && (gatewayErr.Code == aigateway.ErrCodePreparedMissing || gatewayErr.Code == aigateway.ErrCodeDuplicateAttempt) {
			return aigateway.DispatchResult{}, fmt.Errorf("%w: %v", ErrTextAttemptOwnedElsewhere, err)
		}
	}
	return result, err
}

var ErrTextAttemptOwnedElsewhere = errors.New("AI text provider attempt is owned by another worker")

type paidTextTaskExecutor struct {
	db           *gorm.DB
	wallets      *walletmodule.GormRepository
	tasks        *aitext.GormStore
	secretbox    secretbox.Box
	chatEngine   aichat.EngineFactory
	toolEngine   aitool.EngineFactory
	finalizer    aigateway.Finalizer
	finalization *gormTextFinalizationStore
	now          func() time.Time
}

func newPaidTextTaskExecutor(client *database.Client, wallets *walletmodule.GormRepository, tasks *aitext.GormStore, chatEngine aichat.EngineFactory, toolEngine aitool.EngineFactory, box secretbox.Box) *paidTextTaskExecutor {
	if client == nil || client.Gorm == nil || wallets == nil || tasks == nil || chatEngine == nil || toolEngine == nil {
		return nil
	}
	store := newGormTextFinalizationStore(client.Gorm, wallets, tasks)
	if store == nil {
		return nil
	}
	return &paidTextTaskExecutor{
		db: client.Gorm, wallets: wallets, tasks: tasks, secretbox: box, chatEngine: chatEngine, toolEngine: toolEngine,
		finalizer: aigateway.NewFinalizer(store, persistedSettlementPricer{}), finalization: store, now: time.Now,
	}
}

func (executor *paidTextTaskExecutor) ExecuteTextTask(ctx context.Context, taskID uint64) error {
	if executor == nil || executor.db == nil || executor.tasks == nil || executor.finalizer == nil || taskID == 0 {
		return aigateway.ErrNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	execution, err := executor.tasks.LoadExecution(ctx, taskID)
	if err != nil {
		return err
	}
	if execution == nil || execution.Task.ID != taskID || execution.Task.RunID <= 0 {
		return fmt.Errorf("text task execution identity is invalid")
	}
	if execution.Task.Status != aitext.StatusRunning {
		return nil
	}
	latest, hasAttempt, err := executor.latestTextAttempt(ctx, execution.Task.RunID)
	if err != nil {
		return err
	}
	if hasAttempt {
		switch latest.State {
		case replycommand.AttemptSucceeded:
			if err := executor.persistTextCandidate(ctx, execution.Task, latest); err != nil {
				return err
			}
			return executor.finalizer.Finalize(context.WithoutCancel(ctx), aigateway.FinalizeRequest{RunID: execution.Task.RunID})
		case replycommand.AttemptFailed, replycommand.AttemptCanceled, replycommand.AttemptOutcomeUnknown:
			return executor.finalizer.Finalize(context.WithoutCancel(ctx), aigateway.FinalizeRequest{RunID: execution.Task.RunID})
		case replycommand.AttemptDispatched:
			now := time.Now()
			if executor.now != nil {
				now = executor.now()
			}
			if !textDispatchedAttemptStale(latest, now) {
				return errTextAttemptStillActive
			}
			if err := executor.markTextOutcomeUnknown(context.WithoutCancel(ctx), latest.ID, execution.Task.RunID, now); err != nil {
				return err
			}
			return executor.finalizer.Finalize(context.WithoutCancel(ctx), aigateway.FinalizeRequest{RunID: execution.Task.RunID})
		case replycommand.AttemptPrepared:
			return executor.dispatchText(ctx, *execution, uint32(latest.AttemptNo), true)
		default:
			return fmt.Errorf("unsupported text provider attempt state %q", latest.State)
		}
	}
	if strings.TrimSpace(execution.Task.LastErrorCode) != "" {
		return executor.finalizer.Finalize(context.WithoutCancel(ctx), aigateway.FinalizeRequest{RunID: execution.Task.RunID})
	}
	return executor.dispatchText(ctx, *execution, 1, false)
}

func (executor *paidTextTaskExecutor) dispatchText(ctx context.Context, execution aitext.Execution, attemptNo uint32, recoverPrepared bool) error {
	snapshot, err := aitext.DecodeProviderInputSnapshot(execution.InputSnapshot)
	if err != nil {
		return executor.failTextTask(execution.Task, aitext.ErrorCodeConfiguration, "AI文本输入快照无效")
	}
	if len(execution.Task.RequestFingerprint) != sha256.Size {
		return executor.failTextTask(execution.Task, aitext.ErrorCodeConfiguration, "AI文本请求身份快照缺失")
	}
	if strings.TrimSpace(execution.EngineAPIKeyEnc) == "" {
		return executor.failTextTask(execution.Task, aitext.ErrorCodeConfiguration, "AI供应商配置不完整")
	}
	apiKey, err := executor.secretbox.Decrypt(strings.TrimSpace(execution.EngineAPIKeyEnc))
	if err != nil || strings.TrimSpace(apiKey) == "" {
		return executor.failTextTask(execution.Task, aitext.ErrorCodeConfiguration, "AI供应商API Key未配置")
	}
	engine, err := executor.newTextEngine(ctx, execution.Task.Kind, aichat.EngineConfig{
		EngineType: infraai.EngineType(strings.TrimSpace(execution.EngineType)), BaseURL: strings.TrimSpace(execution.EngineBaseURL), APIKey: apiKey,
	})
	if err != nil || engine == nil {
		return executor.failTextTask(execution.Task, aitext.ErrorCodeConfiguration, "创建AI文本引擎失败")
	}
	transport, ok := engine.(aigateway.PreparedChatTransport)
	if !ok {
		return executor.failTextTask(execution.Task, aitext.ErrorCodeConfiguration, "AI供应商不支持prepared请求")
	}
	chatInput := infraai.ChatInput{
		AgentID: execution.Task.AgentID, RunID: uint64(execution.Task.RunID), UserID: uint64(execution.Task.UserID),
		UserKey: fmt.Sprintf("%s:%d", execution.Task.Platform, execution.Task.UserID), Content: snapshot.Prompt,
		Inputs: map[string]any{"model_id": execution.Task.ModelID},
	}
	if snapshot.SystemPrompt != "" {
		chatInput.Inputs["system_prompt"] = snapshot.SystemPrompt
	}
	provider := aigateway.NewPreparedChatProvider(transport, discardTextSink{}, func(result *infraai.ChatResult) (*string, error) {
		return encodeTextResultCandidate(execution.Task.Kind, result)
	})
	runs := gormGatewayRunStore{db: executor.db}
	attempts := gormGatewayAttemptStore{db: executor.db, textTaskID: execution.Task.ID}
	dependencies := aigateway.Dependencies{
		Assembler: paidChatAssembler{transport: transport, input: chatInput}, Quotes: aigateway.PersistedQuoteValidator{},
		Transactions: gormGatewayTransactions{db: executor.db}, Runs: runs, PriorUsage: gormGatewayPriorUsagePricer{},
		Reserve: gormGatewayReserveParticipant{wallets: executor.wallets}, Failures: gormTextReserveFailureRecorder{taskID: execution.Task.ID},
		Attempts: attempts, Provider: provider, Owner: gormTextOwnerGuard{taskID: execution.Task.ID},
	}
	gateway := aigateway.New(dependencies)
	identity := requestidentity.Input{
		UserID: execution.Task.UserID, Operation: snapshot.Operation, Modality: snapshot.Modality,
		AgentID: int64(execution.Task.AgentID), ModelID: execution.Task.ModelID, NormalizedText: snapshot.NormalizedText,
		Options: requestidentity.GenerationOptions{MaxOutputTokens: snapshot.MaxOutputTokens},
	}
	request := aigateway.RunRequest{UserID: execution.Task.UserID, RunID: execution.Task.RunID, RequestID: execution.Task.RequestID, Identity: identity}
	result, err := runTextGatewayAttempt(ctx, gateway, request, attemptNo, recoverPrepared)
	if err != nil {
		if errors.Is(err, ErrTextAttemptOwnedElsewhere) {
			return err
		}
		if isPaidInsufficientBalance(err) {
			return executor.finalizer.Finalize(context.WithoutCancel(ctx), aigateway.FinalizeRequest{RunID: execution.Task.RunID})
		}
		if hasPersistedTextTerminalAttempt(ctx, executor.db, execution.Task.RunID, attemptNo) {
			return executor.finalizer.Finalize(context.WithoutCancel(ctx), aigateway.FinalizeRequest{RunID: execution.Task.RunID})
		}
		if gatewayErr := new(aigateway.Error); errors.As(err, &gatewayErr) && gatewayErr.Code == aigateway.ErrCodeFingerprintConflict {
			return executor.failTextTask(execution.Task, aitext.ErrorCodeConfiguration, "AI文本请求身份与接受快照不一致")
		}
		code := textPreDispatchErrorCode(err)
		message := "AI文本prepared请求失败"
		if code == aitext.ErrorCodePriceUnavailable {
			message = "AI模型价格不可用"
		} else if code == aitext.ErrorCodeUnsafeUpperBound {
			message = "AI请求缺少安全用量上界"
		}
		return executor.failTextTask(execution.Task, code, message)
	}
	if result.ResultCandidateJSON == nil {
		return executor.failTextTask(execution.Task, aitext.ErrorCodeProviderFailed, "AI供应商返回的业务结果无效")
	}
	answer, candidateErr := aitext.AnswerFromResultCandidate(*result.ResultCandidateJSON)
	if candidateErr != nil {
		return executor.failTextTask(execution.Task, aitext.ErrorCodeProviderFailed, "AI文本结果候选无效")
	}
	if err := executor.tasks.SaveCandidate(context.WithoutCancel(ctx), execution.Task.ID, execution.Task.RunID, answer); err != nil {
		return err
	}
	return executor.finalizer.Finalize(context.WithoutCancel(ctx), aigateway.FinalizeRequest{RunID: execution.Task.RunID})
}

func (executor *paidTextTaskExecutor) newTextEngine(ctx context.Context, kind string, input aichat.EngineConfig) (infraai.Engine, error) {
	if executor == nil {
		return nil, aigateway.ErrNotConfigured
	}
	switch strings.TrimSpace(kind) {
	case aitext.KindText:
		if executor.chatEngine == nil {
			return nil, aigateway.ErrNotConfigured
		}
		return executor.chatEngine.NewEngine(ctx, input)
	case aitext.KindToolDraft:
		if executor.toolEngine == nil {
			return nil, aigateway.ErrNotConfigured
		}
		return executor.toolEngine.NewEngine(ctx, aitool.EngineConfig{
			EngineType: input.EngineType,
			BaseURL:    input.BaseURL,
			APIKey:     input.APIKey,
		})
	default:
		return nil, fmt.Errorf("unsupported AI text task kind %q", kind)
	}
}

func (executor *paidTextTaskExecutor) failTextTask(task aitext.TextTask, code, message string) error {
	ctx := context.Background()
	if err := executor.tasks.MarkExecutionFailure(ctx, task.ID, task.RunID, code, message); err != nil {
		return err
	}
	return executor.finalizer.Finalize(ctx, aigateway.FinalizeRequest{RunID: task.RunID})
}

func (executor *paidTextTaskExecutor) latestTextAttempt(ctx context.Context, runID int64) (replycommand.Attempt, bool, error) {
	var row replycommand.Attempt
	err := executor.db.WithContext(ctx).Where("run_id = ?", runID).Order("attempt_no DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return replycommand.Attempt{}, false, nil
	}
	return row, err == nil, err
}

func (executor *paidTextTaskExecutor) persistTextCandidate(ctx context.Context, task aitext.TextTask, attempt replycommand.Attempt) error {
	if attempt.ResultCandidateJSON == nil {
		return executor.tasks.MarkExecutionFailure(ctx, task.ID, task.RunID, aitext.ErrorCodeProviderFailed, "AI供应商返回的业务结果无效")
	}
	answer, err := aitext.AnswerFromResultCandidate(*attempt.ResultCandidateJSON)
	if err != nil {
		return executor.tasks.MarkExecutionFailure(ctx, task.ID, task.RunID, aitext.ErrorCodeProviderFailed, "AI文本结果候选无效")
	}
	return executor.tasks.SaveCandidate(ctx, task.ID, task.RunID, answer)
}

var errTextAttemptStillActive = errors.New("AI text provider attempt is still inside its dispatch deadline")

func textDispatchedAttemptStale(attempt replycommand.Attempt, now time.Time) bool {
	if attempt.State != replycommand.AttemptDispatched {
		return false
	}
	if attempt.DispatchedAt == nil || attempt.DispatchedAt.IsZero() {
		return true
	}
	return !now.Before(attempt.DispatchedAt.Add(aitext.GenerateTimeout))
}

func (executor *paidTextTaskExecutor) markTextOutcomeUnknown(ctx context.Context, attemptID uint64, runID int64, now time.Time) error {
	return executor.db.WithContext(ctx).Model(&replycommand.Attempt{}).
		Where("id = ? AND run_id = ? AND state = ?", attemptID, runID, replycommand.AttemptDispatched).
		Updates(map[string]any{
			"state": replycommand.AttemptOutcomeUnknown, "dispatch_state": infraai.DispatchStateUnknown,
			"usage_json": `{"status":"unavailable"}`, "usage_status": billing.UsageStatusUnavailable,
			"error_code": "ai.provider_outcome_unknown", "finished_at": now, "updated_at": now,
		}).Error
}

func hasPersistedTextTerminalAttempt(ctx context.Context, db *gorm.DB, runID int64, attemptNo uint32) bool {
	var count int64
	if db == nil {
		return false
	}
	db.WithContext(ctx).Model(&replycommand.Attempt{}).Where("run_id = ? AND attempt_no = ? AND state IN ?", runID, attemptNo, []replycommand.AttemptState{replycommand.AttemptSucceeded, replycommand.AttemptFailed, replycommand.AttemptCanceled, replycommand.AttemptOutcomeUnknown}).Count(&count)
	return count == 1
}

type discardTextSink struct{}

func (discardTextSink) Emit(context.Context, infraai.Event) error { return nil }

func encodeTextResultCandidate(kind string, result *infraai.ChatResult) (*string, error) {
	if result == nil {
		return nil, errors.New("AI文本供应商结果为空")
	}
	answer := result.Answer
	if kind == aitext.KindToolDraft {
		normalized, appErr := aitool.NormalizeGenerateDraftCandidate(answer)
		if appErr != nil {
			return nil, nil
		}
		answer = normalized
	} else if kind != aitext.KindText {
		return nil, fmt.Errorf("unsupported AI text task kind %q", kind)
	}
	candidate, err := aitext.MarshalResultCandidate(answer)
	if err != nil {
		if errors.Is(err, aitext.ErrCandidateConflict) {
			return nil, nil
		}
		return nil, err
	}
	return &candidate, nil
}

func textPreDispatchErrorCode(err error) string {
	if errors.Is(err, pricing.ErrPriceUnavailable) || errors.Is(err, pricing.ErrMissingModel) || errors.Is(err, pricing.ErrInvalidCatalog) || errors.Is(err, pricing.ErrUnsupportedUsage) || errors.Is(err, pricing.ErrInvalidMultiplier) {
		return aitext.ErrorCodePriceUnavailable
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(message, "upper-bound") || strings.Contains(message, "upper bound") || strings.Contains(message, "safe input") || strings.Contains(message, "prepared request body") {
		return aitext.ErrorCodeUnsafeUpperBound
	}
	return aitext.ErrorCodeConfiguration
}

type gormTextReserveFailureRecorder struct{ taskID uint64 }

func (r gormTextReserveFailureRecorder) RecordReserveFailure(ctx context.Context, transaction aigateway.Transaction, runID int64, trigger aigateway.FinalizationTrigger) error {
	tx, err := gatewayTransactionDB(transaction)
	if err != nil {
		return err
	}
	if r.taskID == 0 || (trigger != aigateway.TriggerInitialInsufficient && trigger != aigateway.TriggerContinuationTopUpInsufficient) {
		return aigateway.ErrNotConfigured
	}
	result := tx.WithContext(ctx).Model(&aitext.TextTask{}).Where("id = ? AND run_id = ? AND status = ?", r.taskID, runID, aitext.StatusRunning).
		Updates(map[string]any{"last_error_code": aigateway.ErrCodeInsufficientBalance, "error_message": "余额不足，请充值后重试", "updated_at": time.Now()})
	return result.Error
}

type gormTextOwnerGuard struct{ taskID uint64 }

func (guard gormTextOwnerGuard) EnsureRunnable(ctx context.Context, transaction aigateway.Transaction, runID int64) error {
	tx, err := gatewayTransactionDB(transaction)
	if err != nil {
		return err
	}
	if guard.taskID == 0 {
		return aigateway.ErrNotConfigured
	}
	var task aitext.TextTask
	err = tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND run_id = ? AND status = ?", guard.taskID, runID, aitext.StatusRunning).
		Where("EXISTS (SELECT 1 FROM ai_runs r WHERE r.id = ? AND r.status = ?)", runID, enum.AIRunStatusRunning).
		First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("text task is no longer runnable")
	}
	return err
}

type gormTextFinalizationStore struct {
	db      *gorm.DB
	wallets *walletmodule.GormRepository
	tasks   *aitext.GormStore
	now     func() time.Time
}

func newGormTextFinalizationStore(db *gorm.DB, wallets *walletmodule.GormRepository, tasks *aitext.GormStore) *gormTextFinalizationStore {
	if db == nil || wallets == nil || tasks == nil {
		return nil
	}
	return &gormTextFinalizationStore{db: db, wallets: wallets, tasks: tasks, now: time.Now}
}

func (store *gormTextFinalizationStore) WithLockedSettlement(ctx context.Context, runID int64, decide func(aigateway.FinalizationFacts) (aigateway.SettlementDecision, error)) (aigateway.FinalizationApplyResult, error) {
	if store == nil || store.db == nil || store.wallets == nil || runID <= 0 || decide == nil {
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
	err := store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		run, charge, wallet, hold, err := lockChatSettlementMoneyGraph(ctx, tx, runID)
		if err != nil {
			return err
		}
		task, attempts, err := lockTextSettlementBusinessGraph(ctx, tx, run)
		if err != nil {
			return err
		}
		if terminalChatBilling(run, charge) {
			if err := validateTextFinalizationReplay(run, charge, wallet, hold, task, attempts); err != nil {
				return err
			}
			if err := airun.ProjectTerminalDashboardFacts(ctx, tx, run.ID); err != nil {
				return err
			}
			replayed = true
			return nil
		}
		if run.Status != enum.AIRunStatusRunning || charge.Status != billing.ChargeStatusOpen || charge.FinalizedAt != nil || task.Status != aitext.StatusRunning {
			return errors.New("AI text settlement is neither open nor a valid terminal replay")
		}
		attempts, err = normalizeTextAttemptsForFinalization(ctx, tx, task, attempts, now)
		if err != nil {
			return err
		}
		facts, err := buildTextFinalizationFacts(run, charge, hold, task, attempts)
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
		if err := finalizeTextTaskRunAndCharge(ctx, tx, task, run, charge, facts, decision, now); err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Model(&replycommand.Attempt{}).Where("run_id = ? AND result_candidate_json IS NOT NULL", run.ID).Update("result_candidate_json", nil).Error; err != nil {
			return err
		}
		if err := appendChatRunFinalizationEvents(ctx, tx, run.ID, facts, decision, now); err != nil {
			return err
		}
		applied = true
		return nil
	})
	if err != nil {
		return aigateway.FinalizationApplyResult{}, err
	}
	return aigateway.FinalizationApplyResult{Applied: applied, Replayed: replayed}, nil
}

func lockTextSettlementBusinessGraph(ctx context.Context, tx *gorm.DB, run airun.Run) (aitext.TextTask, []replycommand.Attempt, error) {
	var task aitext.TextTask
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ? AND user_id = ? AND request_id = ?", run.ID, run.UserID, run.RequestID).First(&task).Error; err != nil {
		return aitext.TextTask{}, nil, err
	}
	var attempts []replycommand.Attempt
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ?", run.ID).Order("attempt_no ASC").Find(&attempts).Error; err != nil {
		return aitext.TextTask{}, nil, err
	}
	return task, attempts, nil
}

func normalizeTextAttemptsForFinalization(ctx context.Context, tx *gorm.DB, task aitext.TextTask, attempts []replycommand.Attempt, now time.Time) ([]replycommand.Attempt, error) {
	if len(attempts) == 0 || strings.TrimSpace(task.LastErrorCode) == "" {
		return attempts, nil
	}
	latest := &attempts[len(attempts)-1]
	if latest.State != replycommand.AttemptPrepared || strings.TrimSpace(latest.DispatchState) != infraai.DispatchStateNotDispatched {
		return attempts, nil
	}
	updates := map[string]any{
		"state": replycommand.AttemptCanceled, "dispatch_state": infraai.DispatchStateNotDispatched,
		"usage_status": billing.UsageStatusUnavailable, "usage_json": `{"status":"unavailable"}`,
		"result_candidate_json": nil, "provider_request_id": "", "response_sha256": "",
		"error_code": task.LastErrorCode, "finished_at": now, "updated_at": now,
	}
	result := tx.WithContext(ctx).Model(&replycommand.Attempt{}).Where("id = ? AND state = ?", latest.ID, replycommand.AttemptPrepared).Updates(updates)
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

func deriveTextFinalizationTrigger(task aitext.TextTask, attempts []replycommand.Attempt) (aigateway.FinalizationTrigger, error) {
	code := strings.TrimSpace(task.LastErrorCode)
	if code == aigateway.ErrCodeInsufficientBalance {
		if len(attempts) == 0 {
			return aigateway.TriggerInitialInsufficient, nil
		}
		return aigateway.TriggerContinuationTopUpInsufficient, nil
	}
	if len(attempts) == 0 {
		if code != "" {
			return aigateway.TriggerPreDispatchFailed, nil
		}
		return "", aigateway.ErrFinalizationPending
	}
	latest := attempts[len(attempts)-1]
	if code != "" && latest.State == replycommand.AttemptSucceeded {
		return aigateway.TriggerLocalFailure, nil
	}
	switch latest.State {
	case replycommand.AttemptSucceeded:
		return aigateway.TriggerSuccess, nil
	case replycommand.AttemptFailed, replycommand.AttemptCanceled:
		if code != "" && latest.DispatchState == infraai.DispatchStateNotDispatched {
			return aigateway.TriggerPreDispatchFailed, nil
		}
		return aigateway.TriggerProviderFailed, nil
	case replycommand.AttemptOutcomeUnknown:
		return aigateway.TriggerOutcomeUnknown, nil
	default:
		return "", aigateway.ErrFinalizationPending
	}
}

func buildTextFinalizationFacts(run airun.Run, charge billing.UsageCharge, hold *walletmodule.Hold, task aitext.TextTask, attempts []replycommand.Attempt) (aigateway.FinalizationFacts, error) {
	snapshot, err := gatewayRunSnapshot(run)
	if err != nil {
		return aigateway.FinalizationFacts{}, err
	}
	trigger, err := deriveTextFinalizationTrigger(task, attempts)
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
	facts.Attempts = make([]aigateway.FinalizationAttempt, 0, len(attempts))
	for _, row := range attempts {
		if row.ID == 0 || row.RunID != run.ID {
			return aigateway.FinalizationFacts{}, errors.New("text provider attempt identity is invalid")
		}
		usage, usageErr := usageFromAttempt(row)
		if usageErr != nil {
			usage = infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
		}
		auditOnlyPrepared := trigger == aigateway.TriggerPreDispatchFailed && row.State == replycommand.AttemptCanceled && row.DispatchState == infraai.DispatchStateNotDispatched
		if !auditOnlyPrepared {
			if _, quoteErr := paidQuoteFromAttempt(row); quoteErr != nil {
				return aigateway.FinalizationFacts{}, fmt.Errorf("load text attempt %d quote: %w", row.ID, quoteErr)
			}
			if len(row.PreparedRequestSHA256) != sha256.Size || sha256.Sum256([]byte(row.PreparedRequestJSON)) != digestFromBytes(row.PreparedRequestSHA256) {
				return aigateway.FinalizationFacts{}, fmt.Errorf("text attempt %d prepared request evidence is invalid", row.ID)
			}
		}
		responseHash, hashErr := decodeOptionalSHA256(row.ResponseSHA256)
		if hashErr != nil {
			return aigateway.FinalizationFacts{}, hashErr
		}
		facts.Attempts = append(facts.Attempts, aigateway.FinalizationAttempt{
			ID: int64(row.ID), RunID: row.RunID, AttemptNo: uint32(row.AttemptNo), EvidenceKind: aigateway.AttemptEvidencePaid,
			State: billing.AttemptState(row.State), DispatchState: billing.DispatchState(row.DispatchState), Usage: usage,
			ProviderRequestID: strings.TrimSpace(row.ProviderRequestID), ResponseSHA256: responseHash,
		})
	}
	if len(attempts) > 0 {
		latest := attempts[len(attempts)-1]
		facts.CurrentAttemptID = int64(latest.ID)
		if latest.ResultCandidateJSON != nil {
			facts.Candidate = aigateway.FinalizationCandidate{AttemptID: int64(latest.ID), JSON: strings.TrimSpace(*latest.ResultCandidateJSON)}
		}
	}
	return facts, nil
}

func finalizeTextTaskRunAndCharge(ctx context.Context, tx *gorm.DB, task aitext.TextTask, run airun.Run, charge billing.UsageCharge, facts aigateway.FinalizationFacts, decision aigateway.SettlementDecision, now time.Time) error {
	taskUpdates := map[string]any{
		"status": aitext.StatusFailed, "answer": nil, "finished_at": now,
		"elapsed_ms": finalizationDurationMS(run.StartedAt, now), "updated_at": now,
	}
	if decision.RunStatus == enum.AIRunStatusSuccess && decision.CandidateAction == aigateway.SettlementCandidatePublish {
		answer, err := aitext.AnswerFromResultCandidate(decision.Candidate.JSON)
		if err != nil || task.Answer == nil || strings.TrimSpace(*task.Answer) != answer {
			return errors.New("settled AI text candidate does not match durable task candidate")
		}
		taskUpdates["status"] = aitext.StatusSuccess
		taskUpdates["answer"] = answer
		taskUpdates["last_error_code"] = ""
		taskUpdates["error_message"] = nil
	} else {
		code, message := textFinalizationFailure(task, facts, decision)
		taskUpdates["last_error_code"] = code
		taskUpdates["error_message"] = message
	}
	taskUpdate := tx.WithContext(ctx).Model(&aitext.TextTask{}).Where("id = ? AND run_id = ? AND status = ?", task.ID, run.ID, aitext.StatusRunning).Updates(taskUpdates)
	if taskUpdate.Error != nil {
		return taskUpdate.Error
	}
	if taskUpdate.RowsAffected != 1 {
		return errors.New("AI text task terminal compare-and-set was rejected")
	}
	prompt, completion, total, err := finalizationTokenTotals(facts)
	if err != nil {
		return err
	}
	runUpdates := map[string]any{
		"status": decision.RunStatus, "billing_status": decision.BillingStatus, "billing_reason": decision.BillingReason,
		"prompt_tokens": prompt, "completion_tokens": completion, "total_tokens": total,
		"finished_at": now, "duration_ms": finalizationDurationMS(run.StartedAt, now), "updated_at": now,
		"assistant_message_id": nil,
	}
	if decision.RunStatus == enum.AIRunStatusSuccess {
		runUpdates["error_message"] = ""
	} else {
		_, message := textFinalizationFailure(task, facts, decision)
		runUpdates["error_message"] = truncateFinalizationMessage(message)
	}
	runUpdate := tx.WithContext(ctx).Model(&airun.Run{}).Where("id = ? AND status = ? AND billing_status IN ?", run.ID, enum.AIRunStatusRunning, []billing.BillingStatus{billing.BillingStatusPending, billing.BillingStatusHeld}).Updates(runUpdates)
	if runUpdate.Error != nil {
		return runUpdate.Error
	}
	if runUpdate.RowsAffected != 1 {
		return errors.New("AI text Run terminal compare-and-set was rejected")
	}
	actualUnits := int64(0)
	if decision.ChargeStatus == billing.ChargeStatusSettled {
		actualUnits = decision.ActualUnits
	}
	chargeUpdate := tx.WithContext(ctx).Model(&billing.UsageCharge{}).Where("id = ? AND run_id = ? AND status = ? AND finalized_at IS NULL", charge.ID, run.ID, billing.ChargeStatusOpen).
		Updates(map[string]any{"actual_units": actualUnits, "status": decision.ChargeStatus, "finalized_at": now, "updated_at": now})
	if chargeUpdate.Error != nil {
		return chargeUpdate.Error
	}
	if chargeUpdate.RowsAffected != 1 {
		return errors.New("AI text usage charge terminal compare-and-set was rejected")
	}
	return airun.ProjectTerminalDashboardFacts(ctx, tx, run.ID)
}

func textFinalizationFailure(task aitext.TextTask, facts aigateway.FinalizationFacts, decision aigateway.SettlementDecision) (string, string) {
	if facts.Trigger == aigateway.TriggerInitialInsufficient || facts.Trigger == aigateway.TriggerContinuationTopUpInsufficient {
		return aitext.ErrorCodeInsufficientBalance, "余额不足，请充值后重试"
	}
	if decision.BillingReason == billing.BillingReasonUnbilledUsageIncomplete {
		return aitext.ErrorCodeUsageIncomplete, "AI供应商未返回完整用量，本次未扣费"
	}
	if decision.BillingReason == billing.BillingReasonUnbilledOverHold {
		return "ai.billing.over_hold", "AI用量超过冻结上限，本次未扣费"
	}
	if code := strings.TrimSpace(task.LastErrorCode); code != "" {
		message := "AI文本生成配置错误"
		if task.ErrorMessage != nil && strings.TrimSpace(*task.ErrorMessage) != "" {
			message = strings.TrimSpace(*task.ErrorMessage)
		}
		return code, message
	}
	if facts.Trigger == aigateway.TriggerOutcomeUnknown {
		return "ai.provider_outcome_unknown", "上游结果无法确认，本次未扣费"
	}
	return aitext.ErrorCodeProviderFailed, "AI文本生成失败，本次未扣费"
}

func validateTextFinalizationReplay(run airun.Run, charge billing.UsageCharge, wallet *walletmodule.Wallet, hold *walletmodule.Hold, task aitext.TextTask, attempts []replycommand.Attempt) error {
	if charge.FinalizedAt == nil || charge.RunID != run.ID || charge.UserID != run.UserID || task.RunID != run.ID || task.FinishedAt == nil {
		return errors.New("terminal AI text replay facts are invalid")
	}
	switch billing.BillingStatus(run.BillingStatus) {
	case billing.BillingStatusSettled:
		if charge.Status != billing.ChargeStatusSettled || task.Status != aitext.StatusSuccess || task.Answer == nil || hold == nil || wallet == nil || hold.Status != walletmodule.HoldCaptured || hold.HeldUnits != 0 || hold.CapturedUnits != charge.ActualUnits {
			return errors.New("settled AI text replay facts are inconsistent")
		}
	case billing.BillingStatusReleased, billing.BillingStatusUnbilled:
		if task.Status != aitext.StatusFailed || task.Answer != nil || charge.ActualUnits != 0 || (hold != nil && (hold.Status != walletmodule.HoldReleased || hold.HeldUnits != 0 || hold.CapturedUnits != 0)) {
			return errors.New("released AI text replay facts are inconsistent")
		}
	default:
		return errors.New("AI text replay billing status is not terminal")
	}
	for _, attempt := range attempts {
		if attempt.ResultCandidateJSON != nil {
			return errors.New("terminal AI text replay retained a result candidate")
		}
	}
	return nil
}

var _ aigateway.FinalizationStore = (*gormTextFinalizationStore)(nil)
