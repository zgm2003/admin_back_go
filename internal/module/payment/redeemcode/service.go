package redeemcode

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	exporttask "admin_back_go/internal/module/export"
	"admin_back_go/internal/module/payment/serialno"
	"admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/clock"
	"admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/telemetry"
)

const (
	ErrorRequestInvalid              = "payment.redeem_code.request_invalid"
	ErrorRequestConflict             = "payment.redeem_code.request_conflict"
	ErrorVoidConflict                = "payment.redeem_code.void_conflict"
	ErrorExportTooLarge              = "payment.redeem_code.export_too_large"
	ErrorDependencyUnavailable       = "payment.redeem_code.dependency_unavailable"
	ErrorIntegrityViolation          = "payment.redeem_code.integrity_violation"
	ErrorServiceMissing              = "payment.redeem_code.service_missing"
	ErrorWalletCodeRequired          = "wallet.redeem.code_required"
	ErrorWalletUnavailable           = "wallet.redeem.unavailable"
	ErrorWalletDependencyUnavailable = "wallet.redeem.dependency_unavailable"
	ErrorWalletIntegrityViolation    = "wallet.redeem.integrity_violation"
	ErrorWalletRateLimitUnavailable  = "wallet.redeem.rate_limit_unavailable"

	defaultPageSize        = 20
	maxPageSize            = 100
	maxCreateBatchAttempts = 3
)

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

type Repository interface {
	FindBatchByRequest(context.Context, int64, string) (*BatchWithCodes, error)
	CreateOrReplayBatch(context.Context, CreateBatchRecord) (*BatchWithCodes, bool, error)
	ListCodes(context.Context, ListQuery, time.Time) ([]CodeView, int64, error)
	LookupCode(context.Context, string, time.Time) (*CodeView, error)
	ExportCodes(context.Context, ListQuery, time.Time, int) ([]CodeView, error)
	VoidCodes(context.Context, []int64, time.Time) (int, error)
	Redeem(context.Context, int64, string) (*RedemptionFact, error)
}

type Service struct {
	repository Repository
	limiter    AttemptLimiter
	clock      clock.Clock
	recorder   telemetry.Recorder
	random     io.Reader
	logger     *slog.Logger
	metrics    metrics
}

type Option func(*Service)

func WithClock(value clock.Clock) Option {
	return func(service *Service) { service.clock = value }
}

func WithTelemetry(value telemetry.Recorder) Option {
	return func(service *Service) { service.recorder = value }
}

func WithRandom(value io.Reader) Option {
	return func(service *Service) { service.random = value }
}

func WithAttemptLimiter(value AttemptLimiter) Option {
	return func(service *Service) { service.limiter = value }
}

func WithLogger(value *slog.Logger) Option {
	return func(service *Service) { service.logger = value }
}

func NewService(repository Repository, options ...Option) *Service {
	service := &Service{repository: repository}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	if service.clock == nil {
		service.clock = clock.SystemClock{}
	}
	if service.recorder == nil {
		service.recorder = telemetry.Noop()
	}
	if service.random == nil {
		service.random = rand.Reader
	}
	if service.logger == nil {
		service.logger = slog.Default()
	}
	if service.limiter == nil {
		service.limiter = unavailableAttemptLimiter{}
	}
	service.metrics = newMetrics(service.recorder)
	return service
}

func (service *Service) PageInit(context.Context) (*PageInitResponse, *apperror.Error) {
	return &PageInitResponse{States: []dict.Option[string]{
		{Label: "未使用", Value: StateUnused}, {Label: "已使用", Value: StateUsed},
		{Label: "已作废", Value: StateVoided}, {Label: "已过期", Value: StateExpired},
	}}, nil
}

