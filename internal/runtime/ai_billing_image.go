package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/secretbox"
	storagecos "admin_back_go/internal/infra/storage/cos"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/billing"
	aiimage "admin_back_go/internal/module/ai/image"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/module/ai/requestidentity"
	airun "admin_back_go/internal/module/ai/run"
	walletmodule "admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/enum"

	_ "golang.org/x/image/webp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const imageUpperBoundFramingBytes int64 = 64
const imageGenerateTimeout = 10 * time.Minute

type imageAttemptGateway interface {
	AssembleAndQuote(context.Context, aigateway.RunRequest) (aigateway.PreparedCall, error)
	ReserveAndPrepare(context.Context, aigateway.ReserveAndPrepareInput) (aigateway.ProviderAttempt, error)
	Dispatch(context.Context, aigateway.ProviderAttempt) (aigateway.DispatchResult, error)
}

var (
	ErrImageAttemptOwnedElsewhere     = errors.New("AI image provider attempt is owned by another worker")
	ErrImageReferenceUsageUnavailable = errors.New("AI image reference edit lacks categorized authoritative usage")
)

func runImageGatewayAttempt(ctx context.Context, gateway imageAttemptGateway, request aigateway.RunRequest, attemptNo uint32, recoverPrepared bool) (aigateway.DispatchResult, error) {
	var (
		attempt aigateway.ProviderAttempt
		err     error
	)
	if recoverPrepared {
		attempt, err = gateway.ReserveAndPrepare(ctx, aigateway.ReserveAndPrepareInput{RunID: request.RunID, AttemptNo: attemptNo})
	} else {
		call, assembleErr := gateway.AssembleAndQuote(ctx, request)
		if assembleErr != nil {
			return aigateway.DispatchResult{}, assembleErr
		}
		attempt, err = gateway.ReserveAndPrepare(ctx, aigateway.ReserveAndPrepareInput{RunID: request.RunID, AttemptNo: attemptNo, NewCall: &call})
	}
	if err != nil {
		return aigateway.DispatchResult{}, err
	}
	result, err := gateway.Dispatch(ctx, attempt)
	if err != nil {
		var gatewayErr *aigateway.Error
		if errors.As(err, &gatewayErr) && (gatewayErr.Code == aigateway.ErrCodePreparedMissing || gatewayErr.Code == aigateway.ErrCodeDuplicateAttempt) {
			return aigateway.DispatchResult{}, fmt.Errorf("%w: %v", ErrImageAttemptOwnedElsewhere, err)
		}
	}
	return result, err
}

type paidImageExecution struct {
	Task                aiimage.ImageTask
	InputSnapshot       string
	PricingSnapshotJSON string
	BillingStatus       string
	BillingReason       string
	EngineType          string
	EngineBaseURL       string
	EngineAPIKeyEnc     string
}

type paidImageExecutionStore interface {
	LoadImageExecution(context.Context, uint64) (*paidImageExecution, error)
	LatestImageAttempt(context.Context, int64) (replycommand.Attempt, bool, error)
	MarkImageOutcomeUnknown(context.Context, uint64, int64, time.Time) error
	MarkImageFailure(context.Context, uint64, int64, string, string) error
}

type paidImageTaskExecutor struct {
	store      paidImageExecutionStore
	finalizer  aigateway.Finalizer
	dispatch   func(context.Context, paidImageExecution, uint32, bool) error
	now        func() time.Time
	db         *gorm.DB
	wallets    *walletmodule.GormRepository
	repository aiimage.Repository
	secretbox  secretbox.Box
	engines    aiimage.ImageEngineFactory
	objectsIn  storagecos.ObjectReader
	objectsOut storagecos.ObjectWriter
}

func newPaidImageTaskExecutor(client *database.Client, wallets *walletmodule.GormRepository, repository aiimage.Repository, engines aiimage.ImageEngineFactory, box secretbox.Box, objectsIn storagecos.ObjectReader, objectsOut storagecos.ObjectWriter) *paidImageTaskExecutor {
	if client == nil || client.Gorm == nil || wallets == nil || repository == nil || engines == nil || objectsOut == nil {
		return nil
	}
	store := &gormImageExecutionStore{db: client.Gorm}
	finalization := newGormImageFinalizationStore(client.Gorm, wallets)
	if finalization == nil {
		return nil
	}
	executor := &paidImageTaskExecutor{
		store: store, finalizer: aigateway.NewFinalizer(finalization, persistedSettlementPricer{}), now: time.Now,
		db: client.Gorm, wallets: wallets, repository: repository, secretbox: box, engines: engines,
		objectsIn: objectsIn, objectsOut: objectsOut,
	}
	executor.dispatch = executor.dispatchImage
	return executor
}

func (executor *paidImageTaskExecutor) ExecuteImageTask(ctx context.Context, taskID uint64) (string, error) {
	if executor == nil || executor.store == nil || executor.finalizer == nil || taskID == 0 {
		return "", aigateway.ErrNotConfigured
	}
	lease, ok := aiimage.TaskLeaseFromContext(ctx)
	if !ok || lease.Task.ID != taskID {
		return "", aiimage.ErrTaskLeaseLost
	}
	execution, err := executor.store.LoadImageExecution(ctx, taskID)
	if err != nil {
		return "", err
	}
	if err := validatePaidImageExecution(execution, taskID); err != nil {
		return "", err
	}
	if execution.Task.Status == aiimage.StatusSuccess || execution.Task.Status == aiimage.StatusFailed {
		return execution.Task.Status, nil
	}
	if execution.Task.Status != aiimage.StatusRunning {
		return execution.Task.Status, errors.New("AI image task is not runnable")
	}
	latest, hasAttempt, err := executor.store.LatestImageAttempt(ctx, execution.Task.RunID)
	if err != nil {
		return "", err
	}
	if hasAttempt {
		switch latest.State {
		case replycommand.AttemptSucceeded, replycommand.AttemptFailed, replycommand.AttemptCanceled, replycommand.AttemptOutcomeUnknown:
			return executor.finalizeAndLoadImageStatus(ctx, execution.Task)
		case replycommand.AttemptDispatched:
			now := time.Now()
			if executor.now != nil {
				now = executor.now()
			}
			if err := executor.store.MarkImageOutcomeUnknown(ctx, latest.ID, execution.Task.RunID, now); err != nil {
				return "", err
			}
			return executor.finalizeAndLoadImageStatus(ctx, execution.Task)
		case replycommand.AttemptPrepared:
			return executor.dispatchAndLoadImageStatus(ctx, *execution, uint32(latest.AttemptNo), true)
		default:
			return "", fmt.Errorf("unsupported image provider attempt state %q", latest.State)
		}
	}
	if strings.TrimSpace(execution.Task.LastErrorCode) != "" {
		return executor.finalizeAndLoadImageStatus(ctx, execution.Task)
	}
	return executor.dispatchAndLoadImageStatus(ctx, *execution, 1, false)
}

func validatePaidImageExecution(execution *paidImageExecution, taskID uint64) error {
	if execution == nil || execution.Task.ID != taskID || execution.Task.RunID <= 0 || execution.Task.UserID == 0 ||
		strings.TrimSpace(execution.Task.RequestID) == "" || len(execution.Task.RequestFingerprint) != sha256.Size ||
		execution.Task.RequestIdentityStatus != "replayable" || strings.TrimSpace(execution.Task.RequestIdentityMarker) != "" ||
		execution.Task.AgentID == 0 || execution.Task.ProviderIDSnapshot == 0 || strings.TrimSpace(execution.Task.ModelIDSnapshot) == "" {
		return errors.New("AI image task execution identity is invalid")
	}
	return nil
}

func (executor *paidImageTaskExecutor) dispatchAndLoadImageStatus(ctx context.Context, execution paidImageExecution, attemptNo uint32, recoverPrepared bool) (string, error) {
	if executor.dispatch == nil {
		return "", aigateway.ErrNotConfigured
	}
	if err := executor.dispatch(ctx, execution, attemptNo, recoverPrepared); err != nil {
		return "", err
	}
	loaded, err := executor.store.LoadImageExecution(ctx, execution.Task.ID)
	if err != nil {
		return "", err
	}
	if err := validatePaidImageExecution(loaded, execution.Task.ID); err != nil {
		return "", err
	}
	return loaded.Task.Status, nil
}

func (executor *paidImageTaskExecutor) finalizeAndLoadImageStatus(ctx context.Context, task aiimage.ImageTask) (string, error) {
	if err := executor.finalizer.Finalize(ctx, aigateway.FinalizeRequest{RunID: task.RunID}); err != nil {
		return "", err
	}
	loaded, err := executor.store.LoadImageExecution(ctx, task.ID)
	if err != nil {
		return "", err
	}
	if err := validatePaidImageExecution(loaded, task.ID); err != nil {
		return "", err
	}
	if loaded.Task.Status != aiimage.StatusSuccess && loaded.Task.Status != aiimage.StatusFailed {
		return "", errors.New("AI image finalization did not produce a terminal task")
	}
	return loaded.Task.Status, nil
}

