package replycommand

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	aichat "admin_back_go/internal/module/ai/chat"
)

func TestRunnerClaimsTransitionsAndCompletesCommand(t *testing.T) {
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	repository := &fakeRunnerRepository{claim: &Claim{
		Command: Command{ID: 41, ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "request-1", State: StateClaimed, AttemptCount: 1, MaxAttempts: 3},
		Owner:   "worker-a", FencingToken: 2, LeaseExpiresAt: now.Add(time.Minute),
	}, renewal: Renewal{Alive: true}}
	executor := &fakeReplyExecutor{result: &aichat.ConversationReplyResult{ConversationID: 3, AssistantMessageID: 22}}
	runner := NewRunner(RunnerOptions{Repository: repository, Executor: executor, Owner: "worker-a", LeaseTTL: time.Minute, Now: func() time.Time { return now }})

	worked, err := runner.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("RunOnce worked=%v err=%v", worked, err)
	}
	if executor.input.CommandID != 41 || executor.input.LeaseOwner != "worker-a" || executor.input.LeaseToken != 2 || executor.input.ConversationID != 3 || executor.input.UserID != 7 || executor.input.UserMessageID != 9 || executor.input.RequestID != "request-1" {
		t.Fatalf("executor input=%+v", executor.input)
	}
	want := []stateTransition{{from: StateClaimed, to: StateRunning}}
	if len(repository.transitions) != len(want) {
		t.Fatalf("transitions=%+v", repository.transitions)
	}
	for index := range want {
		if repository.transitions[index].from != want[index].from || repository.transitions[index].to != want[index].to {
			t.Fatalf("transitions=%+v", repository.transitions)
		}
	}
}

func TestRunnerCancelsExecutionAndDoesNotTerminalizeAfterLeaseLoss(t *testing.T) {
	now := time.Now()
	repository := &fakeRunnerRepository{claim: &Claim{
		Command: Command{ID: 42, ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "request-2", State: StateClaimed, AttemptCount: 1, MaxAttempts: 3},
		Owner:   "worker-a", FencingToken: 1, LeaseExpiresAt: now.Add(30 * time.Millisecond),
	}, renewal: Renewal{}}
	executor := &fakeReplyExecutor{blockUntilCanceled: true}
	runner := NewRunner(RunnerOptions{Repository: repository, Executor: executor, Owner: "worker-a", LeaseTTL: 30 * time.Millisecond, Now: time.Now})

	worked, err := runner.RunOnce(context.Background())
	if !worked || !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("RunOnce worked=%v err=%v", worked, err)
	}
	if len(repository.transitions) != 1 || repository.transitions[0].to != StateRunning {
		t.Fatalf("lease-lost worker terminalized command: %+v", repository.transitions)
	}
}

func TestRunnerTerminalizesDurableCancellation(t *testing.T) {
	now := time.Now()
	repository := &fakeRunnerRepository{claim: &Claim{
		Command: Command{ID: 43, ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "request-3", State: StateClaimed, AttemptCount: 1, MaxAttempts: 3},
		Owner:   "worker-a", FencingToken: 1, LeaseExpiresAt: now.Add(30 * time.Millisecond),
	}, renewal: Renewal{Alive: true, CancelRequested: true}}
	executor := &fakeReplyExecutor{blockUntilCanceled: true}
	runner := NewRunner(RunnerOptions{Repository: repository, Executor: executor, Owner: "worker-a", LeaseTTL: 30 * time.Millisecond, Now: time.Now})

	worked, err := runner.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("RunOnce worked=%v err=%v", worked, err)
	}
	if len(repository.transitions) != 2 || repository.transitions[0].to != StateRunning || repository.transitions[1].from != StateRunning || repository.transitions[1].to != StateCanceled {
		t.Fatalf("transitions=%+v", repository.transitions)
	}
	if executor.calls != 0 {
		t.Fatalf("provider executed despite pre-existing durable cancellation: calls=%d", executor.calls)
	}
}