func (service *Service) GenerateBatch(ctx context.Context, createdBy int64, input GenerateBatchInput) (*GenerateBatchResponse, *apperror.Error) {
	repository, appErr := service.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	normalized, appErr := normalizeGenerateInput(createdBy, input)
	if appErr != nil {
		service.metrics.batch("rejected", "invalid")
		return nil, appErr
	}
	now := operationNow(service.clock)
	fingerprint, err := batchRequestFingerprint(normalized.amountCents, normalized.quantity, normalized.expiresAt, normalized.note)
	if err != nil {
		return nil, managementIntegrity(nil)
	}

	existing, err := repository.FindBatchByRequest(ctx, createdBy, normalized.requestID)
	if err != nil {
		if errors.Is(err, ErrIntegrityViolation) {
			service.metrics.batch("error", "integrity")
		} else {
			service.metrics.batch("error", "dependency")
		}
		return nil, managementRepositoryError(err)
	}
	if existing != nil {
		response, replayErr := service.replayResponse(existing, fingerprint)
		if replayErr != nil {
			return nil, replayErr
		}
		service.metrics.batch("replayed", "replayed")
		return response, nil
	}

	for attempt := 0; attempt < maxCreateBatchAttempts; attempt++ {
		codes, generateErr := generateUniqueCodes(service.random, normalized.quantity)
		if generateErr != nil {
			service.metrics.batch("error", "code_collision")
			service.metrics.conflict("generate", "code_collision")
			return nil, managementIntegrity(nil)
		}
		record := CreateBatchRecord{Batch: Batch{
			BatchNo: serialno.New("RCB", now), RequestID: normalized.requestID,
			RequestFingerprintVersion: RequestFingerprintVersion, RequestFingerprint: fingerprint,
			AmountCents: normalized.amountCents, Quantity: normalized.quantity, ExpiresAt: normalized.expiresAt,
			Note: normalized.note, CreatedBy: createdBy,
		}, Codes: make([]Code, normalized.quantity)}
		for index, code := range codes {
			record.Codes[index] = Code{Code: code, State: StateUnused}
		}

		created, replayed, createErr := repository.CreateOrReplayBatch(ctx, record)
		if createErr != nil {
			switch {
			case errors.Is(createErr, ErrBatchNumberCollision):
				service.metrics.conflict("generate", "batch_collision")
				continue
			case errors.Is(createErr, ErrCodeCollision):
				service.metrics.conflict("generate", "code_collision")
				continue
			case errors.Is(createErr, ErrRequestConflict):
				service.metrics.conflict("generate", "request")
				return nil, managementConflict(ErrorRequestConflict, "生成请求ID已用于其他参数")
			case errors.Is(createErr, ErrExpiryNotFuture):
				return nil, managementInvalid("过期时间必须晚于创建时间")
			case errors.Is(createErr, ErrIntegrityViolation):
				return nil, managementIntegrity(nil)
			default:
				return nil, managementDependency(createErr)
			}
		}
		response, replayErr := service.replayResponse(created, fingerprint)
		if replayErr != nil {
			return nil, replayErr
		}
		response.Batch.Replayed = replayed
		if replayed {
			service.metrics.batch("replayed", "replayed")
		} else {
			service.metrics.batch("ok", "created")
			service.metrics.codes(normalized.quantity, StateUnused)
		}
		return response, nil
	}
	service.metrics.batch("error", "code_collision")
	return nil, managementIntegrity(nil)
}

func (service *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repository, appErr := service.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query, appErr = normalizeListQuery(query, true)
	if appErr != nil {
		return nil, appErr
	}
	now := operationNow(service.clock)
	rows, total, err := repository.ListCodes(ctx, query, now)
	if err != nil {
		return nil, managementRepositoryError(err)
	}
	items := codeItems(rows, now)
	return &ListResponse{List: items, Page: Page{
		CurrentPage: query.CurrentPage, PageSize: query.PageSize, TotalPage: totalPages(total, query.PageSize), Total: total,
	}}, nil
}

func (service *Service) Lookup(ctx context.Context, input LookupInput) (*LookupResponse, *apperror.Error) {
	repository, appErr := service.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	code, err := NormalizeCode(input.Code)
	if err != nil {
		return nil, managementInvalid("兑换码格式无效")
	}
	now := operationNow(service.clock)
	row, err := repository.LookupCode(ctx, code, now)
	if err != nil {
		return nil, managementRepositoryError(err)
	}
	if row == nil {
		return &LookupResponse{}, nil
	}
	item := codeItem(*row, now)
	return &LookupResponse{Item: &item}, nil
}