type gormImageExecutionStore struct{ db *gorm.DB }

func (store *gormImageExecutionStore) LoadImageExecution(ctx context.Context, taskID uint64) (*paidImageExecution, error) {
	if store == nil || store.db == nil || taskID == 0 {
		return nil, aigateway.ErrNotConfigured
	}
	var row struct {
		aiimage.ImageTask
		InputSnapshot            string `gorm:"column:input_snapshot"`
		PricingSnapshotJSON      string `gorm:"column:pricing_snapshot_json"`
		BillingStatus            string `gorm:"column:billing_status"`
		BillingReason            string `gorm:"column:billing_reason"`
		EngineType               string `gorm:"column:engine_type"`
		EngineBaseURL            string `gorm:"column:engine_base_url"`
		EngineAPIKeyEnc          string `gorm:"column:engine_api_key_enc"`
		RunUserID                int64  `gorm:"column:run_user_id"`
		RunRequestID             string `gorm:"column:run_request_id"`
		RunRequestFingerprint    []byte `gorm:"column:run_request_fingerprint"`
		RunRequestIdentityStatus string `gorm:"column:run_request_identity_status"`
		RunRequestIdentityMarker string `gorm:"column:run_request_identity_marker"`
		RunAgentID               int64  `gorm:"column:run_agent_id"`
		RunProviderID            int64  `gorm:"column:run_provider_id"`
		RunModelID               string `gorm:"column:run_model_id"`
	}
	err := store.db.WithContext(ctx).Table("ai_image_tasks AS t").
		Select(`t.*, r.input_snapshot, r.pricing_snapshot_json, r.billing_status, r.billing_reason,
			p.engine_type, p.base_url AS engine_base_url, p.api_key_enc AS engine_api_key_enc,
			r.user_id AS run_user_id, r.request_id AS run_request_id,
			r.request_fingerprint AS run_request_fingerprint,
			r.request_identity_status AS run_request_identity_status,
			r.request_identity_marker AS run_request_identity_marker,
			r.agent_id AS run_agent_id, r.provider_id AS run_provider_id, r.model_id AS run_model_id`).
		Joins("JOIN ai_runs r ON r.id = t.run_id AND r.user_id = t.user_id AND r.request_id = t.request_id").
		Joins("LEFT JOIN ai_providers p ON p.id = t.provider_id_snapshot").
		Where("t.id = ?", taskID).Take(&row).Error
	if err != nil {
		return nil, err
	}
	if row.RunUserID <= 0 || uint64(row.RunUserID) != row.UserID || row.RunRequestID != row.RequestID ||
		len(row.RequestFingerprint) != sha256.Size || len(row.RunRequestFingerprint) != sha256.Size || !bytes.Equal(row.RequestFingerprint, row.RunRequestFingerprint) ||
		row.RequestIdentityStatus != string(requestidentity.IdentityStatusReplayable) || row.RunRequestIdentityStatus != row.RequestIdentityStatus ||
		row.RequestIdentityMarker != "" || row.RunRequestIdentityMarker != "" || row.RunAgentID <= 0 || uint64(row.RunAgentID) != row.AgentID ||
		row.RunProviderID <= 0 || uint64(row.RunProviderID) != row.ProviderIDSnapshot || strings.TrimSpace(row.RunModelID) != strings.TrimSpace(row.ModelIDSnapshot) ||
		strings.TrimSpace(row.InputSnapshot) == "" || strings.TrimSpace(row.PricingSnapshotJSON) == "" {
		return nil, errors.New("AI image accepted execution graph is inconsistent")
	}
	return &paidImageExecution{
		Task: row.ImageTask, InputSnapshot: strings.TrimSpace(row.InputSnapshot), PricingSnapshotJSON: strings.TrimSpace(row.PricingSnapshotJSON),
		BillingStatus: strings.TrimSpace(row.BillingStatus), BillingReason: strings.TrimSpace(row.BillingReason),
		EngineType: strings.TrimSpace(row.EngineType), EngineBaseURL: strings.TrimSpace(row.EngineBaseURL), EngineAPIKeyEnc: strings.TrimSpace(row.EngineAPIKeyEnc),
	}, nil
}

func (store *gormImageExecutionStore) LatestImageAttempt(ctx context.Context, runID int64) (replycommand.Attempt, bool, error) {
	if store == nil || store.db == nil || runID <= 0 {
		return replycommand.Attempt{}, false, aigateway.ErrNotConfigured
	}
	var row replycommand.Attempt
	err := store.db.WithContext(ctx).Where("run_id = ?", runID).Order("attempt_no DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return replycommand.Attempt{}, false, nil
	}
	return row, err == nil, err
}

