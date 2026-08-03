package replycommand

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/module/ai/contextengine"
)

func TestContextTerminalMatrixUnpaidFailuresNeverRemainRunning(t *testing.T) {
	codes := []contextengine.ErrorCode{
		contextengine.ErrCodeProfileUnavailable,
		contextengine.ErrCodeEmbeddingFailed,
		contextengine.ErrCodeIndexFailed,
		contextengine.ErrCodeIndexInconsistent,
		contextengine.ErrCodeSnapshotConflict,
		contextengine.ErrCodePermissionDenied,
		contextengine.ErrCodeRetrievalFailed,
		contextengine.ErrCodeRerankFailed,
		contextengine.ErrCodeRequiredOverflow,
		contextengine.ErrCodeToolContinuationOverflow,
		contextengine.ErrCodeAttachmentUnavailable,
		contextengine.ErrCodeMemoryUnavailable,
		contextengine.ErrCodePlanConflict,
	}
	for index, code := range codes {
		t.Run(string(code), func(t *testing.T) {
			now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
			repository := &fakeRunnerRepository{claim: &Claim{
				Command: Command{
					ID: uint64(index + 1), ConversationID: 3, UserID: 7, UserMessageID: 9,
					RequestID: string(code), State: StateClaimed, AttemptCount: 1, MaxAttempts: 1,
				},
				Owner: "worker-context", FencingToken: 1,
			}, renewal: Renewal{Alive: true}}
			appErr, err := contextengine.NewContextAppError(code, nil)
			if err != nil {
				t.Fatal(err)
			}
			runner := NewRunner(RunnerOptions{
				Repository: repository, Executor: &fakeReplyExecutor{err: appErr}, Owner: "worker-context",
				LeaseTTL: time.Minute, Now: func() time.Time { return now },
			})

			worked, runErr := runner.RunOnce(context.Background())
			if !worked || runErr == nil {
				t.Fatalf("worked=%v err=%v", worked, runErr)
			}
			if len(repository.transitions) != 2 || repository.transitions[0].to != StateRunning || repository.transitions[1].to != StateFailed {
				t.Fatalf("Context failure transitions=%+v", repository.transitions)
			}
			if repository.transitions[1].values["last_error_code"] != string(code) {
				t.Fatalf("terminal error facts=%+v", repository.transitions[1].values)
			}
		})
	}
}