func (service *Service) Void(ctx context.Context, input VoidInput) (*VoidResponse, *apperror.Error) {
	repository, appErr := service.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	if len(input.IDs) == 0 || len(input.IDs) > MaxVoidCodes {
		return nil, managementInvalid("作废兑换码数量无效")
	}
	unique := make(map[int64]struct{}, len(input.IDs))
	for _, id := range input.IDs {
		if id <= 0 {
			return nil, managementInvalid("兑换码ID无效")
		}
		unique[id] = struct{}{}
	}
	ids := make([]int64, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	now := operationNow(service.clock)
	count, err := repository.VoidCodes(ctx, ids, now)
	if err != nil {
		if errors.Is(err, ErrVoidConflict) {
			service.metrics.conflict("void", "request")
			return nil, managementConflict(ErrorVoidConflict, "兑换码集合无法作废")
		}
		if errors.Is(err, ErrIntegrityViolation) {
			return nil, managementIntegrity(nil)
		}
		return nil, managementDependency(err)
	}
	service.metrics.transition(count, StateVoided, "admin_void")
	return &VoidResponse{Voided: count}, nil
}

func (service *Service) Export(ctx context.Context, input ExportInput) (*ExportResponse, *apperror.Error) {
	repository, appErr := service.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query, appErr := normalizeListQuery(input.ListQuery(), false)
	if appErr != nil {
		return nil, appErr
	}
	now := operationNow(service.clock)
	rows, err := repository.ExportCodes(ctx, query, now, MaxExportRows+1)
	if err != nil {
		return nil, managementRepositoryError(err)
	}
	if len(rows) > MaxExportRows {
		return nil, newAppError(ErrorExportTooLarge, apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent, "导出结果超过上限", nil)
	}
	items := codeItems(rows, now)
	data := exporttask.FileData{
		Headers: []exporttask.Column{
			{Key: "code", Title: "兑换码"}, {Key: "batch_no", Title: "批次号"}, {Key: "amount", Title: "面额"},
			{Key: "state", Title: "状态"}, {Key: "expires_at", Title: "过期时间"}, {Key: "note", Title: "备注"},
			{Key: "used_account", Title: "兑换用户"}, {Key: "used_at", Title: "兑换时间"},
			{Key: "creator", Title: "创建人"}, {Key: "created_at", Title: "创建时间"},
			{Key: "wallet_transaction_no", Title: "钱包流水号"},
		},
		Rows: make([]map[string]string, len(items)),
	}
	for index, item := range items {
		data.Rows[index] = map[string]string{
			"code": item.Code, "batch_no": item.BatchNo, "amount": item.Amount, "state": item.State,
			"expires_at": item.ExpiresAt, "note": item.Note, "used_account": item.UsedAccount,
			"used_at": item.UsedAt, "creator": item.CreatorUsername, "created_at": item.CreatedAt,
			"wallet_transaction_no": item.WalletTransactionNo,
		}
	}
	body, err := (exporttask.CSVWriter{}).Write(data)
	if err != nil {
		return nil, managementIntegrity(err)
	}
	return &ExportResponse{Filename: "redeem-codes-" + now.Format("20060102") + ".csv", Content: string(body), RowCount: len(items)}, nil
}

func (service *Service) Redeem(ctx context.Context, userID int64, rawCode string) (*RedemptionResponse, *apperror.Error) {
	return service.redeemLimited(ctx, userID, rawCode)
}

func (service *Service) redeemLimited(requestCtx context.Context, userID int64, rawCode string) (*RedemptionResponse, *apperror.Error) {
	const platform = "admin"
	lease, err := service.limiter.Acquire(requestCtx, platform, userID)
	if err != nil {
		if locked, ok := err.(*AttemptLockedError); ok {
			return nil, walletRateLimited(locked.RetryAfter)
		}
		if errors.Is(err, ErrAttemptLocked) {
			return nil, walletRateLimited(1)
		}
		if errors.Is(err, errLimiterUnavailable) {
			return nil, walletRateDependency(err)
		}
		return nil, walletDependency(err)
	}

	attemptCtx, cancel := context.WithTimeout(requestCtx, attemptTimeout)
	defer cancel()
	release := func() error {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(requestCtx), cleanupTimeout)
		defer cleanupCancel()
		return service.limiter.Release(cleanupCtx, lease)
	}
	state, err := service.limiter.FailureState(attemptCtx, platform, userID)
	if err != nil {
		_ = release()
		return nil, walletDependency(err)
	}
	if state.Count >= failureLimit {
		if releaseErr := release(); releaseErr != nil {
			return nil, walletDependency(releaseErr)
		}
		return nil, walletRateLimited(failureRetryAfter(state))
	}

	markFailure := func() *apperror.Error {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(requestCtx), cleanupTimeout)
		defer cleanupCancel()
		if _, recordErr := service.limiter.RecordFailure(cleanupCtx, platform, userID); recordErr != nil {
			_ = release()
			return walletDependency(recordErr)
		}
		return nil
	}

	repository, appErr := service.requireRepository()
	if appErr != nil {
		if releaseErr := release(); releaseErr != nil {
			return nil, walletDependency(releaseErr)
		}
		return nil, walletDependency(nil)
	}
	if strings.TrimSpace(rawCode) == "" {
		if recordErr := markFailure(); recordErr != nil {
			return nil, recordErr
		}
		if releaseErr := release(); releaseErr != nil {
			return nil, walletDependency(releaseErr)
		}
		return nil, newAppError(ErrorWalletCodeRequired, apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent, "请输入兑换码", nil)
	}
	code, normalizeErr := NormalizeCode(rawCode)
	if normalizeErr != nil {
		service.metrics.redemption("unavailable", "invalid", 0)
		if recordErr := markFailure(); recordErr != nil {
			return nil, recordErr
		}
		if releaseErr := release(); releaseErr != nil {
			return nil, walletDependency(releaseErr)
		}
		return nil, walletUnavailable()
	}

	started := time.Now()
	fact, redeemErr := repository.Redeem(attemptCtx, userID, code)
	elapsed := time.Since(started)
	if redeemErr != nil {
		var responseErr *apperror.Error
		switch {
		case errors.Is(redeemErr, ErrExpired):
			service.metrics.codes(1, StateExpired)
			service.metrics.redemption("unavailable", "expired", elapsed)
			responseErr = walletUnavailable()
		case errors.Is(redeemErr, ErrUnavailable):
			service.metrics.redemption("unavailable", "unavailable", elapsed)
			responseErr = walletUnavailable()
		case errors.Is(redeemErr, ErrSourceConflict):
			service.metrics.conflict("redeem", "source_unique")
			service.metrics.redemption("error", "source_unique", elapsed)
			responseErr = walletIntegrity(nil)
		case errors.Is(redeemErr, ErrOverflow):
			service.metrics.redemption("rejected", "wallet_overflow", elapsed)
			responseErr = walletIntegrity(nil)
		case errors.Is(redeemErr, ErrIntegrityViolation):
			service.metrics.redemption("error", "integrity", elapsed)
			responseErr = walletIntegrity(nil)
		default:
			service.metrics.redemption("error", "dependency", elapsed)
			responseErr = walletDependency(redeemErr)
		}
		if responseErr.Code == ErrorWalletUnavailable {
			if recordErr := markFailure(); recordErr != nil {
				return nil, recordErr
			}
		}
		if releaseErr := release(); releaseErr != nil {
			return nil, walletDependency(releaseErr)
		}
		return nil, responseErr
	}
	if !validRedemptionFact(fact, userID) {
		service.metrics.redemption("error", "integrity", elapsed)
		if releaseErr := release(); releaseErr != nil {
			return nil, walletDependency(releaseErr)
		}
		return nil, walletIntegrity(nil)
	}
	outcome, reason := "ok", "created"
	if fact.Replayed {
		outcome, reason = "replayed", "replayed"
	} else {
		service.metrics.codes(1, StateUsed)
		service.metrics.transition(1, StateUsed, "created")
	}
	service.metrics.redemption(outcome, reason, elapsed)
	response := redemptionResponse(fact)
	// Once the repository has established a successful fact, keep that fact even if cleanup fails.
	if releaseErr := release(); releaseErr != nil {
		service.logger.Warn("redeem limiter cleanup failed", "operation", "redeem", "reason", "release")
	}
	return response, nil
}