func (store *gormImageExecutionStore) MarkImageOutcomeUnknown(ctx context.Context, attemptID uint64, runID int64, now time.Time) error {
	if store == nil || store.db == nil || attemptID == 0 || runID <= 0 {
		return aigateway.ErrNotConfigured
	}
	lease, ok := aiimage.TaskLeaseFromContext(ctx)
	if !ok {
		return aiimage.ErrTaskLeaseLost
	}
	result := store.db.WithContext(ctx).Model(&replycommand.Attempt{}).
		Where("id = ? AND run_id = ? AND state = ?", attemptID, runID, replycommand.AttemptDispatched).
		Where("EXISTS (SELECT 1 FROM ai_image_tasks t WHERE t.id = ? AND t.run_id = ? AND t.status = ? AND t.lease_owner = ? AND t.lease_token = ? AND t.lease_expires_at > ?)", lease.Task.ID, runID, aiimage.StatusRunning, lease.Owner, lease.Token, now).
		Updates(map[string]any{
			"state": replycommand.AttemptOutcomeUnknown, "dispatch_state": infraai.DispatchStateUnknown,
			"usage_json": `{"status":"unavailable"}`, "usage_status": billing.UsageStatusUnavailable,
			"error_code": "ai.provider_outcome_unknown", "result_candidate_json": nil, "finished_at": now, "updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return aiimage.ErrTaskLeaseLost
	}
	return nil
}

func (store *gormImageExecutionStore) MarkImageFailure(ctx context.Context, taskID uint64, runID int64, code, message string) error {
	if store == nil || store.db == nil || taskID == 0 || runID <= 0 || strings.TrimSpace(code) == "" || strings.TrimSpace(message) == "" {
		return aigateway.ErrNotConfigured
	}
	lease, ok := aiimage.TaskLeaseFromContext(ctx)
	if !ok || lease.Task.ID != taskID {
		return aiimage.ErrTaskLeaseLost
	}
	now := time.Now()
	result := store.db.WithContext(ctx).Model(&aiimage.ImageTask{}).
		Where("id = ? AND run_id = ? AND status = ? AND lease_owner = ? AND lease_token = ? AND lease_expires_at > ?", taskID, runID, aiimage.StatusRunning, lease.Owner, lease.Token, now).
		Updates(map[string]any{"last_error_code": strings.TrimSpace(code), "error_message": strings.TrimSpace(message), "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return aiimage.ErrTaskLeaseLost
	}
	return nil
}

func (executor *paidImageTaskExecutor) dispatchImage(ctx context.Context, execution paidImageExecution, attemptNo uint32, recoverPrepared bool) error {
	lease, ok := aiimage.TaskLeaseFromContext(ctx)
	if !ok || lease.Task.ID != execution.Task.ID {
		return aiimage.ErrTaskLeaseLost
	}
	snapshot, err := aiimage.DecodeProviderInputSnapshot(execution.InputSnapshot)
	if err != nil {
		return executor.failImageTask(ctx, execution.Task, imageErrorCodeConfiguration, "AI图片输入快照无效")
	}
	if len(snapshot.Attachments) > 0 {
		return executor.failImageTask(ctx, execution.Task, imageErrorCodePriceUnavailable, "参考图编辑暂不可计费")
	}
	identity, err := aiimage.RequestIdentityInput(execution.Task.UserID, execution.Task.AgentID, snapshot)
	if err != nil {
		return executor.failImageTask(ctx, execution.Task, imageErrorCodeConfiguration, "AI图片请求身份快照无效")
	}
	fingerprint, err := requestidentity.Fingerprint(identity)
	if err != nil || requestidentity.CompareForReplay(requestidentity.IdentityStatus(execution.Task.RequestIdentityStatus), digestFromBytes(execution.Task.RequestFingerprint), fingerprint) != nil {
		return executor.failImageTask(ctx, execution.Task, imageErrorCodeConfiguration, "AI图片请求身份与接受快照不一致")
	}
	if executor.engines == nil || strings.TrimSpace(execution.EngineAPIKeyEnc) == "" {
		return executor.failImageTask(ctx, execution.Task, imageErrorCodeConfiguration, "AI供应商配置不完整")
	}
	apiKey, err := executor.secretbox.Decrypt(execution.EngineAPIKeyEnc)
	if err != nil || strings.TrimSpace(apiKey) == "" {
		return executor.failImageTask(ctx, execution.Task, imageErrorCodeConfiguration, "AI供应商API Key未配置")
	}
	engine := executor.engines.NewImageEngine(aiimage.ImageEngineConfig{
		EngineType: execution.EngineType, BaseURL: execution.EngineBaseURL, APIKey: strings.TrimSpace(apiKey), Timeout: imageGenerateTimeout,
	})
	transport, ok := engine.(infraai.PreparedImageEngine)
	if !ok || transport == nil {
		return executor.failImageTask(ctx, execution.Task, imageErrorCodeConfiguration, "AI供应商不支持prepared图片请求")
	}
	destination, err := executor.loadImageObjectDestination(ctx)
	if err != nil {
		return executor.failImageTask(ctx, execution.Task, imageErrorCodeConfiguration, "AI图片对象存储配置不可用")
	}
	inputs, mask, err := loadPreparedImageAssets(ctx, executor.objectsIn, destination, snapshot.Attachments)
	if err != nil {
		return executor.failImageTask(ctx, execution.Task, imageErrorCodeConfiguration, "AI图片附件快照不可用")
	}
	input := infraai.ImageInput{
		Model: snapshot.Model, Prompt: snapshot.Prompt, Size: snapshot.Size, Quality: snapshot.Quality,
		OutputFormat: snapshot.OutputFormat, OutputCompression: snapshot.OutputCompression, Moderation: snapshot.Moderation,
		N: snapshot.N, InputAssets: inputs, MaskAsset: mask,
	}
	provider := newPreparedImageProvider(transport, inputs, mask, &cosImageCandidateWriter{objects: executor.objectsOut, destination: destination}, execution.Task.ID)
	attempts := gormImageAttemptStore{db: executor.db, taskID: execution.Task.ID, lease: lease}
	gateway := aigateway.New(aigateway.Dependencies{
		Assembler: paidImageAssembler{transport: transport, input: input}, Quotes: aigateway.PersistedQuoteValidator{},
		Transactions: gormGatewayTransactions{db: executor.db}, Runs: gormGatewayRunStore{db: executor.db}, PriorUsage: gormGatewayPriorUsagePricer{},
		Reserve: gormGatewayReserveParticipant{wallets: executor.wallets}, Failures: gormImageReserveFailureRecorder{taskID: execution.Task.ID, lease: lease},
		Attempts: attempts, Provider: provider, Owner: gormImageOwnerGuard{taskID: execution.Task.ID, lease: lease},
	})
	request := aigateway.RunRequest{
		UserID: int64(execution.Task.UserID), RunID: execution.Task.RunID, RequestID: execution.Task.RequestID, Identity: identity,
	}
	result, err := runImageGatewayAttempt(ctx, gateway, request, attemptNo, recoverPrepared)
	if err != nil {
		if errors.Is(err, aiimage.ErrTaskLeaseLost) || errors.Is(context.Cause(ctx), aiimage.ErrTaskLeaseLost) {
			return aiimage.ErrTaskLeaseLost
		}
		if errors.Is(err, ErrImageAttemptOwnedElsewhere) {
			return err
		}
		if isPaidInsufficientBalance(err) || hasPersistedImageTerminalAttempt(ctx, executor.db, execution.Task.RunID, attemptNo) {
			return executor.finalizer.Finalize(ctx, aigateway.FinalizeRequest{RunID: execution.Task.RunID})
		}
		var gatewayErr *aigateway.Error
		if errors.As(err, &gatewayErr) && gatewayErr.Code == aigateway.ErrCodeFingerprintConflict {
			return executor.failImageTask(ctx, execution.Task, imageErrorCodeConfiguration, "AI图片请求身份与接受快照不一致")
		}
		code := imagePreDispatchErrorCode(err)
		message := "AI图片prepared请求失败"
		if code == imageErrorCodePriceUnavailable {
			message = "AI模型价格不可用"
		} else if code == imageErrorCodeUnsafeUpperBound {
			message = "AI图片请求缺少安全用量上界"
		}
		return executor.failImageTask(ctx, execution.Task, code, message)
	}
	if result.ResultCandidateJSON == nil {
		return executor.failImageTask(ctx, execution.Task, imageErrorCodeProviderFailed, "AI供应商返回的图片结果候选无效")
	}
	return executor.finalizer.Finalize(ctx, aigateway.FinalizeRequest{RunID: execution.Task.RunID})
}

func (executor *paidImageTaskExecutor) failImageTask(ctx context.Context, task aiimage.ImageTask, code, message string) error {
	if err := executor.store.MarkImageFailure(ctx, task.ID, task.RunID, code, message); err != nil {
		return err
	}
	return executor.finalizer.Finalize(ctx, aigateway.FinalizeRequest{RunID: task.RunID})
}

func (executor *paidImageTaskExecutor) loadImageObjectDestination(ctx context.Context) (imageObjectDestination, error) {
	if executor == nil || executor.repository == nil || executor.objectsOut == nil {
		return imageObjectDestination{}, aigateway.ErrNotConfigured
	}
	config, err := executor.repository.LoadUploadConfig(ctx)
	if err != nil {
		return imageObjectDestination{}, err
	}
	if config == nil || strings.TrimSpace(config.Driver) != aiimage.StorageProviderCOS {
		return imageObjectDestination{}, errors.New("AI image COS destination is not configured")
	}
	secretID, err := executor.secretbox.Decrypt(strings.TrimSpace(config.SecretIDEnc))
	if err != nil || strings.TrimSpace(secretID) == "" {
		return imageObjectDestination{}, errors.New("AI image COS SecretID is unavailable")
	}
	secretKey, err := executor.secretbox.Decrypt(strings.TrimSpace(config.SecretKeyEnc))
	if err != nil || strings.TrimSpace(secretKey) == "" {
		return imageObjectDestination{}, errors.New("AI image COS SecretKey is unavailable")
	}
	return normalizeImageObjectDestination(imageObjectDestination{
		SecretID: secretID, SecretKey: secretKey, Bucket: config.Bucket, Region: config.Region,
		Endpoint: config.Endpoint, BucketDomain: config.BucketDomain,
	}), nil
}

const (
	imageErrorCodeConfiguration       = "ai.image.configuration_error"
	imageErrorCodePriceUnavailable    = "ai.billing.price_unavailable"
	imageErrorCodeUnsafeUpperBound    = "ai.billing.unsafe_upper_bound"
	imageErrorCodeInsufficientBalance = "ai.billing.insufficient_balance"
	imageErrorCodeUsageIncomplete     = "ai.billing.usage_incomplete"
	imageErrorCodeProviderFailed      = "ai.provider_failed"
)

func imagePreDispatchErrorCode(err error) string {
	if errors.Is(err, ErrImageReferenceUsageUnavailable) || errors.Is(err, pricing.ErrPriceUnavailable) || errors.Is(err, pricing.ErrMissingModel) ||
		errors.Is(err, pricing.ErrInvalidCatalog) || errors.Is(err, pricing.ErrUnsupportedUsage) || errors.Is(err, pricing.ErrInvalidMultiplier) {
		return imageErrorCodePriceUnavailable
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(message, "upper-bound") || strings.Contains(message, "upper bound") || strings.Contains(message, "safe image") || strings.Contains(message, "prepared image request") {
		return imageErrorCodeUnsafeUpperBound
	}
	return imageErrorCodeConfiguration
}

func hasPersistedImageTerminalAttempt(ctx context.Context, db *gorm.DB, runID int64, attemptNo uint32) bool {
	if db == nil {
		return false
	}
	var count int64
	db.WithContext(ctx).Model(&replycommand.Attempt{}).
		Where("run_id = ? AND attempt_no = ? AND state IN ?", runID, attemptNo, []replycommand.AttemptState{
			replycommand.AttemptSucceeded, replycommand.AttemptFailed, replycommand.AttemptCanceled, replycommand.AttemptOutcomeUnknown,
		}).Count(&count)
	return count == 1
}

type gormImageReserveFailureRecorder struct {
	taskID uint64
	lease  aiimage.TaskLease
}

func (recorder gormImageReserveFailureRecorder) RecordReserveFailure(ctx context.Context, transaction aigateway.Transaction, runID int64, trigger aigateway.FinalizationTrigger) error {
	tx, err := gatewayTransactionDB(transaction)
	if err != nil {
		return err
	}
	if recorder.taskID == 0 || (trigger != aigateway.TriggerInitialInsufficient && trigger != aigateway.TriggerContinuationTopUpInsufficient) {
		return aigateway.ErrNotConfigured
	}
	now := time.Now()
	result := tx.WithContext(ctx).Model(&aiimage.ImageTask{}).
		Where("id = ? AND run_id = ? AND status = ? AND lease_owner = ? AND lease_token = ? AND lease_expires_at > ?", recorder.taskID, runID, aiimage.StatusRunning, recorder.lease.Owner, recorder.lease.Token, now).
		Updates(map[string]any{
			"last_error_code": imageErrorCodeInsufficientBalance, "error_message": "余额不足，请充值后重试", "updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return aiimage.ErrTaskLeaseLost
	}
	return nil
}

type gormImageOwnerGuard struct {
	taskID uint64
	lease  aiimage.TaskLease
}

func (guard gormImageOwnerGuard) EnsureRunnable(ctx context.Context, transaction aigateway.Transaction, runID int64) error {
	tx, err := gatewayTransactionDB(transaction)
	if err != nil {
		return err
	}
	if guard.taskID == 0 || runID <= 0 {
		return aigateway.ErrNotConfigured
	}
	var task aiimage.ImageTask
	err = tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND run_id = ? AND status = ? AND lease_owner = ? AND lease_token = ? AND lease_expires_at > ?", guard.taskID, runID, aiimage.StatusRunning, guard.lease.Owner, guard.lease.Token, time.Now()).
		Where("EXISTS (SELECT 1 FROM ai_runs r WHERE r.id = ? AND r.status = ?)", runID, enum.AIRunStatusRunning).
		First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return aiimage.ErrTaskLeaseLost
	}
	return err
}

type gormImageAttemptStore struct {
	db     *gorm.DB
	taskID uint64
	lease  aiimage.TaskLease
}

func (store gormImageAttemptStore) PutPrepared(ctx context.Context, transaction aigateway.Transaction, attempt aigateway.ProviderAttempt) (aigateway.PreparedWriteResult, error) {
	tx, err := gatewayTransactionDB(transaction)
	if err != nil {
		return aigateway.PreparedWriteResult{}, err
	}
	if store.db == nil || store.taskID == 0 {
		return aigateway.PreparedWriteResult{}, aigateway.ErrNotConfigured
	}
	var task aiimage.ImageTask
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND run_id = ? AND status = ? AND lease_owner = ? AND lease_token = ? AND lease_expires_at > ?", store.taskID, attempt.RunID, aiimage.StatusRunning, store.lease.Owner, store.lease.Token, time.Now()).
		First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return aigateway.PreparedWriteResult{}, aiimage.ErrTaskLeaseLost
		}
		return aigateway.PreparedWriteResult{}, err
	}
	quoteJSON, err := json.Marshal(attempt.Quote)
	if err != nil {
		return aigateway.PreparedWriteResult{}, err
	}
	now := time.Now()
	row := replycommand.Attempt{
		RunID: attempt.RunID, AttemptNo: uint(attempt.AttemptNo), IdempotencyKey: attempt.IdempotencyKey,
		State: replycommand.AttemptPrepared, PreparedRequestJSON: string(attempt.PreparedRequest),
		PreparedRequestSHA256: append([]byte(nil), attempt.RequestSHA256[:]...), QuoteJSON: string(quoteJSON),
		UsageJSON: `{"status":"unavailable"}`, UsageStatus: string(billing.UsageStatusUnavailable),
		DispatchState: infraai.DispatchStateNotDispatched, CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return aigateway.PreparedWriteResult{}, err
	}
	persisted, err := gatewayAttemptFromRow(row)
	if err != nil {
		return aigateway.PreparedWriteResult{}, err
	}
	return aigateway.PreparedWriteResult{Attempt: persisted, Inserted: true}, nil
}

func (store gormImageAttemptStore) generic() gormGatewayAttemptStore {
	return gormGatewayAttemptStore{db: store.db}
}

func (store gormImageAttemptStore) GetPreparedForUpdate(ctx context.Context, transaction aigateway.Transaction, runID int64, attemptNo uint32) (aigateway.ProviderAttempt, error) {
	return store.generic().GetPreparedForUpdate(ctx, transaction, runID, attemptNo)
}

func (store gormImageAttemptStore) GetDispatchedForUpdate(ctx context.Context, transaction aigateway.Transaction, runID int64, attemptNo uint32) (aigateway.ProviderAttempt, error) {
	return store.generic().GetDispatchedForUpdate(ctx, transaction, runID, attemptNo)
}

func (store gormImageAttemptStore) MarkDispatched(ctx context.Context, transaction aigateway.Transaction, runID int64, attemptNo uint32) (bool, error) {
	return store.generic().MarkDispatched(ctx, transaction, runID, attemptNo)
}

func (store gormImageAttemptStore) GetTerminalOutcome(ctx context.Context, transaction aigateway.Transaction, runID int64, attemptNo uint32) (aigateway.DispatchResult, error) {
	return store.generic().GetTerminalOutcome(ctx, transaction, runID, attemptNo)
}

func (store gormImageAttemptStore) RecordTerminalOutcome(ctx context.Context, transaction aigateway.Transaction, runID int64, attemptNo uint32, outcome aigateway.DispatchResult) (aigateway.TerminalOutcomeWriteResult, error) {
	tx, err := gatewayTransactionDB(transaction)
	if err != nil {
		return aigateway.TerminalOutcomeWriteResult{}, err
	}
	var task aiimage.ImageTask
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND run_id = ? AND status = ? AND lease_owner = ? AND lease_token = ? AND lease_expires_at > ?", store.taskID, runID, aiimage.StatusRunning, store.lease.Owner, store.lease.Token, time.Now()).
		First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return aigateway.TerminalOutcomeWriteResult{}, aiimage.ErrTaskLeaseLost
		}
		return aigateway.TerminalOutcomeWriteResult{}, err
	}
	return store.generic().RecordTerminalOutcome(ctx, transaction, runID, attemptNo, outcome)
}

