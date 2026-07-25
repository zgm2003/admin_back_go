package runtime

import (
	"context"
	"testing"
	"time"

	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/ai/replycommand"
)

type fakeReplyAttemptRepository struct {
	prepared replycommand.PrepareAttemptInput
	marked   replycommand.Attempt
	finished replycommand.FinishAttemptInput
}

func (f *fakeReplyAttemptRepository) PrepareLegacyAttempt(_ context.Context, input replycommand.PrepareAttemptInput) (*replycommand.Attempt, bool, error) {
	f.prepared = input
	return &replycommand.Attempt{ID: 91, RunID: input.RunID, IdempotencyKey: "attempt-key"}, true, nil
}

func (f *fakeReplyAttemptRepository) MarkAttemptDispatchedForRun(_ context.Context, runID int64, attemptID uint64, commandID uint64, owner string, token uint64, now time.Time) (bool, error) {
	f.marked = replycommand.Attempt{ID: attemptID, CommandID: &commandID, ProviderRequestID: owner, AttemptNo: uint(token), UpdatedAt: now}
	f.marked.RunID = runID
	return true, nil
}

func (f *fakeReplyAttemptRepository) FinishAttempt(_ context.Context, input replycommand.FinishAttemptInput) (bool, error) {
	f.finished = input
	return true, nil
}

func TestReplyAttemptRecorderMapsLifecycleWithoutFieldFallback(t *testing.T) {
	repository := &fakeReplyAttemptRepository{}
	recorder := replyAttemptRecorder{repository: repository}
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	attempt, err := recorder.PrepareProviderAttempt(context.Background(), aichat.ProviderAttemptPrepareInput{RunID: 100, CommandID: 41, Owner: "worker-a", Token: 7, Now: now})
	if err != nil || attempt.ID != 91 || attempt.IdempotencyKey != "attempt-key" {
		t.Fatalf("attempt=%+v err=%v", attempt, err)
	}
	if err := recorder.MarkProviderAttemptDispatched(context.Background(), aichat.ProviderAttemptMarkInput{RunID: 100, AttemptID: 91, CommandID: 41, Owner: "worker-a", Token: 7, Now: now}); err != nil {
		t.Fatal(err)
	}
	if err := recorder.FinishProviderAttempt(context.Background(), aichat.ProviderAttemptFinishInput{RunID: 100, AttemptID: 91, CommandID: 41, Owner: "worker-a", Token: 7, State: aichat.ProviderAttemptOutcomeUnknown, ProviderRequestID: "provider-request-1", ResponseSHA256: "hash", ErrorCode: "ai.provider_outcome_unknown", DispatchState: "unknown", UsageJSON: `{"status":"unavailable"}`, UsageStatus: "unavailable", Now: now}); err != nil {
		t.Fatal(err)
	}
	if repository.prepared.RunID != 100 || repository.marked.RunID != 100 || repository.finished.RunID != 100 || repository.finished.State != replycommand.AttemptOutcomeUnknown || repository.finished.ProviderRequestID != "provider-request-1" || repository.finished.ResponseSHA256 != "hash" || repository.finished.UsageStatus != "unavailable" || repository.finished.DispatchState != "unknown" {
		t.Fatalf("prepared=%+v marked=%+v finished=%+v", repository.prepared, repository.marked, repository.finished)
	}
}