var errLimiterUnavailable = errors.New("redeem limiter unavailable")

type unavailableAttemptLimiter struct{}

func (unavailableAttemptLimiter) Acquire(context.Context, string, int64) (AttemptLease, error) {
	return AttemptLease{}, errLimiterUnavailable
}
func (unavailableAttemptLimiter) FailureState(context.Context, string, int64) (FailureState, error) {
	return FailureState{}, errLimiterUnavailable
}
func (unavailableAttemptLimiter) RecordFailure(context.Context, string, int64) (FailureState, error) {
	return FailureState{}, errLimiterUnavailable
}
func (unavailableAttemptLimiter) Release(context.Context, AttemptLease) error {
	return errLimiterUnavailable
}

func failureRetryAfter(state FailureState) int {
	if state.RetryAfter > 0 {
		return state.RetryAfter
	}
	return retryAfter(state.TTL)
}

func (service *Service) redeemUnlocked(ctx context.Context, userID int64, rawCode string) (*RedemptionResponse, *apperror.Error) {
	repository, appErr := service.requireRepository()
	if appErr != nil {
		return nil, walletDependency(nil)
	}
	if strings.TrimSpace(rawCode) == "" {
		return nil, newAppError(ErrorWalletCodeRequired, apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent, "请输入兑换码", nil)
	}
	code, err := NormalizeCode(rawCode)
	if err != nil {
		service.metrics.redemption("unavailable", "invalid", 0)
		return nil, walletUnavailable()
	}
	started := time.Now()
	fact, err := repository.Redeem(ctx, userID, code)
	elapsed := time.Since(started)
	if err != nil {
		switch {
		case errors.Is(err, ErrExpired):
			service.metrics.codes(1, StateExpired)
			service.metrics.redemption("unavailable", "expired", elapsed)
			return nil, walletUnavailable()
		case errors.Is(err, ErrUnavailable):
			service.metrics.redemption("unavailable", "unavailable", elapsed)
			return nil, walletUnavailable()
		case errors.Is(err, ErrSourceConflict):
			service.metrics.conflict("redeem", "source_unique")
			service.metrics.redemption("error", "source_unique", elapsed)
			return nil, walletIntegrity(nil)
		case errors.Is(err, ErrOverflow):
			service.metrics.redemption("rejected", "wallet_overflow", elapsed)
			return nil, walletIntegrity(nil)
		case errors.Is(err, ErrIntegrityViolation):
			service.metrics.redemption("error", "integrity", elapsed)
			return nil, walletIntegrity(nil)
		default:
			service.metrics.redemption("error", "dependency", elapsed)
			return nil, walletDependency(err)
		}
	}
	if !validRedemptionFact(fact, userID) {
		service.metrics.redemption("error", "integrity", elapsed)
		return nil, walletIntegrity(nil)
	}
	outcome := "ok"
	reason := "created"
	if fact.Replayed {
		outcome, reason = "replayed", "replayed"
	} else {
		service.metrics.codes(1, StateUsed)
		service.metrics.transition(1, StateUsed, "created")
	}
	service.metrics.redemption(outcome, reason, elapsed)
	return redemptionResponse(fact), nil
}