var (
	_ paidImageExecutionStore          = (*gormImageExecutionStore)(nil)
	_ aigateway.ReserveFailureRecorder = gormImageReserveFailureRecorder{}
	_ aigateway.OwnerGuard             = gormImageOwnerGuard{}
	_ aigateway.AttemptStore           = gormImageAttemptStore{}
	_ aiimage.TaskExecutor             = (*paidImageTaskExecutor)(nil)
)

type gormImageFinalizationStore struct {
	db      *gorm.DB
	wallets *walletmodule.GormRepository
	now     func() time.Time
}

func newGormImageFinalizationStore(db *gorm.DB, wallets *walletmodule.GormRepository) *gormImageFinalizationStore {
	if db == nil || wallets == nil {
		return nil
	}
	return &gormImageFinalizationStore{db: db, wallets: wallets, now: time.Now}
}

func (store *gormImageFinalizationStore) WithLockedSettlement(ctx context.Context, runID int64, decide func(aigateway.FinalizationFacts) (aigateway.SettlementDecision, error)) (aigateway.FinalizationApplyResult, error) {
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
	lease, hasLease := aiimage.TaskLeaseFromContext(ctx)
	if !hasLease {
		return aigateway.FinalizationApplyResult{}, aiimage.ErrTaskLeaseLost
	}
	var applied, replayed bool
	err := store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		run, charge, wallet, hold, err := lockChatSettlementMoneyGraph(ctx, tx, runID)
		if err != nil {
			return err
		}
		task, attempts, outputs, err := lockImageSettlementBusinessGraph(ctx, tx, run, lease, now)
		if err != nil {
			return err
		}
		if terminalChatBilling(run, charge) {
			if err := validateImageFinalizationReplay(run, charge, wallet, hold, task, attempts, outputs); err != nil {
				return err
			}
			if err := airun.ProjectTerminalDashboardFacts(ctx, tx, run.ID); err != nil {
				return err
			}
			replayed = true
			return nil
		}
		if run.Status != enum.AIRunStatusRunning || charge.Status != billing.ChargeStatusOpen || charge.FinalizedAt != nil || task.Status != aiimage.StatusRunning {
			return errors.New("AI image settlement is neither open nor a valid terminal replay")
		}
		if len(outputs) != 0 {
			return errors.New("unsettled AI image task already has published outputs")
		}
		attempts, err = normalizeImageAttemptsForFinalization(ctx, tx, task, attempts, now)
		if err != nil {
			return err
		}
		facts, err := buildImageFinalizationFacts(run, charge, hold, task, attempts)
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
		if err := finalizeImageTaskRunAndCharge(ctx, tx, task, attempts, run, charge, facts, decision, now); err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Model(&replycommand.Attempt{}).
			Where("run_id = ? AND result_candidate_json IS NOT NULL", run.ID).Update("result_candidate_json", nil).Error; err != nil {
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

func lockImageSettlementBusinessGraph(ctx context.Context, tx *gorm.DB, run airun.Run, lease aiimage.TaskLease, now time.Time) (aiimage.ImageTask, []replycommand.Attempt, []aiimage.ImageFile, error) {
	var task aiimage.ImageTask
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND run_id = ? AND user_id = ? AND request_id = ? AND lease_owner = ? AND lease_token = ? AND lease_expires_at > ?", lease.Task.ID, run.ID, run.UserID, run.RequestID, lease.Owner, lease.Token, now).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return aiimage.ImageTask{}, nil, nil, aiimage.ErrTaskLeaseLost
		}
		return aiimage.ImageTask{}, nil, nil, err
	}
	var attempts []replycommand.Attempt
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("run_id = ?", run.ID).Order("attempt_no ASC").Find(&attempts).Error; err != nil {
		return aiimage.ImageTask{}, nil, nil, err
	}
	var outputs []aiimage.ImageFile
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("task_id = ? AND role = ?", task.ID, aiimage.FileRoleOutput).Order("sort_order ASC, id ASC").Find(&outputs).Error; err != nil {
		return aiimage.ImageTask{}, nil, nil, err
	}
	return task, attempts, outputs, nil
}

