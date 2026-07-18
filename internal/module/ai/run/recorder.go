package airun

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"admin_back_go/internal/shared/enum"
)

var (
	ErrRecorderRepositoryMissing = errors.New("airun recorder repository not configured")
	ErrRecorderInvalidInput      = errors.New("invalid AI run recorder input")
)

type Recorder interface {
	Start(ctx context.Context, input StartInput) (int64, error)
	Complete(ctx context.Context, input CompleteInput) error
	Fail(ctx context.Context, input FailInput) error
	Cancel(ctx context.Context, input CancelInput) error
	Timeout(ctx context.Context, input TimeoutInput) error
}

type RecorderRepository interface {
	StartRun(ctx context.Context, input StartRecord) (int64, error)
	CompleteRun(ctx context.Context, input CompleteRecord) error
	FinishRun(ctx context.Context, input FinishRecord) error
}

type RunRecorder struct {
	repository RecorderRepository
	now        func() time.Time
}

func NewRecorder(repository RecorderRepository, now func() time.Time) *RunRecorder {
	if now == nil {
		now = time.Now
	}
	return &RunRecorder{repository: repository, now: now}
}

type StartInput struct {
	Platform         string
	IdempotencyKey   string
	ConversationID   *int64
	UserMessageID    *int64
	RequestID        string
	UserID           int64
	AgentID          int64
	ProviderID       int64
	ModelID          string
	ModelDisplayName string
	InputSnapshot    string
	StartedAt        time.Time
}

type CompleteInput struct {
	RunID              int64
	AssistantMessageID *int64
	PromptTokens       int
	CompletionTokens   int
	TotalTokens        int
	FinishedAt         time.Time
	DurationMS         uint
}

type FailInput struct {
	RunID      int64
	Message    string
	FinishedAt time.Time
	DurationMS uint
}

type CancelInput struct {
	RunID      int64
	Message    string
	FinishedAt time.Time
	DurationMS uint
}

type TimeoutInput struct {
	RunID      int64
	Message    string
	FinishedAt time.Time
	DurationMS uint
}

type StartRecord struct {
	Platform         string
	IdempotencyKey   string
	ConversationID   *int64
	UserMessageID    *int64
	RequestID        string
	UserID           int64
	AgentID          int64
	ProviderID       int64
	ModelID          string
	ModelDisplayName string
	InputSnapshot    string
	StartedAt        time.Time
}

type CompleteRecord = CompleteInput

type FinishRecord struct {
	RunID      int64
	Status     string
	EventType  string
	Message    string
	FinishedAt time.Time
	DurationMS uint
}

func (r *RunRecorder) Start(ctx context.Context, input StartInput) (int64, error) {
	if r == nil || r.repository == nil {
		return 0, ErrRecorderRepositoryMissing
	}
	record, err := normalizeStartInput(input, r.now())
	if err != nil {
		return 0, err
	}
	return r.repository.StartRun(ctx, record)
}

func (r *RunRecorder) Complete(ctx context.Context, input CompleteInput) error {
	if r == nil || r.repository == nil {
		return ErrRecorderRepositoryMissing
	}
	record, err := normalizeCompleteInput(input, r.now())
	if err != nil {
		return err
	}
	return r.repository.CompleteRun(ctx, record)
}

func (r *RunRecorder) Fail(ctx context.Context, input FailInput) error {
	return r.finish(ctx, FinishRecord{RunID: input.RunID, Status: enum.AIRunStatusFailed, EventType: enum.AIRunEventFailed, Message: input.Message, FinishedAt: input.FinishedAt, DurationMS: input.DurationMS})
}

func (r *RunRecorder) Cancel(ctx context.Context, input CancelInput) error {
	return r.finish(ctx, FinishRecord{RunID: input.RunID, Status: enum.AIRunStatusCanceled, EventType: enum.AIRunEventCanceled, Message: input.Message, FinishedAt: input.FinishedAt, DurationMS: input.DurationMS})
}