type normalizedGenerate struct {
	requestID   string
	amountCents int64
	quantity    int
	expiresAt   *time.Time
	note        string
}

func normalizeGenerateInput(createdBy int64, input GenerateBatchInput) (normalizedGenerate, *apperror.Error) {
	if createdBy <= 0 || !requestIDPattern.MatchString(input.RequestID) {
		return normalizedGenerate{}, managementInvalid("生成请求ID无效")
	}
	amountCents, _, err := ParseAmountCents(input.Amount)
	if err != nil || amountCents <= 0 || amountCents > MaxAmountCents {
		return normalizedGenerate{}, managementInvalid("兑换码面额无效")
	}
	if input.Quantity <= 0 || input.Quantity > MaxBatchQuantity {
		return normalizedGenerate{}, managementInvalid("兑换码数量无效")
	}
	note := strings.TrimSpace(input.Note)
	if !utf8.ValidString(note) || utf8.RuneCountInString(note) > 255 {
		return normalizedGenerate{}, managementInvalid("兑换码备注无效")
	}
	for _, character := range note {
		if unicode.IsControl(character) {
			return normalizedGenerate{}, managementInvalid("兑换码备注无效")
		}
	}
	var expiresAt *time.Time
	if input.ExpiresAt != nil {
		value := input.ExpiresAt.UTC().Truncate(time.Microsecond)
		expiresAt = &value
	}
	return normalizedGenerate{requestID: input.RequestID, amountCents: amountCents, quantity: input.Quantity, expiresAt: expiresAt, note: note}, nil
}