func normalizeImageAttemptsForFinalization(ctx context.Context, tx *gorm.DB, task aiimage.ImageTask, attempts []replycommand.Attempt, now time.Time) ([]replycommand.Attempt, error) {
	if len(attempts) == 0 || strings.TrimSpace(task.LastErrorCode) == "" {
		return attempts, nil
	}
	latest := &attempts[len(attempts)-1]
	if latest.State != replycommand.AttemptPrepared || strings.TrimSpace(latest.DispatchState) != infraai.DispatchStateNotDispatched {
		return attempts, nil
	}
	result := tx.WithContext(ctx).Model(&replycommand.Attempt{}).
		Where("id = ? AND state = ?", latest.ID, replycommand.AttemptPrepared).
		Updates(map[string]any{
			"state": replycommand.AttemptCanceled, "dispatch_state": infraai.DispatchStateNotDispatched,
			"usage_status": billing.UsageStatusUnavailable, "usage_json": `{"status":"unavailable"}`,
			"result_candidate_json": nil, "provider_request_id": "", "response_sha256": "",
			"error_code": task.LastErrorCode, "finished_at": now, "updated_at": now,
		})
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

func deriveImageFinalizationTrigger(task aiimage.ImageTask, attempts []replycommand.Attempt) (aigateway.FinalizationTrigger, error) {
	code := strings.TrimSpace(task.LastErrorCode)
	if code == imageErrorCodeInsufficientBalance {
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

func buildImageFinalizationFacts(run airun.Run, charge billing.UsageCharge, hold *walletmodule.Hold, task aiimage.ImageTask, attempts []replycommand.Attempt) (aigateway.FinalizationFacts, error) {
	snapshot, err := gatewayRunSnapshot(run)
	if err != nil {
		return aigateway.FinalizationFacts{}, err
	}
	trigger, err := deriveImageFinalizationTrigger(task, attempts)
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
			return aigateway.FinalizationFacts{}, errors.New("image provider attempt identity is invalid")
		}
		usage, usageErr := usageFromAttempt(row)
		if usageErr != nil {
			usage = infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
		}
		auditOnlyPrepared := trigger == aigateway.TriggerPreDispatchFailed && row.State == replycommand.AttemptCanceled && row.DispatchState == infraai.DispatchStateNotDispatched
		if !auditOnlyPrepared {
			if _, quoteErr := paidQuoteFromAttempt(row); quoteErr != nil {
				return aigateway.FinalizationFacts{}, fmt.Errorf("load image attempt %d quote: %w", row.ID, quoteErr)
			}
			if len(row.PreparedRequestSHA256) != sha256.Size || sha256.Sum256([]byte(row.PreparedRequestJSON)) != digestFromBytes(row.PreparedRequestSHA256) {
				return aigateway.FinalizationFacts{}, fmt.Errorf("image attempt %d prepared request evidence is invalid", row.ID)
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

func finalizeImageTaskRunAndCharge(ctx context.Context, tx *gorm.DB, task aiimage.ImageTask, attempts []replycommand.Attempt, run airun.Run, charge billing.UsageCharge, facts aigateway.FinalizationFacts, decision aigateway.SettlementDecision, now time.Time) error {
	taskUpdates := map[string]any{
		"status": aiimage.StatusFailed, "actual_params_json": nil, "raw_response_json": nil,
		"finished_at": now, "elapsed_ms": int(finalizationDurationMS(run.StartedAt, now)), "updated_at": now,
		"lease_owner": nil, "lease_expires_at": nil,
	}
	if decision.RunStatus == enum.AIRunStatusSuccess && decision.CandidateAction == aigateway.SettlementCandidatePublish {
		attemptNo, err := imageCandidateAttemptNo(decision.Candidate.AttemptID, attempts)
		if err != nil {
			return err
		}
		files, actualParamsJSON, rawResponseJSON, err := imageFilesFromCandidate(decision.Candidate.JSON, task.ID, attemptNo, now)
		if err != nil {
			return err
		}
		if len(files) == 0 {
			return errors.New("settled AI image candidate has no outputs")
		}
		if err := tx.WithContext(ctx).Create(&files).Error; err != nil {
			return err
		}
		taskUpdates["status"] = aiimage.StatusSuccess
		taskUpdates["last_error_code"] = ""
		taskUpdates["error_message"] = ""
		taskUpdates["actual_params_json"] = actualParamsJSON
		taskUpdates["raw_response_json"] = rawResponseJSON
	} else {
		code, message := imageFinalizationFailure(task, facts, decision)
		taskUpdates["last_error_code"] = code
		taskUpdates["error_message"] = message
	}
	taskUpdate := tx.WithContext(ctx).Model(&aiimage.ImageTask{}).
		Where("id = ? AND run_id = ? AND status = ? AND lease_owner = ? AND lease_token = ? AND lease_expires_at > ?", task.ID, run.ID, aiimage.StatusRunning, task.LeaseOwner, task.LeaseToken, now).Updates(taskUpdates)
	if taskUpdate.Error != nil {
		return taskUpdate.Error
	}
	if taskUpdate.RowsAffected != 1 {
		return aiimage.ErrTaskLeaseLost
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
		_, message := imageFinalizationFailure(task, facts, decision)
		runUpdates["error_message"] = truncateFinalizationMessage(message)
	}
	runUpdate := tx.WithContext(ctx).Model(&airun.Run{}).
		Where("id = ? AND status = ? AND billing_status IN ?", run.ID, enum.AIRunStatusRunning, []billing.BillingStatus{billing.BillingStatusPending, billing.BillingStatusHeld}).
		Updates(runUpdates)
	if runUpdate.Error != nil {
		return runUpdate.Error
	}
	if runUpdate.RowsAffected != 1 {
		return errors.New("AI image Run terminal compare-and-set was rejected")
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
		return errors.New("AI image usage charge terminal compare-and-set was rejected")
	}
	return airun.ProjectTerminalDashboardFacts(ctx, tx, run.ID)
}

func imageCandidateAttemptNo(attemptID int64, attempts []replycommand.Attempt) (uint32, error) {
	for _, attempt := range attempts {
		if int64(attempt.ID) == attemptID && attempt.AttemptNo > 0 {
			return uint32(attempt.AttemptNo), nil
		}
	}
	return 0, errors.New("AI image result candidate attempt is missing")
}

func imageFinalizationFailure(task aiimage.ImageTask, facts aigateway.FinalizationFacts, decision aigateway.SettlementDecision) (string, string) {
	if facts.Trigger == aigateway.TriggerInitialInsufficient || facts.Trigger == aigateway.TriggerContinuationTopUpInsufficient {
		return imageErrorCodeInsufficientBalance, "余额不足，请充值后重试"
	}
	if decision.BillingReason == billing.BillingReasonUnbilledUsageIncomplete {
		return imageErrorCodeUsageIncomplete, "AI供应商未返回完整用量，本次未扣费"
	}
	if decision.BillingReason == billing.BillingReasonUnbilledOverHold {
		return "ai.billing.over_hold", "AI用量超过冻结上限，本次未扣费"
	}
	if code := strings.TrimSpace(task.LastErrorCode); code != "" {
		message := strings.TrimSpace(task.ErrorMessage)
		if message == "" {
			message = "AI图片生成配置错误"
		}
		return code, message
	}
	if facts.Trigger == aigateway.TriggerOutcomeUnknown {
		return "ai.provider_outcome_unknown", "上游结果无法确认，本次未扣费"
	}
	return imageErrorCodeProviderFailed, "AI图片生成失败，本次未扣费"
}

func validateImageFinalizationReplay(run airun.Run, charge billing.UsageCharge, wallet *walletmodule.Wallet, hold *walletmodule.Hold, task aiimage.ImageTask, attempts []replycommand.Attempt, outputs []aiimage.ImageFile) error {
	if charge.FinalizedAt == nil || charge.RunID != run.ID || charge.UserID != run.UserID || task.RunID != run.ID || task.FinishedAt == nil {
		return errors.New("terminal AI image replay facts are invalid")
	}
	switch billing.BillingStatus(run.BillingStatus) {
	case billing.BillingStatusSettled:
		if charge.Status != billing.ChargeStatusSettled || task.Status != aiimage.StatusSuccess || len(outputs) == 0 || hold == nil || wallet == nil ||
			hold.Status != walletmodule.HoldCaptured || hold.HeldUnits != 0 || hold.CapturedUnits != charge.ActualUnits {
			return errors.New("settled AI image replay facts are inconsistent")
		}
	case billing.BillingStatusReleased:
		if charge.Status != billing.ChargeStatusReleased || task.Status != aiimage.StatusFailed || len(outputs) != 0 || charge.ActualUnits != 0 ||
			(hold != nil && (hold.Status != walletmodule.HoldReleased || hold.HeldUnits != 0 || hold.CapturedUnits != 0)) {
			return errors.New("released AI image replay facts are inconsistent")
		}
	case billing.BillingStatusUnbilled:
		if charge.Status != billing.ChargeStatusUnbilled || task.Status != aiimage.StatusFailed || len(outputs) != 0 || charge.ActualUnits != 0 ||
			wallet == nil || hold == nil || hold.WalletID != wallet.ID || hold.Status != walletmodule.HoldReleased || hold.HeldUnits != 0 || hold.CapturedUnits != 0 {
			return errors.New("unbilled AI image replay facts are inconsistent")
		}
	default:
		return errors.New("AI image replay billing status is not terminal")
	}
	for _, attempt := range attempts {
		if attempt.ResultCandidateJSON != nil {
			return errors.New("terminal AI image replay retained a result candidate")
		}
	}
	return nil
}

var _ aigateway.FinalizationStore = (*gormImageFinalizationStore)(nil)

type paidImageAssembler struct {
	transport infraai.PreparedImageEngine
	input     infraai.ImageInput
}

func validatePaidImageCapabilities(capabilities infraai.CapabilityMetadata) error {
	if !capabilities.SupportsIdempotencyHeader || strings.TrimSpace(capabilities.SafeInputUpperBoundStrategy) != infraai.SafeImageUpperBoundStrategyLogicalAndAttachmentBytesV1 {
		return &aigateway.Error{Code: aigateway.ErrCodeInvalidPrepared, Message: "provider lacks required image idempotency or upper-bound capability", Status: 409}
	}
	var supportsInput, supportsOutput bool
	for _, rawIdentity := range capabilities.SupportedUsageIdentities {
		identity, err := rawIdentity.Normalized()
		if err != nil {
			return &aigateway.Error{Code: aigateway.ErrCodeInvalidPrepared, Message: "provider image usage capability is invalid", Status: 409}
		}
		if identity.Unit != "token" || identity.TierKey != "" {
			continue
		}
		switch identity.Category {
		case infraai.UsageCategoryInput:
			supportsInput = true
		case infraai.UsageCategoryOutput:
			supportsOutput = true
		}
	}
	if !supportsInput || !supportsOutput {
		return &aigateway.Error{Code: aigateway.ErrCodeInvalidPrepared, Message: "provider lacks required image input/output token usage capability", Status: 409}
	}
	return nil
}

func (assembler paidImageAssembler) AssembleAndQuote(ctx context.Context, run aigateway.RunSnapshot, _ aigateway.RunRequest) (aigateway.PreparedCall, error) {
	if assembler.transport == nil {
		return aigateway.PreparedCall{}, aigateway.ErrNotConfigured
	}
	if err := validatePaidImageCapabilities(assembler.transport.Capabilities()); err != nil {
		return aigateway.PreparedCall{}, err
	}
	if len(assembler.input.InputAssets) > 0 || assembler.input.MaskAsset != nil {
		return aigateway.PreparedCall{}, ErrImageReferenceUsageUnavailable
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return aigateway.PreparedCall{}, err
		}
	}
	snapshot, err := aigateway.ParsePricingSnapshot(run.PricingSnapshotJSON)
	if err != nil {
		return aigateway.PreparedCall{}, err
	}
	input := clonePreparedImageInput(assembler.input)
	input.Model = snapshot.RequestedModelID
	body, err := assembler.transport.PrepareImageRequest(input)
	if err != nil {
		return aigateway.PreparedCall{}, err
	}
	inputBound, err := safeImageInputUpperBound(body, input.InputAssets, input.MaskAsset)
	if err != nil {
		return aigateway.PreparedCall{}, err
	}
	items := []billing.UsageItem{
		{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: inputBound},
		{Category: billing.UsageCategoryOutputText, Unit: "token", Quantity: int64(snapshot.EffectiveMaxOutputTokens)},
	}
	quoted, err := quotePricingSnapshot(snapshot, items, "image-upper-bound")
	if err != nil {
		return aigateway.PreparedCall{}, err
	}
	if quoted.AmountUnits <= 0 {
		return aigateway.PreparedCall{}, errors.New("AI image request upper-bound price must be positive")
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

func safeImageInputUpperBound(body []byte, inputs []infraai.ImageAsset, mask *infraai.ImageAsset) (int64, error) {
	if len(body) == 0 {
		return 0, errors.New("prepared image request body is required")
	}
	bound := int64(len(body))
	add := func(asset infraai.ImageAsset) error {
		if asset.SizeBytes <= 0 || int64(len(asset.Data)) != asset.SizeBytes {
			return errors.New("prepared image attachment size evidence is invalid")
		}
		if bound > math.MaxInt64-asset.SizeBytes {
			return errors.New("prepared image request upper bound overflows")
		}
		bound += asset.SizeBytes
		return nil
	}
	for _, asset := range inputs {
		if err := add(asset); err != nil {
			return 0, err
		}
	}
	if mask != nil {
		if err := add(*mask); err != nil {
			return 0, err
		}
	}
	if bound > math.MaxInt64-imageUpperBoundFramingBytes {
		return 0, errors.New("prepared image request upper bound overflows")
	}
	return bound + imageUpperBoundFramingBytes, nil
}

type imageCandidateWriter interface {
	WriteImageCandidate(context.Context, uint64, uint32, *infraai.ImageResult) (string, error)
}

type preparedImageProvider struct {
	transport infraai.PreparedImageEngine
	inputs    []infraai.ImageAsset
	mask      *infraai.ImageAsset
	writer    imageCandidateWriter
	taskID    uint64
}

func newPreparedImageProvider(transport infraai.PreparedImageEngine, inputs []infraai.ImageAsset, mask *infraai.ImageAsset, writer imageCandidateWriter, taskID uint64) *preparedImageProvider {
	return &preparedImageProvider{transport: transport, inputs: cloneImageAssets(inputs), mask: cloneImageAsset(mask), writer: writer, taskID: taskID}
}

func (provider *preparedImageProvider) Capabilities() infraai.CapabilityMetadata {
	if provider == nil || provider.transport == nil {
		return infraai.CapabilityMetadata{}
	}
	return provider.transport.Capabilities()
}

func (provider *preparedImageProvider) ProvePreparedUpperBound(ctx context.Context, attempt aigateway.ProviderAttempt) (aigateway.PreparedUpperBoundProof, error) {
	if provider == nil || provider.transport == nil {
		return aigateway.PreparedUpperBoundProof{}, aigateway.ErrNotConfigured
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return aigateway.PreparedUpperBoundProof{}, err
		}
	}
	if strings.TrimSpace(provider.transport.Capabilities().SafeInputUpperBoundStrategy) != infraai.SafeImageUpperBoundStrategyLogicalAndAttachmentBytesV1 {
		return aigateway.PreparedUpperBoundProof{}, &aigateway.Error{Code: aigateway.ErrCodeInvalidPrepared, Message: "provider safe image upper-bound strategy is unsupported", Status: 409}
	}
	inputBound, err := safeImageInputUpperBound(attempt.PreparedRequest, provider.inputs, provider.mask)
	if err != nil {
		return aigateway.PreparedUpperBoundProof{}, &aigateway.Error{Code: aigateway.ErrCodeInvalidPrepared, Message: err.Error(), Status: 409}
	}
	items := make([]billing.UsageItem, len(attempt.Quote.UpperBoundItems))
	var inputItems, outputItems int
	for index, raw := range attempt.Quote.UpperBoundItems {
		item, normalizeErr := raw.Normalized()
		if normalizeErr != nil {
			return aigateway.PreparedUpperBoundProof{}, &aigateway.Error{Code: aigateway.ErrCodeInvalidPrepared, Message: "image quote contains invalid upper-bound usage", Status: 409}
		}
		switch {
		case item.Category == billing.UsageCategoryInputText && item.Unit == "token":
			inputItems++
			if item.Quantity != inputBound {
				return aigateway.PreparedUpperBoundProof{}, &aigateway.Error{Code: aigateway.ErrCodeInvalidPrepared, Message: "quoted image input bound differs from prepared request proof", Status: 409}
			}
		case item.Category == billing.UsageCategoryOutputText && item.Unit == "token":
			outputItems++
			if item.Quantity != int64(attempt.Quote.EffectiveMaxOutputTokens) {
				return aigateway.PreparedUpperBoundProof{}, &aigateway.Error{Code: aigateway.ErrCodeInvalidPrepared, Message: "quoted image output bound differs from effective output cap", Status: 409}
			}
		default:
			return aigateway.PreparedUpperBoundProof{}, &aigateway.Error{Code: aigateway.ErrCodeInvalidPrepared, Message: "prepared image quote contains an unsupported upper-bound item", Status: 409}
		}
		items[index] = item
	}
	if inputItems != 1 || outputItems != 1 {
		return aigateway.PreparedUpperBoundProof{}, &aigateway.Error{Code: aigateway.ErrCodeInvalidPrepared, Message: "prepared image quote requires one input and one output token bound", Status: 409}
	}
	return aigateway.PreparedUpperBoundProof{RequestSHA256: attempt.RequestSHA256, Strategy: infraai.SafeImageUpperBoundStrategyLogicalAndAttachmentBytesV1, Items: items}, nil
}

func (provider *preparedImageProvider) Dispatch(ctx context.Context, attempt aigateway.ProviderAttempt) (aigateway.DispatchResult, error) {
	if provider == nil || provider.transport == nil || provider.writer == nil || provider.taskID == 0 {
		return aigateway.DispatchResult{}, aigateway.ErrNotConfigured
	}
	result, err := provider.transport.GeneratePreparedImages(ctx, infraai.PreparedImageRequest{
		Body: append([]byte(nil), attempt.PreparedRequest...), IdempotencyKey: attempt.IdempotencyKey,
		InputAssets: cloneImageAssets(provider.inputs), MaskAsset: cloneImageAsset(provider.mask),
	})
	if err != nil {
		return aigateway.DispatchResult{}, err
	}
	if result == nil || len(result.Images) == 0 {
		return aigateway.DispatchResult{}, infraai.NewProviderError(infraai.ProviderOutcomeUnknown, "", errors.New("prepared image provider returned no result"))
	}
	candidate, err := provider.writer.WriteImageCandidate(ctx, provider.taskID, attempt.AttemptNo, result)
	if err != nil {
		return aigateway.DispatchResult{}, err
	}
	candidate = strings.TrimSpace(candidate)
	if candidate == "" || !json.Valid([]byte(candidate)) {
		return aigateway.DispatchResult{}, errors.New("prepared image result candidate is invalid")
	}
	dispatchState := strings.TrimSpace(result.DispatchState)
	if dispatchState == "" {
		dispatchState = infraai.DispatchStateDispatched
	}
	usage := result.Usage
	if usage.Status == "" {
		usage.Status = strings.TrimSpace(result.UsageStatus)
	}
	if usage.Status == "" {
		usage.Status = infraai.UsageStatusUnavailable
	}
	responseHash := result.ResponseSHA256
	if responseHash == ([sha256.Size]byte{}) && len(result.RawResponse) > 0 {
		responseHash = sha256.Sum256(result.RawResponse)
	}
	return aigateway.DispatchResult{
		ProviderRequestID: strings.TrimSpace(result.ProviderRequestID), ResponseSHA256: responseHash,
		DispatchState: dispatchState, TerminalState: "succeeded", Usage: usage, ResultCandidateJSON: &candidate,
	}, nil
}

func clonePreparedImageInput(input infraai.ImageInput) infraai.ImageInput {
	copy := input
	copy.InputAssets = cloneImageAssets(input.InputAssets)
	copy.MaskAsset = cloneImageAsset(input.MaskAsset)
	if input.OutputCompression != nil {
		value := *input.OutputCompression
		copy.OutputCompression = &value
	}
	return copy
}

func cloneImageAssets(assets []infraai.ImageAsset) []infraai.ImageAsset {
	cloned := make([]infraai.ImageAsset, len(assets))
	for index := range assets {
		cloned[index] = assets[index]
		cloned[index].Data = append([]byte(nil), assets[index].Data...)
	}
	return cloned
}

func cloneImageAsset(asset *infraai.ImageAsset) *infraai.ImageAsset {
	if asset == nil {
		return nil
	}
	copy := *asset
	copy.Data = append([]byte(nil), asset.Data...)
	return &copy
}

var _ aigateway.Provider = (*preparedImageProvider)(nil)

const imageResultCandidateVersion = "ai_image_result_v1"

type imageObjectDestination struct {
	SecretID     string
	SecretKey    string
	SessionToken string
	Bucket       string
	Region       string
	Endpoint     string
	BucketDomain string
}

type imageCandidateOutput struct {
	SortOrder       int     `json:"sort_order"`
	StorageProvider string  `json:"storage_provider"`
	StorageKey      string  `json:"storage_key"`
	StorageURL      string  `json:"storage_url"`
	SHA256          string  `json:"sha256"`
	MimeType        string  `json:"mime_type"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	SizeBytes       int64   `json:"size_bytes"`
	RevisedPrompt   *string `json:"revised_prompt,omitempty"`
}

type imageResultCandidate struct {
	Version      string                 `json:"version"`
	TaskID       uint64                 `json:"task_id"`
	AttemptNo    uint32                 `json:"attempt_no"`
	Outputs      []imageCandidateOutput `json:"outputs"`
	ActualParams map[string]any         `json:"actual_params,omitempty"`
	RawResponse  map[string]any         `json:"raw_response,omitempty"`
}

type cosImageCandidateWriter struct {
	objects     storagecos.ObjectWriter
	destination imageObjectDestination
}

func (writer *cosImageCandidateWriter) WriteImageCandidate(ctx context.Context, taskID uint64, attemptNo uint32, result *infraai.ImageResult) (string, error) {
	if writer == nil || writer.objects == nil || taskID == 0 || attemptNo == 0 || result == nil || len(result.Images) == 0 {
		return "", aigateway.ErrNotConfigured
	}
	destination := normalizeImageObjectDestination(writer.destination)
	if destination.SecretID == "" || destination.SecretKey == "" || destination.Bucket == "" || destination.Region == "" {
		return "", errors.New("AI image object destination is incomplete")
	}
	candidate := imageResultCandidate{Version: imageResultCandidateVersion, TaskID: taskID, AttemptNo: attemptNo, Outputs: make([]imageCandidateOutput, 0, len(result.Images))}
	for index, generated := range result.Images {
		encoded := strings.TrimSpace(generated.B64JSON)
		if encoded == "" {
			return "", errors.New("AI image result lacks immutable image bytes")
		}
		body, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(body) == 0 {
			return "", errors.New("AI image result base64 is invalid")
		}
		imageConfig, _, err := image.DecodeConfig(bytes.NewReader(body))
		if err != nil || imageConfig.Width <= 0 || imageConfig.Height <= 0 {
			return "", errors.New("AI image result dimensions are invalid")
		}
		mimeType, extension, err := normalizedCandidateImageMime(generated.MimeType, body)
		if err != nil {
			return "", err
		}
		digest := sha256.Sum256(body)
		digestText := hex.EncodeToString(digest[:])
		key := fmt.Sprintf("ai-images/task-%d/attempt-%d/%02d-%s%s", taskID, attemptNo, index+1, digestText, extension)
		if err := writer.objects.Put(ctx, storagecos.PutInput{
			SecretID: destination.SecretID, SecretKey: destination.SecretKey, SessionToken: destination.SessionToken,
			Bucket: destination.Bucket, Region: destination.Region, Endpoint: destination.Endpoint,
			Key: key, Body: body, ContentType: mimeType,
		}); err != nil {
			return "", fmt.Errorf("store AI image result candidate: %w", err)
		}
		candidate.Outputs = append(candidate.Outputs, imageCandidateOutput{
			SortOrder: index + 1, StorageProvider: aiimage.StorageProviderCOS, StorageKey: key,
			StorageURL: imageObjectPublicURL(destination, key), SHA256: digestText, MimeType: mimeType,
			Width: imageConfig.Width, Height: imageConfig.Height, SizeBytes: int64(len(body)),
			RevisedPrompt: optionalImageCandidateString(generated.RevisedPrompt),
		})
	}
	if len(result.ActualParams) > 0 {
		encoded, err := json.Marshal(result.ActualParams)
		if err != nil || json.Unmarshal(encoded, &candidate.ActualParams) != nil {
			return "", errors.New("AI image actual parameters are invalid")
		}
		sanitizeImageCandidateJSON(candidate.ActualParams)
	}
	if len(result.RawResponse) > 0 {
		var raw map[string]any
		if json.Unmarshal(result.RawResponse, &raw) == nil {
			sanitizeImageCandidateJSON(raw)
			candidate.RawResponse = raw
		}
	}
	encoded, err := json.Marshal(candidate)
	if err != nil {
		return "", err
	}
	if _, err := decodeImageResultCandidate(string(encoded)); err != nil {
		return "", err
	}
	return string(encoded), nil
}

func decodeImageResultCandidate(raw string) (imageResultCandidate, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	var candidate imageResultCandidate
	if err := decoder.Decode(&candidate); err != nil {
		return imageResultCandidate{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return imageResultCandidate{}, errors.New("AI image result candidate has trailing JSON")
	}
	if candidate.Version != imageResultCandidateVersion || candidate.TaskID == 0 || candidate.AttemptNo == 0 || len(candidate.Outputs) == 0 {
		return imageResultCandidate{}, errors.New("AI image result candidate is incomplete")
	}
	seen := make(map[int]struct{}, len(candidate.Outputs))
	for _, output := range candidate.Outputs {
		if output.SortOrder <= 0 || strings.TrimSpace(output.StorageProvider) != aiimage.StorageProviderCOS || strings.TrimSpace(output.StorageKey) == "" ||
			!validImageCandidateURL(output.StorageURL) || len(strings.TrimSpace(output.SHA256)) != sha256.Size*2 || output.Width <= 0 || output.Height <= 0 || output.SizeBytes <= 0 {
			return imageResultCandidate{}, errors.New("AI image result candidate output is invalid")
		}
		if digest, err := hex.DecodeString(strings.TrimSpace(output.SHA256)); err != nil || len(digest) != sha256.Size {
			return imageResultCandidate{}, errors.New("AI image result candidate digest is invalid")
		}
		if _, duplicate := seen[output.SortOrder]; duplicate {
			return imageResultCandidate{}, errors.New("AI image result candidate output order is duplicated")
		}
		seen[output.SortOrder] = struct{}{}
	}
	return candidate, nil
}

func normalizeImageObjectDestination(destination imageObjectDestination) imageObjectDestination {
	destination.SecretID = strings.TrimSpace(destination.SecretID)
	destination.SecretKey = strings.TrimSpace(destination.SecretKey)
	destination.SessionToken = strings.TrimSpace(destination.SessionToken)
	destination.Bucket = strings.TrimSpace(destination.Bucket)
	destination.Region = strings.TrimSpace(destination.Region)
	destination.Endpoint = strings.TrimRight(strings.TrimSpace(destination.Endpoint), "/")
	destination.BucketDomain = strings.TrimRight(strings.TrimSpace(destination.BucketDomain), "/")
	return destination
}

func normalizedCandidateImageMime(raw string, body []byte) (string, string, error) {
	mimeType := strings.ToLower(strings.TrimSpace(raw))
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = strings.ToLower(strings.TrimSpace(http.DetectContentType(body)))
	}
	switch mimeType {
	case "image/png":
		return mimeType, ".png", nil
	case "image/jpeg":
		return mimeType, ".jpg", nil
	case "image/webp":
		return mimeType, ".webp", nil
	default:
		return "", "", fmt.Errorf("unsupported AI image result MIME type %q", mimeType)
	}
}

func imageObjectPublicURL(destination imageObjectDestination, key string) string {
	base := destination.BucketDomain
	if base == "" {
		base = destination.Endpoint
	}
	if base == "" {
		base = fmt.Sprintf("https://%s.cos.%s.myqcloud.com", destination.Bucket, destination.Region)
	} else if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "https://" + strings.TrimLeft(base, "/")
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(key, "/")
}

func validImageCandidateURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func optionalImageCandidateString(raw string) *string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	return &value
}

func sanitizeImageCandidateJSON(value map[string]any) {
	for key, item := range value {
		if strings.EqualFold(key, "b64_json") {
			value[key] = "[omitted]"
			continue
		}
		switch typed := item.(type) {
		case map[string]any:
			sanitizeImageCandidateJSON(typed)
		case []any:
			for _, child := range typed {
				if object, ok := child.(map[string]any); ok {
					sanitizeImageCandidateJSON(object)
				}
			}
		}
	}
}

func imageFilesFromCandidate(raw string, taskID uint64, attemptNo uint32, now time.Time) ([]aiimage.ImageFile, *string, *string, error) {
	candidate, err := decodeImageResultCandidate(raw)
	if err != nil {
		return nil, nil, nil, err
	}
	if taskID == 0 || attemptNo == 0 || candidate.TaskID != taskID || candidate.AttemptNo != attemptNo {
		return nil, nil, nil, errors.New("AI image result candidate belongs to another task or attempt")
	}
	files := make([]aiimage.ImageFile, 0, len(candidate.Outputs))
	for _, output := range candidate.Outputs {
		files = append(files, aiimage.ImageFile{
			TaskID: taskID, Role: aiimage.FileRoleOutput, SortOrder: output.SortOrder,
			StorageProvider: output.StorageProvider, StorageKey: output.StorageKey, StorageURL: output.StorageURL,
			MimeType: output.MimeType, Width: output.Width, Height: output.Height, SizeBytes: output.SizeBytes,
			RevisedPrompt: output.RevisedPrompt, IsDel: enum.CommonNo, CreatedAt: now,
		})
	}
	actual, err := imageCandidateMapJSON(candidate.ActualParams)
	if err != nil {
		return nil, nil, nil, err
	}
	rawResponse, err := imageCandidateMapJSON(candidate.RawResponse)
	if err != nil {
		return nil, nil, nil, err
	}
	return files, actual, rawResponse, nil
}

func imageCandidateMapJSON(value map[string]any) (*string, error) {
	if len(value) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	encoded := string(raw)
	return &encoded, nil
}

func loadPreparedImageAssets(ctx context.Context, reader storagecos.ObjectReader, destination imageObjectDestination, attachments []aiimage.AttachmentSnapshot) ([]infraai.ImageAsset, *infraai.ImageAsset, error) {
	if len(attachments) == 0 {
		return []infraai.ImageAsset{}, nil, nil
	}
	if reader == nil {
		return nil, nil, aigateway.ErrNotConfigured
	}
	destination = normalizeImageObjectDestination(destination)
	if destination.SecretID == "" || destination.SecretKey == "" || destination.Bucket == "" || destination.Region == "" {
		return nil, nil, errors.New("AI image object destination is incomplete")
	}
	inputs := make([]infraai.ImageAsset, 0, len(attachments))
	inputSortOrders := make(map[int]struct{}, len(attachments))
	var mask *infraai.ImageAsset
	var maskRelatedSort int
	for _, attachment := range attachments {
		if attachment.StorageProvider != aiimage.StorageProviderCOS || strings.TrimSpace(attachment.StorageKey) == "" || attachment.SizeBytes <= 0 {
			return nil, nil, errors.New("AI image attachment snapshot is incomplete")
		}
		result, err := reader.Get(ctx, storagecos.GetInput{
			SecretID: destination.SecretID, SecretKey: destination.SecretKey, SessionToken: destination.SessionToken,
			Bucket: destination.Bucket, Region: destination.Region, Endpoint: destination.Endpoint, Key: attachment.StorageKey,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("load AI image attachment: %w", err)
		}
		if result == nil || int64(len(result.Body)) != attachment.SizeBytes {
			return nil, nil, errors.New("AI image attachment size changed after acceptance")
		}
		digest := sha256.Sum256(result.Body)
		if !strings.EqualFold(hex.EncodeToString(digest[:]), strings.TrimSpace(attachment.SHA256)) {
			return nil, nil, errors.New("AI image attachment digest changed after acceptance")
		}
		asset := infraai.ImageAsset{
			Name: path.Base(strings.TrimSpace(attachment.StorageKey)), MimeType: strings.TrimSpace(attachment.MimeType),
			StorageProvider: attachment.StorageProvider, StorageKey: strings.TrimSpace(attachment.StorageKey),
			SHA256: strings.ToLower(strings.TrimSpace(attachment.SHA256)), SizeBytes: attachment.SizeBytes,
			Data: append([]byte(nil), result.Body...),
		}
		switch attachment.Role {
		case aiimage.FileRoleInput:
			if attachment.SortOrder <= 0 {
				return nil, nil, errors.New("AI image input attachment order is invalid")
			}
			if _, duplicate := inputSortOrders[attachment.SortOrder]; duplicate {
				return nil, nil, errors.New("AI image input attachment order is duplicated")
			}
			inputSortOrders[attachment.SortOrder] = struct{}{}
			inputs = append(inputs, asset)
		case aiimage.FileRoleMask:
			if mask != nil || attachment.RelatedSortOrder <= 0 {
				return nil, nil, errors.New("AI image mask attachment is invalid")
			}
			mask = &asset
			maskRelatedSort = attachment.RelatedSortOrder
		default:
			return nil, nil, errors.New("AI image attachment role is invalid")
		}
	}
	if mask != nil {
		if _, exists := inputSortOrders[maskRelatedSort]; !exists {
			return nil, nil, errors.New("AI image mask target is missing")
		}
	}
	return inputs, mask, nil
}
