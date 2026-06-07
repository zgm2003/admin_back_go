package airun

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
	CompleteRunBySource(ctx context.Context, input CompleteSourceRecord) error
	FinishRunBySource(ctx context.Context, input FinishSourceRecord) error
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
	Modality         string
	SourceType       string
	SourceID         uint64
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
	UsageStatus        string
	FinishedAt         time.Time
	DurationMS         uint
}

type CompleteSourceInput struct {
	SourceType         string
	SourceID           uint64
	AssistantMessageID *int64
	PromptTokens       int
	CompletionTokens   int
	TotalTokens        int
	UsageStatus        string
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

type FailSourceInput struct {
	SourceType string
	SourceID   uint64
	Message    string
	FinishedAt time.Time
	DurationMS uint
}

type CancelSourceInput struct {
	SourceType string
	SourceID   uint64
	Message    string
	FinishedAt time.Time
	DurationMS uint
}

type TimeoutSourceInput struct {
	SourceType string
	SourceID   uint64
	Message    string
	FinishedAt time.Time
	DurationMS uint
}

type StartRecord struct {
	Platform         string
	Modality         string
	SourceType       string
	SourceID         uint64
	ConversationID   *int64
	UserMessageID    *int64
	RequestID        string
	UserID           int64
	AgentID          int64
	ProviderID       int64
	ModelID          string
	ModelDisplayName string
	InputSnapshot    string
	UsageStatus      string
	StartedAt        time.Time
}

type CompleteRecord = CompleteInput

type CompleteSourceRecord = CompleteSourceInput

type FinishRecord struct {
	RunID       int64
	Status      string
	EventType   string
	Message     string
	UsageStatus string
	FinishedAt  time.Time
	DurationMS  uint
}

type FinishSourceRecord struct {
	SourceType  string
	SourceID    uint64
	Status      string
	EventType   string
	Message     string
	UsageStatus string
	FinishedAt  time.Time
	DurationMS  uint
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

func (r *RunRecorder) CompleteSource(ctx context.Context, input CompleteSourceInput) error {
	if r == nil || r.repository == nil {
		return ErrRecorderRepositoryMissing
	}
	record, err := normalizeCompleteSourceInput(input, r.now())
	if err != nil {
		return err
	}
	return r.repository.CompleteRunBySource(ctx, record)
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

func (r *RunRecorder) FailSource(ctx context.Context, input FailSourceInput) error {
	return r.finishSource(ctx, FinishSourceRecord{SourceType: input.SourceType, SourceID: input.SourceID, Status: enum.AIRunStatusFailed, EventType: enum.AIRunEventFailed, Message: input.Message, FinishedAt: input.FinishedAt, DurationMS: input.DurationMS})
}

func (r *RunRecorder) CancelSource(ctx context.Context, input CancelSourceInput) error {
	return r.finishSource(ctx, FinishSourceRecord{SourceType: input.SourceType, SourceID: input.SourceID, Status: enum.AIRunStatusCanceled, EventType: enum.AIRunEventCanceled, Message: input.Message, FinishedAt: input.FinishedAt, DurationMS: input.DurationMS})
}

func (r *RunRecorder) TimeoutSource(ctx context.Context, input TimeoutSourceInput) error {
	return r.finishSource(ctx, FinishSourceRecord{SourceType: input.SourceType, SourceID: input.SourceID, Status: enum.AIRunStatusTimeout, EventType: enum.AIRunEventTimeout, Message: input.Message, FinishedAt: input.FinishedAt, DurationMS: input.DurationMS})
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

func (r *RunRecorder) finishSource(ctx context.Context, input FinishSourceRecord) error {
	if r == nil || r.repository == nil {
		return ErrRecorderRepositoryMissing
	}
	record, err := normalizeFinishSourceInput(input, r.now())
	if err != nil {
		return err
	}
	return r.repository.FinishRunBySource(ctx, record)
}

func normalizeStartInput(input StartInput, now time.Time) (StartRecord, error) {
	input.Platform = strings.TrimSpace(input.Platform)
	input.Modality = strings.TrimSpace(input.Modality)
	input.SourceType = strings.TrimSpace(input.SourceType)
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.ModelID = strings.TrimSpace(input.ModelID)
	input.ModelDisplayName = strings.TrimSpace(input.ModelDisplayName)
	if input.Platform == "" || !enum.IsPlatform(input.Platform) {
		return StartRecord{}, fmt.Errorf("%w: platform", ErrRecorderInvalidInput)
	}
	if input.Modality == "" || !enum.IsAIRunModality(input.Modality) {
		return StartRecord{}, fmt.Errorf("%w: modality", ErrRecorderInvalidInput)
	}
	if input.SourceType == "" || !enum.IsAIRunSourceType(input.SourceType) {
		return StartRecord{}, fmt.Errorf("%w: source_type", ErrRecorderInvalidInput)
	}
	if input.SourceID == 0 {
		return StartRecord{}, fmt.Errorf("%w: source_id", ErrRecorderInvalidInput)
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
	if input.RequestID == "" {
		input.RequestID = fmt.Sprintf("%s-%d", input.SourceType, input.SourceID)
	}
	if input.StartedAt.IsZero() {
		input.StartedAt = now
	}
	return StartRecord{
		Platform:         input.Platform,
		Modality:         input.Modality,
		SourceType:       input.SourceType,
		SourceID:         input.SourceID,
		ConversationID:   input.ConversationID,
		UserMessageID:    input.UserMessageID,
		RequestID:        input.RequestID,
		UserID:           input.UserID,
		AgentID:          input.AgentID,
		ProviderID:       input.ProviderID,
		ModelID:          input.ModelID,
		ModelDisplayName: input.ModelDisplayName,
		InputSnapshot:    input.InputSnapshot,
		UsageStatus:      enum.AIRunUsagePending,
		StartedAt:        input.StartedAt,
	}, nil
}

func normalizeCompleteInput(input CompleteInput, now time.Time) (CompleteRecord, error) {
	input.UsageStatus = strings.TrimSpace(input.UsageStatus)
	if input.RunID <= 0 {
		return CompleteRecord{}, fmt.Errorf("%w: run_id", ErrRecorderInvalidInput)
	}
	if input.PromptTokens < 0 || input.CompletionTokens < 0 || input.TotalTokens < 0 {
		return CompleteRecord{}, fmt.Errorf("%w: token_count", ErrRecorderInvalidInput)
	}
	if !enum.IsAIRunTerminalUsageStatus(input.UsageStatus) {
		return CompleteRecord{}, fmt.Errorf("%w: usage_status", ErrRecorderInvalidInput)
	}
	if input.FinishedAt.IsZero() {
		input.FinishedAt = now
	}
	return CompleteRecord(input), nil
}

func normalizeCompleteSourceInput(input CompleteSourceInput, now time.Time) (CompleteSourceRecord, error) {
	input.SourceType = strings.TrimSpace(input.SourceType)
	input.UsageStatus = strings.TrimSpace(input.UsageStatus)
	if input.SourceType == "" || !enum.IsAIRunSourceType(input.SourceType) {
		return CompleteSourceRecord{}, fmt.Errorf("%w: source_type", ErrRecorderInvalidInput)
	}
	if input.SourceID == 0 {
		return CompleteSourceRecord{}, fmt.Errorf("%w: source_id", ErrRecorderInvalidInput)
	}
	if input.PromptTokens < 0 || input.CompletionTokens < 0 || input.TotalTokens < 0 {
		return CompleteSourceRecord{}, fmt.Errorf("%w: token_count", ErrRecorderInvalidInput)
	}
	if !enum.IsAIRunTerminalUsageStatus(input.UsageStatus) {
		return CompleteSourceRecord{}, fmt.Errorf("%w: usage_status", ErrRecorderInvalidInput)
	}
	if input.FinishedAt.IsZero() {
		input.FinishedAt = now
	}
	return CompleteSourceRecord(input), nil
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
	input.UsageStatus = enum.AIRunUsageUnavailable
	if input.FinishedAt.IsZero() {
		input.FinishedAt = now
	}
	return input, nil
}

func normalizeFinishSourceInput(input FinishSourceRecord, now time.Time) (FinishSourceRecord, error) {
	input.SourceType = strings.TrimSpace(input.SourceType)
	input.Message = strings.TrimSpace(input.Message)
	if input.SourceType == "" || !enum.IsAIRunSourceType(input.SourceType) {
		return FinishSourceRecord{}, fmt.Errorf("%w: source_type", ErrRecorderInvalidInput)
	}
	if input.SourceID == 0 {
		return FinishSourceRecord{}, fmt.Errorf("%w: source_id", ErrRecorderInvalidInput)
	}
	if !enum.IsAIRunStatus(input.Status) || input.Status == enum.AIRunStatusRunning || input.Status == enum.AIRunStatusSuccess {
		return FinishSourceRecord{}, fmt.Errorf("%w: terminal_status", ErrRecorderInvalidInput)
	}
	if !enum.IsAIRunEvent(input.EventType) || input.EventType == enum.AIRunEventStart || input.EventType == enum.AIRunEventCompleted {
		return FinishSourceRecord{}, fmt.Errorf("%w: terminal_event", ErrRecorderInvalidInput)
	}
	if input.Message == "" {
		input.Message = enum.AIRunStatusLabels[input.Status]
	}
	input.UsageStatus = enum.AIRunUsageUnavailable
	if input.FinishedAt.IsZero() {
		input.FinishedAt = now
	}
	return input, nil
}
