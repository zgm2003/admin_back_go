package aitext

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"admin_back_go/internal/module/ai/requestidentity"
	"admin_back_go/internal/shared/apperror"
)

const (
	ErrorCodeRequestInvalid      = "ai.text.request_invalid"
	ErrorCodeTaskMissing         = "ai.text.task_missing"
	ErrorCodeQueueUnavailable    = "ai.text.queue_unavailable"
	ErrorCodeConfiguration       = "ai.text.configuration_error"
	ErrorCodePriceUnavailable    = "ai.billing.price_unavailable"
	ErrorCodeUnsafeUpperBound    = "ai.billing.unsafe_upper_bound"
	ErrorCodeInsufficientBalance = "ai.billing.insufficient_balance"
	ErrorCodeUsageIncomplete     = "ai.billing.usage_incomplete"
	ErrorCodeProviderFailed      = "ai.provider_failed"
)

type Waker interface {
	WakeTextTask(context.Context, uint64) error
}

type TaskExecutor interface {
	ExecuteTextTask(context.Context, uint64) error
}

type ServiceDependencies struct {
	Store        DurableStore
	Waker        Waker
	Executor     TaskExecutor
	WaitInterval time.Duration
}

type Service struct {
	store        DurableStore
	waker        Waker
	executor     TaskExecutor
	waitInterval time.Duration
}