func TestRunnerUsesCancelSignalWithoutWaitingForRenewInterval(t *testing.T) {
	now := time.Now()
	repository := &fakeRunnerRepository{claim: &Claim{
		Command: Command{ID: 44, ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "request-4", State: StateClaimed, AttemptCount: 1, MaxAttempts: 3},
		Owner:   "worker-a", FencingToken: 1, LeaseExpiresAt: now.Add(time.Minute),
	}, renewals: []Renewal{{Alive: true}, {Alive: true, CancelRequested: true}}}
	signal := make(chan struct{}, 1)
	signal <- struct{}{}
	executor := &fakeReplyExecutor{blockUntilCanceled: true}
	runner := NewRunner(RunnerOptions{
		Repository:       repository,
		Executor:         executor,
		Owner:            "worker-a",
		LeaseTTL:         time.Minute,
		Now:              time.Now,
		CancelSubscriber: fakeCancelSubscriber{subscription: &fakeCancelSubscription{signal: signal}},
	})

	started := time.Now()
	worked, err := runner.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("RunOnce worked=%v err=%v", worked, err)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("cancel signal waited for lease renewal: %s", time.Since(started))
	}
	if len(repository.transitions) != 2 || repository.transitions[1].to != StateCanceled {
		t.Fatalf("transitions=%+v", repository.transitions)
	}
	if !errors.Is(executor.cancelCause, infraai.ErrCanceled) {
		t.Fatalf("cancel cause=%v", executor.cancelCause)
	}
}

func TestRunnerMovesAmbiguousProviderFailureToOutcomeUnknownWithoutRetry(t *testing.T) {
	now := time.Now()
	repository := &fakeRunnerRepository{claim: &Claim{
		Command: Command{ID: 45, ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "request-5", State: StateClaimed, AttemptCount: 1, MaxAttempts: 3},
		Owner:   "worker-a", FencingToken: 1, LeaseExpiresAt: now.Add(time.Minute),
	}, renewal: Renewal{Alive: true}}
	executor := &fakeReplyExecutor{err: infraai.NewProviderError(infraai.ProviderOutcomeUnknown, "provider-request-5", errors.New("stream disconnected"))}
	runner := NewRunner(RunnerOptions{Repository: repository, Executor: executor, Owner: "worker-a", LeaseTTL: time.Minute, Now: func() time.Time { return now }})

	worked, err := runner.RunOnce(context.Background())
	if !worked || err == nil {
		t.Fatalf("RunOnce worked=%v err=%v", worked, err)
	}
	if len(repository.transitions) != 2 || repository.transitions[1].from != StateRunning || repository.transitions[1].to != StateOutcomeUnknown || repository.transitions[1].values["last_error_code"] != "ai.provider_outcome_unknown" {
		t.Fatalf("transitions=%+v", repository.transitions)
	}
}

type fakeCancelSubscriber struct {
	subscription CancelSubscription
}

func (f fakeCancelSubscriber) SubscribeCancel(context.Context, uint64) (CancelSubscription, error) {
	return f.subscription, nil
}

type stateTransition struct {
	from   State
	to     State
	values map[string]any
}

type fakeRunnerRepository struct {
	mu          sync.Mutex
	claim       *Claim
	renewal     Renewal
	renewals    []Renewal
	renewIndex  int
	transitions []stateTransition
}

func (f *fakeRunnerRepository) ClaimNext(context.Context, string, time.Time, time.Duration) (*Claim, error) {
	return f.claim, nil
}
func (f *fakeRunnerRepository) ClaimByID(context.Context, uint64, string, time.Time, time.Duration) (*Claim, error) {
	return f.claim, nil
}
func (f *fakeRunnerRepository) Renew(context.Context, uint64, string, uint64, time.Time) (Renewal, error) {
	if f.renewIndex < len(f.renewals) {
		result := f.renewals[f.renewIndex]
		f.renewIndex++
		return result, nil
	}
	return f.renewal, nil
}
func (f *fakeRunnerRepository) Transition(_ context.Context, _ uint64, _ string, _ uint64, from State, to State, values map[string]any) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	copyValues := make(map[string]any, len(values))
	for key, value := range values {
		copyValues[key] = value
	}
	f.transitions = append(f.transitions, stateTransition{from: from, to: to, values: copyValues})
	return true, nil
}

type fakeReplyExecutor struct {
	input              aichat.ConversationReplyInput
	result             *aichat.ConversationReplyResult
	err                error
	blockUntilCanceled bool
	calls              int
	cancelCause        error
}

func (f *fakeReplyExecutor) ExecuteConversationReply(ctx context.Context, input aichat.ConversationReplyInput) (*aichat.ConversationReplyResult, error) {
	f.calls++
	f.input = input
	if f.blockUntilCanceled {
		<-ctx.Done()
		f.cancelCause = context.Cause(ctx)
		return nil, ctx.Err()
	}
	return f.result, f.err
}
