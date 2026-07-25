package runtime

import (
	"context"
	"fmt"
	"time"

	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/ai/replycommand"
)

type replyAttemptRepository interface {
	PrepareAttempt(context.Context, replycommand.PrepareAttemptInput) (*replycommand.Attempt, bool, error)
	MarkAttemptDispatchedForRun(context.Context, int64, uint64, uint64, string, uint64, time.Time) (bool, error)
	FinishAttempt(context.Context, replycommand.FinishAttemptInput) (bool, error)
}

type replyAttemptRecorder struct {
	repository replyAttemptRepository
}

func (r replyAttemptRecorder) PrepareProviderAttempt(ctx context.Context, input aichat.ProviderAttemptPrepareInput) (*aichat.ProviderAttemptRef, error) {
	attempt, ok, err := r.repository.PrepareAttempt(ctx, replycommand.PrepareAttemptInput{RunID: input.RunID, CommandID: input.CommandID, Owner: input.Owner, Token: input.Token, Now: input.Now})
	if err != nil {
		return nil, err
	}
	if !ok || attempt == nil {
		return nil, replycommand.ErrLeaseLost
	}
	return &aichat.ProviderAttemptRef{ID: attempt.ID, RunID: attempt.RunID, AttemptNo: attempt.AttemptNo, IdempotencyKey: attempt.IdempotencyKey, PreparedRequestJSON: attempt.PreparedRequestJSON, QuoteJSON: attempt.QuoteJSON}, nil
}

func (r replyAttemptRecorder) MarkProviderAttemptDispatched(ctx context.Context, input aichat.ProviderAttemptMarkInput) error {
	ok, err := r.repository.MarkAttemptDispatchedForRun(ctx, input.RunID, input.AttemptID, input.CommandID, input.Owner, input.Token, input.Now)
	if err != nil {
		return err
	}
	if !ok {
		return replycommand.ErrLeaseLost
	}
	return nil
}

func (r replyAttemptRecorder) FinishProviderAttempt(ctx context.Context, input aichat.ProviderAttemptFinishInput) error {
	state, err := replyAttemptState(input.State)
	if err != nil {
		return err
	}
	ok, err := r.repository.FinishAttempt(ctx, replycommand.FinishAttemptInput{
		RunID:               input.RunID,
		AttemptID:           input.AttemptID,
		CommandID:           input.CommandID,
		Owner:               input.Owner,
		Token:               input.Token,
		State:               state,
		ProviderRequestID:   input.ProviderRequestID,
		ResponseSHA256:      input.ResponseSHA256,
		ErrorCode:           input.ErrorCode,
		DispatchState:       input.DispatchState,
		UsageJSON:           input.UsageJSON,
		UsageStatus:         input.UsageStatus,
		ResultCandidateJSON: input.ResultCandidateJSON,
		Now:                 input.Now,
	})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("finish provider attempt %d: %w", input.AttemptID, replycommand.ErrLeaseLost)
	}
	return nil
}

func replyAttemptState(state aichat.ProviderAttemptState) (replycommand.AttemptState, error) {
	switch state {
	case aichat.ProviderAttemptSucceeded:
		return replycommand.AttemptSucceeded, nil
	case aichat.ProviderAttemptFailed:
		return replycommand.AttemptFailed, nil
	case aichat.ProviderAttemptCanceled:
		return replycommand.AttemptCanceled, nil
	case aichat.ProviderAttemptOutcomeUnknown:
		return replycommand.AttemptOutcomeUnknown, nil
	default:
		return "", fmt.Errorf("unknown provider attempt state %q", state)
	}
}