func batchRequestFingerprint(amountCents int64, quantity int, expiresAt *time.Time, note string) (string, error) {
	type canonicalRequest struct {
		AmountCents int64   `json:"amount_cents"`
		Quantity    int     `json:"quantity"`
		ExpiresAt   *string `json:"expires_at"`
		Note        string  `json:"note"`
	}
	var expiry *string
	if expiresAt != nil {
		value := expiresAt.UTC().Truncate(time.Microsecond).Format("2006-01-02T15:04:05.000000Z07:00")
		expiry = &value
	}
	canonical, err := json.Marshal(canonicalRequest{AmountCents: amountCents, Quantity: quantity, ExpiresAt: expiry, Note: note})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func (service *Service) replayResponse(batch *BatchWithCodes, fingerprint string) (*GenerateBatchResponse, *apperror.Error) {
	if batch == nil {
		return nil, managementIntegrity(nil)
	}
	if batch.Batch.RequestFingerprintVersion != RequestFingerprintVersion || batch.Batch.RequestFingerprint != fingerprint {
		service.metrics.conflict("generate", "request")
		return nil, managementConflict(ErrorRequestConflict, "生成请求ID已用于其他参数")
	}
	if batch.Batch.Quantity <= 0 || len(batch.Codes) != batch.Batch.Quantity {
		return nil, managementIntegrity(nil)
	}
	codes := append([]Code(nil), batch.Codes...)
	sort.Slice(codes, func(left, right int) bool { return codes[left].ID < codes[right].ID })
	response := &GenerateBatchResponse{Batch: GeneratedBatchItem{
		ID: batch.Batch.ID, BatchNo: batch.Batch.BatchNo, RequestID: batch.Batch.RequestID,
		Amount: amountText(batch.Batch.AmountCents), Quantity: batch.Batch.Quantity,
		ExpiresAt: formatOptionalTime(batch.Batch.ExpiresAt), Note: batch.Batch.Note,
		CreatedBy: batch.Batch.CreatedBy, CreatedAt: formatTime(batch.Batch.CreatedAt), Replayed: true,
	}, Codes: make([]GeneratedCodeItem, len(codes))}
	for index, code := range codes {
		response.Codes[index] = GeneratedCodeItem{ID: code.ID, Code: code.Code}
	}
	return response, nil
}

func normalizeListQuery(query ListQuery, paginated bool) (ListQuery, *apperror.Error) {
	query.BatchNo = strings.TrimSpace(query.BatchNo)
	query.State = strings.TrimSpace(query.State)
	query.UsedUser = strings.TrimSpace(query.UsedUser)
	query.Note = strings.TrimSpace(query.Note)
	if query.State != "" && query.State != StateUnused && query.State != StateUsed && query.State != StateVoided && query.State != StateExpired {
		return ListQuery{}, managementInvalid("兑换码状态无效")
	}
	if query.UsedBy < 0 || query.CreatedBy < 0 {
		return ListQuery{}, managementInvalid("用户筛选无效")
	}
	query.CreatedFrom = normalizeOptionalTime(query.CreatedFrom)
	query.CreatedTo = normalizeOptionalTime(query.CreatedTo)
	query.ExpiresFrom = normalizeOptionalTime(query.ExpiresFrom)
	query.ExpiresTo = normalizeOptionalTime(query.ExpiresTo)
	if paginated {
		if query.CurrentPage <= 0 {
			query.CurrentPage = 1
		}
		if query.PageSize <= 0 {
			query.PageSize = defaultPageSize
		}
		if query.PageSize > maxPageSize {
			query.PageSize = maxPageSize
		}
	} else {
		query.CurrentPage, query.PageSize = 0, 0
	}
	return query, nil
}

func normalizeOptionalTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := value.UTC().Truncate(time.Microsecond)
	return &normalized
}