func (r *RunRecorder) Timeout(ctx context.Context, input TimeoutInput) error {
	return r.finish(ctx, FinishRecord{RunID: input.RunID, Status: enum.AIRunStatusTimeout, EventType: enum.AIRunEventTimeout, Message: input.Message, FinishedAt: input.FinishedAt, DurationMS: input.DurationMS})
}

func (r *RunRecorder) finish(ctx context.Context, input FinishRecord) error {
	if r == nil || r.repository == nil {
		return ErrRecorderRepositoryMissing
	}
	record, err := normalizeFinishInput(input, r.now())
	if err != nil {
		return err
	}
	return r.repository.FinishRun(ctx, record)
}

func normalizeStartInput(input StartInput, now time.Time) (StartRecord, error) {
	input.Platform = strings.TrimSpace(input.Platform)
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.ModelID = strings.TrimSpace(input.ModelID)
	input.ModelDisplayName = strings.TrimSpace(input.ModelDisplayName)
	if input.Platform == "" || !enum.IsPlatform(input.Platform) {
		return StartRecord{}, fmt.Errorf("%w: platform", ErrRecorderInvalidInput)
	}
	if input.RequestID == "" || utf8.RuneCountInString(input.RequestID) > 128 {
		return StartRecord{}, fmt.Errorf("%w: request_id", ErrRecorderInvalidInput)
	}
	if input.UserID <= 0 {
		return StartRecord{}, fmt.Errorf("%w: user_id", ErrRecorderInvalidInput)
	}
	if input.AgentID <= 0 {
		return StartRecord{}, fmt.Errorf("%w: agent_id", ErrRecorderInvalidInput)
	}
	if input.ProviderID <= 0 {
		return StartRecord{}, fmt.Errorf("%w: provider_id", ErrRecorderInvalidInput)
	}
	if input.ModelID == "" {
		return StartRecord{}, fmt.Errorf("%w: model_id", ErrRecorderInvalidInput)
	}
	if strings.TrimSpace(input.InputSnapshot) == "" {
		return StartRecord{}, fmt.Errorf("%w: input_snapshot", ErrRecorderInvalidInput)
	}
	if input.StartedAt.IsZero() {
		input.StartedAt = now
	}
	return StartRecord(input), nil
}

func normalizeCompleteInput(input CompleteInput, now time.Time) (CompleteRecord, error) {
	if input.RunID <= 0 {
		return CompleteRecord{}, fmt.Errorf("%w: run_id", ErrRecorderInvalidInput)
	}
	if input.PromptTokens < 0 || input.CompletionTokens < 0 || input.TotalTokens < 0 {
		return CompleteRecord{}, fmt.Errorf("%w: token_count", ErrRecorderInvalidInput)
	}
	if input.FinishedAt.IsZero() {
		input.FinishedAt = now
	}
	return CompleteRecord(input), nil
}

func normalizeFinishInput(input FinishRecord, now time.Time) (FinishRecord, error) {
	input.Message = strings.TrimSpace(input.Message)
	if input.RunID <= 0 {
		return FinishRecord{}, fmt.Errorf("%w: run_id", ErrRecorderInvalidInput)
	}
	if !enum.IsAIRunStatus(input.Status) || input.Status == enum.AIRunStatusRunning || input.Status == enum.AIRunStatusSuccess {
		return FinishRecord{}, fmt.Errorf("%w: terminal_status", ErrRecorderInvalidInput)
	}
	if !enum.IsAIRunEvent(input.EventType) || input.EventType == enum.AIRunEventStart || input.EventType == enum.AIRunEventCompleted {
		return FinishRecord{}, fmt.Errorf("%w: terminal_event", ErrRecorderInvalidInput)
	}
	if input.Message == "" {
		input.Message = enum.AIRunStatusLabels[input.Status]
	}
	if input.FinishedAt.IsZero() {
		input.FinishedAt = now
	}
	return input, nil
}