type Result struct {
	TaskID           uint64
	RunID            int64
	RequestID        string
	Kind             string
	Answer           string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type ReplayInput struct {
	UserID         int64
	RequestID      string
	AgentID        uint64
	Operation      string
	Modality       string
	NormalizedText string
}

type ProviderInputSnapshot struct {
	Version         string `json:"version"`
	Operation       string `json:"operation"`
	Modality        string `json:"modality"`
	NormalizedText  string `json:"normalized_text"`
	MaxOutputTokens int64  `json:"max_output_tokens"`
	Prompt          string `json:"prompt"`
	SystemPrompt    string `json:"system_prompt,omitempty"`
}

const providerInputSnapshotVersion = "ai_text_input_v1"

func EncodeProviderInputSnapshot(input ProviderInputSnapshot) (string, error) {
	input.Version = providerInputSnapshotVersion
	input.Operation = strings.TrimSpace(input.Operation)
	input.Modality = strings.TrimSpace(input.Modality)
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.SystemPrompt = strings.TrimSpace(input.SystemPrompt)
	if input.Operation == "" || input.Modality == "" || input.MaxOutputTokens <= 0 || input.Prompt == "" {
		return "", ErrAcceptInputInvalid
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func DecodeProviderInputSnapshot(raw string) (ProviderInputSnapshot, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	var input ProviderInputSnapshot
	if err := decoder.Decode(&input); err != nil {
		return ProviderInputSnapshot{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ProviderInputSnapshot{}, ErrAcceptInputInvalid
	}
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.SystemPrompt = strings.TrimSpace(input.SystemPrompt)
	input.Operation = strings.TrimSpace(input.Operation)
	input.Modality = strings.TrimSpace(input.Modality)
	if input.Version != providerInputSnapshotVersion || input.Operation == "" || input.Modality == "" || input.MaxOutputTokens <= 0 || input.Prompt == "" {
		return ProviderInputSnapshot{}, ErrAcceptInputInvalid
	}
	return input, nil
}

type resultCandidate struct {
	Version string `json:"version"`
	Answer  string `json:"answer"`
}

const resultCandidateVersion = "ai_text_result_v1"

func MarshalResultCandidate(answer string) (string, error) {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "", ErrCandidateConflict
	}
	raw, err := json.Marshal(resultCandidate{Version: resultCandidateVersion, Answer: answer})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func AnswerFromResultCandidate(raw string) (string, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	var candidate resultCandidate
	if err := decoder.Decode(&candidate); err != nil {
		return "", err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return "", ErrCandidateConflict
	}
	candidate.Answer = strings.TrimSpace(candidate.Answer)
	if candidate.Version != resultCandidateVersion || candidate.Answer == "" {
		return "", ErrCandidateConflict
	}
	return candidate.Answer, nil
}

func NewService(deps ServiceDependencies) *Service {
	interval := deps.WaitInterval
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	return &Service{store: deps.Store, waker: deps.Waker, executor: deps.Executor, waitInterval: interval}
}

func (s *Service) Submit(ctx context.Context, input AcceptInput) (*TextTask, *apperror.Error) {
	input.RequestID = strings.TrimSpace(input.RequestID)
	if input.RequestID == "" || utf8.RuneCountInString(input.RequestID) > 128 {
		return nil, applicationError(ErrorCodeRequestInvalid, apperror.CategoryValidation, http.StatusBadRequest, "request_id不能为空", nil)
	}
	if s == nil || s.store == nil {
		return nil, applicationError(ErrorCodeConfiguration, apperror.CategoryInternal, http.StatusInternalServerError, "AI文本任务仓储未配置", ErrStoreNotConfigured)
	}
	acceptCtx := context.Background()
	if ctx != nil {
		acceptCtx = context.WithoutCancel(ctx)
	}
	task, err := s.store.Accept(acceptCtx, input)
	if err != nil {
		if errors.Is(err, requestidentity.ErrRequestIdentityConflict) || errors.Is(err, requestidentity.ErrRequestIdentityNotReplayable) {
			return nil, applicationError(requestidentity.ErrorCodeFingerprintConflict, apperror.CategoryConflict, http.StatusConflict, "request_id与原请求内容冲突", err)
		}
		return nil, applicationError(ErrorCodeConfiguration, apperror.CategoryInternal, http.StatusInternalServerError, "提交AI文本任务失败", err)
	}
	if task == nil || task.ID == 0 || task.RunID <= 0 {
		return nil, applicationError(ErrorCodeConfiguration, apperror.CategoryInternal, http.StatusInternalServerError, "AI文本任务接受结果无效", ErrAcceptInputInvalid)
	}
	if task.Status == StatusRunning {
		if s.waker == nil {
			return nil, applicationError(ErrorCodeQueueUnavailable, apperror.CategoryDependency, http.StatusServiceUnavailable, "AI文本任务队列未配置", nil)
		}
		wakeCtx := context.Background()
		if ctx != nil {
			wakeCtx = context.WithoutCancel(ctx)
		}
		if err := s.waker.WakeTextTask(wakeCtx, task.ID); err != nil {
			return nil, applicationError(ErrorCodeQueueUnavailable, apperror.CategoryDependency, http.StatusServiceUnavailable, "唤醒AI文本任务失败", err)
		}
	}
	return task, nil
}

func (s *Service) SubmitAndWait(ctx context.Context, input AcceptInput) (*Result, *apperror.Error) {
	task, appErr := s.Submit(ctx, input)
	if appErr != nil {
		return nil, appErr
	}
	return s.wait(ctx, task.ID, input.UserID)
}

func (s *Service) ReplayAndWait(ctx context.Context, input ReplayInput) (*Result, bool, *apperror.Error) {
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.Operation = strings.TrimSpace(input.Operation)
	input.Modality = strings.TrimSpace(input.Modality)
	if input.UserID <= 0 || input.RequestID == "" || utf8.RuneCountInString(input.RequestID) > 128 ||
		input.AgentID == 0 || input.AgentID > uint64(math.MaxInt64) || input.Operation == "" || input.Modality == "" {
		return nil, false, applicationError(ErrorCodeRequestInvalid, apperror.CategoryValidation, http.StatusBadRequest, "AI文本重放身份无效", nil)
	}
	if s == nil || s.store == nil {
		return nil, false, applicationError(ErrorCodeConfiguration, apperror.CategoryInternal, http.StatusInternalServerError, "AI文本任务仓储未配置", ErrStoreNotConfigured)
	}
	replay, err := s.store.FindReplay(ctx, input.UserID, input.RequestID)
	if err != nil {
		return nil, false, applicationError(ErrorCodeConfiguration, apperror.CategoryInternal, http.StatusInternalServerError, "查询AI文本重放任务失败", err)
	}
	if replay == nil {
		return nil, false, nil
	}
	snapshot, err := DecodeProviderInputSnapshot(replay.InputSnapshot)
	if err != nil {
		return nil, true, applicationError(ErrorCodeConfiguration, apperror.CategoryInternal, http.StatusInternalServerError, "AI文本重放快照无效", err)
	}
	fingerprint, err := requestidentity.BuildFingerprint(requestidentity.Input{
		UserID: input.UserID, Operation: input.Operation, Modality: input.Modality, AgentID: int64(input.AgentID),
		ModelID: replay.Task.ModelID, NormalizedText: input.NormalizedText,
		Options: requestidentity.GenerationOptions{MaxOutputTokens: snapshot.MaxOutputTokens},
	})
	if err != nil {
		return nil, true, applicationError(ErrorCodeRequestInvalid, apperror.CategoryValidation, http.StatusBadRequest, "AI文本重放身份无效", err)
	}
	if err := compareFingerprint(replay.Task, fingerprint); err != nil {
		if errors.Is(err, requestidentity.ErrRequestIdentityConflict) || errors.Is(err, requestidentity.ErrRequestIdentityNotReplayable) {
			return nil, true, applicationError(requestidentity.ErrorCodeFingerprintConflict, apperror.CategoryConflict, http.StatusConflict, "request_id与原请求内容冲突", err)
		}
		return nil, true, applicationError(ErrorCodeConfiguration, apperror.CategoryInternal, http.StatusInternalServerError, "AI文本重放身份无效", err)
	}
	if replay.Task.Status == StatusRunning {
		if s.waker == nil {
			return nil, true, applicationError(ErrorCodeQueueUnavailable, apperror.CategoryDependency, http.StatusServiceUnavailable, "AI文本任务队列未配置", nil)
		}
		wakeCtx := context.Background()
		if ctx != nil {
			wakeCtx = context.WithoutCancel(ctx)
		}
		if err := s.waker.WakeTextTask(wakeCtx, replay.Task.ID); err != nil {
			return nil, true, applicationError(ErrorCodeQueueUnavailable, apperror.CategoryDependency, http.StatusServiceUnavailable, "唤醒AI文本任务失败", err)
		}
	}
	result, appErr := s.wait(ctx, replay.Task.ID, input.UserID)
	return result, true, appErr
}

func (s *Service) wait(ctx context.Context, taskID uint64, userID int64) (*Result, *apperror.Error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		task, err := s.store.FindByID(ctx, taskID)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, applicationError("request.canceled", apperror.CategoryCanceled, http.StatusRequestTimeout, "等待AI文本任务已取消", ctxErr)
			}
			return nil, applicationError(ErrorCodeTaskMissing, apperror.CategoryInternal, http.StatusInternalServerError, "查询AI文本任务失败", err)
		}
		if task == nil || task.ID != taskID || task.UserID != userID {
			return nil, applicationError(ErrorCodeTaskMissing, apperror.CategoryNotFound, http.StatusNotFound, "AI文本任务不存在", nil)
		}
		switch task.Status {
		case StatusSuccess:
			if task.Answer == nil || strings.TrimSpace(*task.Answer) == "" {
				return nil, applicationError(ErrorCodeProviderFailed, apperror.CategoryInternal, http.StatusInternalServerError, "AI文本任务结果缺失", nil)
			}
			return &Result{
				TaskID: task.ID, RunID: task.RunID, RequestID: task.RequestID, Kind: task.Kind, Answer: *task.Answer,
				PromptTokens: int(task.PromptTokens), CompletionTokens: int(task.CompletionTokens), TotalTokens: int(task.TotalTokens),
			}, nil
		case StatusFailed:
			return nil, terminalTaskError(task)
		case StatusRunning:
		default:
			return nil, applicationError(ErrorCodeConfiguration, apperror.CategoryInternal, http.StatusInternalServerError, "AI文本任务状态无效", nil)
		}
		timer := time.NewTimer(s.waitInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, applicationError("request.canceled", apperror.CategoryCanceled, http.StatusRequestTimeout, "等待AI文本任务已取消", ctx.Err())
		case <-timer.C:
		}
	}
}

func (s *Service) ExecuteTask(ctx context.Context, taskID uint64) error {
	if s == nil || s.executor == nil {
		return ErrStoreNotConfigured
	}
	if taskID == 0 {
		return ErrAcceptInputInvalid
	}
	return s.executor.ExecuteTextTask(ctx, taskID)
}

func terminalTaskError(task *TextTask) *apperror.Error {
	code := strings.TrimSpace(task.LastErrorCode)
	message := "AI文本生成失败"
	if task.ErrorMessage != nil && strings.TrimSpace(*task.ErrorMessage) != "" {
		message = strings.TrimSpace(*task.ErrorMessage)
	}
	switch code {
	case ErrorCodeInsufficientBalance:
		return applicationError(code, apperror.CategoryConflict, http.StatusConflict, message, nil)
	case ErrorCodePriceUnavailable, ErrorCodeUnsafeUpperBound:
		return applicationError(code, apperror.CategoryConflict, http.StatusConflict, message, nil)
	case ErrorCodeConfiguration:
		return applicationError(code, apperror.CategoryValidation, http.StatusBadRequest, message, nil)
	case ErrorCodeUsageIncomplete, ErrorCodeProviderFailed:
		return applicationError(code, apperror.CategoryDependency, http.StatusBadGateway, message, nil)
	default:
		if code == "" {
			code = ErrorCodeProviderFailed
		}
		return applicationError(code, apperror.CategoryDependency, http.StatusBadGateway, message, nil)
	}
}

func applicationError(code string, category apperror.Category, status int, message string, cause error) *apperror.Error {
	return apperror.Wrap(code, category, status, apperror.Permanent, "", nil, message, cause)
}