func codeItems(rows []CodeView, now time.Time) []CodeItem {
	items := make([]CodeItem, len(rows))
	for index, row := range rows {
		items[index] = codeItem(row, now)
	}
	return items
}

func codeItem(row CodeView, now time.Time) CodeItem {
	state := row.State
	if state == StateUnused && row.ExpiresAt != nil && !row.ExpiresAt.After(now) {
		state = StateExpired
	}
	usedBy := int64(0)
	if row.UsedBy != nil {
		usedBy = *row.UsedBy
	}
	return CodeItem{
		ID: row.ID, Code: row.Code, BatchID: row.BatchID, BatchNo: row.BatchNo,
		AmountCents: row.AmountCents, Amount: amountText(row.AmountCents), State: state,
		ExpiresAt: formatOptionalTime(row.ExpiresAt), Note: row.Note, UsedBy: usedBy,
		UsedUsername: row.UsedUsername, UsedAccount: row.UsedAccount, UsedAt: formatOptionalTime(row.UsedAt),
		CreatedBy: row.CreatedBy, CreatorUsername: row.CreatorUsername, CreatedAt: formatTime(row.CreatedAt),
		WalletTransactionNo: row.WalletTransactionNo,
	}
}

func validRedemptionFact(fact *RedemptionFact, userID int64) bool {
	if fact == nil || fact.AmountCents <= 0 || fact.Transaction == nil || fact.Wallet == nil {
		return false
	}
	transaction := fact.Transaction
	currentWallet := fact.Wallet
	if transaction.ID <= 0 || transaction.WalletID <= 0 || transaction.UserID != userID || transaction.Direction != wallet.DirectionIn ||
		transaction.SourceType != wallet.SourceRedeemCode || transaction.SourceID <= 0 || transaction.AmountCents != fact.AmountCents ||
		transaction.IsDel != enum.CommonNo || currentWallet.ID != transaction.WalletID || currentWallet.UserID != userID || currentWallet.IsDel != enum.CommonNo {
		return false
	}
	return transaction.BalanceBeforeCents <= math.MaxInt64-transaction.AmountCents &&
		transaction.BalanceBeforeCents+transaction.AmountCents == transaction.BalanceAfterCents
}

func redemptionResponse(fact *RedemptionFact) *RedemptionResponse {
	transaction := fact.Transaction
	currentWallet := fact.Wallet
	return &RedemptionResponse{
		Amount: amountText(fact.AmountCents), Replayed: fact.Replayed,
		Transaction: wallet.TransactionItem{
			ID: transaction.ID, TransactionNo: transaction.TransactionNo, UserID: transaction.UserID,
			Direction: transaction.Direction, DirectionText: "收入", AmountCents: transaction.AmountCents,
			AmountText: amountText(transaction.AmountCents), BalanceBeforeCents: transaction.BalanceBeforeCents,
			BalanceBeforeText: amountText(transaction.BalanceBeforeCents), BalanceAfterCents: transaction.BalanceAfterCents,
			BalanceAfterText: amountText(transaction.BalanceAfterCents), SourceType: transaction.SourceType,
			SourceTypeText: "兑换码充值", SourceID: transaction.SourceID, Remark: transaction.Remark,
			CreatedAt: formatTime(transaction.CreatedAt),
		},
		Wallet: wallet.SummaryResponse{
			BalanceCents: currentWallet.BalanceCents, BalanceText: amountText(currentWallet.BalanceCents),
			TotalRechargeCents: currentWallet.TotalRechargeCents, TotalRechargeText: amountText(currentWallet.TotalRechargeCents),
			TotalConsumeCents: currentWallet.TotalConsumeCents, TotalConsumeText: amountText(currentWallet.TotalConsumeCents),
		},
	}
}

func (service *Service) requireRepository() (Repository, *apperror.Error) {
	if service == nil || service.repository == nil {
		return nil, newAppError(ErrorServiceMissing, apperror.CategoryInternal, http.StatusInternalServerError, apperror.Permanent, "兑换码服务未配置", nil)
	}
	return service.repository, nil
}

func operationNow(value clock.Clock) time.Time {
	if value == nil {
		value = clock.SystemClock{}
	}
	return value.Now().UTC().Truncate(time.Microsecond)
}

func amountText(cents int64) string { return fmt.Sprintf("%d.%02d", cents/100, cents%100) }

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02T15:04:05.000000Z07:00")
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTime(*value)
}

func totalPages(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int((total + int64(pageSize) - 1) / int64(pageSize))
}

func managementInvalid(message string) *apperror.Error {
	return newAppError(ErrorRequestInvalid, apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent, message, nil)
}

func managementConflict(code, message string) *apperror.Error {
	return newAppError(code, apperror.CategoryConflict, http.StatusConflict, apperror.Permanent, message, nil)
}

func managementDependency(cause error) *apperror.Error {
	return newAppError(ErrorDependencyUnavailable, apperror.CategoryDependency, http.StatusServiceUnavailable, apperror.Retryable, "兑换码服务暂不可用", cause)
}

func managementRepositoryError(err error) *apperror.Error {
	if errors.Is(err, ErrIntegrityViolation) {
		return managementIntegrity(nil)
	}
	return managementDependency(err)
}

func managementIntegrity(cause error) *apperror.Error {
	return newAppError(ErrorIntegrityViolation, apperror.CategoryInternal, http.StatusInternalServerError, apperror.Permanent, "兑换码数据完整性异常", cause)
}

func walletUnavailable() *apperror.Error {
	return newAppError(ErrorWalletUnavailable, apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent, "兑换码不可用", nil)
}

func walletRateLimited(retryAfter int) *apperror.Error {
	if retryAfter < 1 {
		retryAfter = 1
	}
	return apperror.New(ErrorWalletUnavailable, apperror.CategoryRateLimit, http.StatusTooManyRequests, apperror.Retryable, ErrorWalletUnavailable, map[string]any{"retry_after": retryAfter}, "兑换请求过于频繁")
}

func walletRateDependency(cause error) *apperror.Error {
	return newAppError(ErrorWalletRateLimitUnavailable, apperror.CategoryDependency, http.StatusServiceUnavailable, apperror.Retryable, "兑换限流服务暂不可用", cause)
}

func walletDependency(cause error) *apperror.Error {
	return newAppError(ErrorWalletDependencyUnavailable, apperror.CategoryDependency, http.StatusServiceUnavailable, apperror.Retryable, "兑换服务暂不可用", cause)
}

func walletIntegrity(cause error) *apperror.Error {
	return newAppError(ErrorWalletIntegrityViolation, apperror.CategoryInternal, http.StatusInternalServerError, apperror.Permanent, "兑换事实完整性异常", cause)
}

func newAppError(code string, category apperror.Category, status int, retry apperror.RetryClass, message string, cause error) *apperror.Error {
	if cause != nil {
		return apperror.Wrap(code, category, status, retry, code, nil, message, cause)
	}
	return apperror.New(code, category, status, retry, code, nil, message)
}
