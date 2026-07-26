package replycommand

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/aigateway"
	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/shared/apperror"
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

func TestRunnerCancelStopsDeliveryButKeepsDrainAndLeaseAlive(t *testing.T) {
	now := time.Now()
	renewed := make(chan int, 8)
	repository := &fakeRunnerRepository{claim: &Claim{
		Command: Command{ID: 44, ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "request-4", State: StateClaimed, AttemptCount: 1, MaxAttempts: 3},
		Owner:   "worker-a", FencingToken: 1, LeaseExpiresAt: now.Add(30 * time.Millisecond),
	}, renewals: []Renewal{{Alive: true}, {Alive: true, CancelRequested: true}}, renewal: Renewal{Alive: true, CancelRequested: true}, renewed: renewed}
	signal := make(chan struct{}, 1)
	signal <- struct{}{}
	allowDrainFinish := make(chan struct{})
	executor := &fakeReplyExecutor{
		waitForDeliveryStop: true,
		releaseAfterStop:    allowDrainFinish,
	}
	runner := NewRunner(RunnerOptions{
		Repository:       repository,
		Executor:         executor,
		Owner:            "worker-a",
		LeaseTTL:         30 * time.Millisecond,
		Now:              time.Now,
		CancelSubscriber: fakeCancelSubscriber{subscription: &fakeCancelSubscription{signal: signal}},
	})

	type outcome struct {
		worked bool
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		worked, err := runner.RunOnce(context.Background())
		done <- outcome{worked: worked, err: err}
	}()

	deadline := time.After(time.Second)
	for calls := 0; calls < 3; {
		select {
		case calls = <-renewed:
		case <-deadline:
			t.Fatalf("lease was not renewed while draining: calls=%d", calls)
		}
	}
	close(allowDrainFinish)
	result := <-done
	if result.err != nil || !result.worked {
		t.Fatalf("RunOnce worked=%v err=%v", result.worked, result.err)
	}
	if len(repository.transitions) != 2 || repository.transitions[1].to != StateCanceled {
		t.Fatalf("transitions=%+v", repository.transitions)
	}
	if !errors.Is(executor.deliveryCause, infraai.ErrCanceled) {
		t.Fatalf("delivery cause=%v", executor.deliveryCause)
	}
	if executor.drainCanceledBeforeRelease {
		t.Fatalf("user stop canceled provider drain: cause=%v", executor.cancelCause)
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

func TestRunnerUsesFinalizerForPreDispatchCancellation(t *testing.T) {
	now := time.Now()
	repository := &fakeRunnerRepository{claim: &Claim{
		Command: Command{ID: 46, ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "request-finalize", State: StateClaimed, AttemptCount: 1, MaxAttempts: 3},
		Owner:   "worker-a", FencingToken: 1,
	}, renewal: Renewal{Alive: true, CancelRequested: true}}
	finalizer := &fakeReplyFinalizer{result: &aichat.ConversationReplyResult{ConversationID: 3, Finalized: true}}
	runner := NewRunner(RunnerOptions{Repository: repository, Executor: &fakeReplyExecutor{}, Finalizer: finalizer, Owner: "worker-a", LeaseTTL: time.Minute, Now: func() time.Time { return now }})

	worked, err := runner.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("RunOnce worked=%v err=%v", worked, err)
	}
	if finalizer.calls != 1 || len(repository.transitions) != 1 {
		t.Fatalf("finalizer calls=%d transitions=%+v", finalizer.calls, repository.transitions)
	}
}

func TestRunnerFinalizationProbePendingAllowsNormalExecution(t *testing.T) {
	now := time.Now()
	repository := &fakeRunnerRepository{claim: &Claim{
		Command: Command{ID: 461, ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "probe-pending", State: StateClaimed, AttemptCount: 1, MaxAttempts: 3},
		Owner:   "worker-a", FencingToken: 1,
	}, renewal: Renewal{Alive: true}}
	executor := &fakeReplyExecutor{result: &aichat.ConversationReplyResult{ConversationID: 3, AssistantMessageID: 22}}
	finalizer := &fakeReplyFinalizer{err: fmt.Errorf("%w: no terminal evidence", aigateway.ErrFinalizationPending)}
	runner := NewRunner(RunnerOptions{Repository: repository, Executor: executor, Finalizer: finalizer, Owner: "worker-a", LeaseTTL: time.Minute, Now: func() time.Time { return now }})

	worked, err := runner.RunOnce(context.Background())
	if !worked || err != nil || finalizer.calls != 1 || executor.calls != 1 {
		t.Fatalf("worked=%v err=%v finalizer=%d executor=%d", worked, err, finalizer.calls, executor.calls)
	}
}

func TestRunnerFinalizationMarkerFencesExecutorAndSchedulesFinalizationRetry(t *testing.T) {
	now := time.Now()
	repository := &finalizationRetryRunnerRepository{fakeRunnerRepository: fakeRunnerRepository{claim: &Claim{
		Command: Command{ID: 462, ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "finalization-only", State: StateClaimed, AttemptCount: 3, MaxAttempts: 3, LastErrorCode: ErrCodeFinalizationRetry, LastErrorMessage: FinalizationRetryMarker},
		Owner:   "worker-a", FencingToken: 2,
	}, renewal: Renewal{Alive: true}}}
	executor := &fakeReplyExecutor{result: &aichat.ConversationReplyResult{ConversationID: 3, AssistantMessageID: 22}}
	finalizer := &fakeReplyFinalizer{err: errors.New("settlement database unavailable")}
	runner := NewRunner(RunnerOptions{Repository: repository, Executor: executor, Finalizer: finalizer, Owner: "worker-a", LeaseTTL: time.Minute, Now: func() time.Time { return now }})

	worked, err := runner.RunOnce(context.Background())
	if !worked || !errors.Is(err, aichat.ErrPaidFinalizationRetry) {
		t.Fatalf("worked=%v err=%v", worked, err)
	}
	if finalizer.calls != 1 || executor.calls != 0 || repository.finalizationRetryID != 462 {
		t.Fatalf("finalizer=%d executor=%d retry=%d", finalizer.calls, executor.calls, repository.finalizationRetryID)
	}
}

func TestRunnerGenericFinalizationRetryPendingResumesExecutor(t *testing.T) {
	now := time.Now()
	repository := &finalizationRetryRunnerRepository{fakeRunnerRepository: fakeRunnerRepository{claim: &Claim{
		Command: Command{ID: 464, ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "generic-pending", State: StateClaimed, AttemptCount: 1, MaxAttempts: 3, LastErrorCode: ErrCodeFinalizationRetry, LastErrorMessage: FinalizationRetryMarker},
		Owner:   "worker-a", FencingToken: 2,
	}, renewal: Renewal{Alive: true}}}
	executor := &fakeReplyExecutor{result: &aichat.ConversationReplyResult{ConversationID: 3, AssistantMessageID: 22}}
	finalizer := &fakeReplyFinalizer{err: aigateway.ErrFinalizationPending}
	runner := NewRunner(RunnerOptions{Repository: repository, Executor: executor, Finalizer: finalizer, Owner: "worker-a", LeaseTTL: time.Minute, Now: func() time.Time { return now }})

	worked, err := runner.RunOnce(context.Background())
	if !worked || err != nil || executor.calls != 1 || repository.finalizationRetryID != 0 {
		t.Fatalf("worked=%v err=%v executor=%d finalization_retry=%d", worked, err, executor.calls, repository.finalizationRetryID)
	}
}

func TestRunnerCancellationFinalizerFailureSchedulesRetryImmediately(t *testing.T) {
	now := time.Now()
	repository := &finalizationRetryRunnerRepository{fakeRunnerRepository: fakeRunnerRepository{claim: &Claim{
		Command: Command{ID: 463, ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "cancel-finalization", State: StateClaimed, AttemptCount: 1, MaxAttempts: 3},
		Owner:   "worker-a", FencingToken: 2,
	}, renewal: Renewal{Alive: true, CancelRequested: true}}}
	finalizer := &fakeReplyFinalizer{err: errors.New("settlement database unavailable")}
	runner := NewRunner(RunnerOptions{Repository: repository, Executor: &fakeReplyExecutor{}, Finalizer: finalizer, Owner: "worker-a", LeaseTTL: time.Minute, Now: func() time.Time { return now }})

	worked, err := runner.RunOnce(context.Background())
	if !worked || !errors.Is(err, aichat.ErrPaidFinalizationRetry) || finalizer.calls != 1 || repository.finalizationRetryID != 463 {
		t.Fatalf("worked=%v err=%v finalizer=%d retry=%d", worked, err, finalizer.calls, repository.finalizationRetryID)
	}
}

func TestRunnerSchedulesRetryWithDurableRunEvent(t *testing.T) {
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	repository := &retryRunnerRepository{fakeRunnerRepository: fakeRunnerRepository{claim: &Claim{Command: Command{ID: 47, ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "request-retry", State: StateClaimed, AttemptCount: 1, MaxAttempts: 3}, Owner: "worker-a", FencingToken: 2}, renewal: Renewal{Alive: true}}}
	executor := &fakeReplyExecutor{err: apperror.New("ai.provider.failed", apperror.CategoryDependency, 503, apperror.Retryable, "", nil, "provider temporarily unavailable")}
	runner := NewRunner(RunnerOptions{Repository: repository, Executor: executor, Owner: "worker-a", LeaseTTL: time.Minute, Now: func() time.Time { return now }})
	worked, err := runner.RunOnce(context.Background())
	if !worked || err == nil {
		t.Fatalf("worked=%v err=%v", worked, err)
	}
	if repository.retryCommandID != 47 || repository.retryCode != "ai.provider.failed" || repository.retryNext != now.Add(time.Second) {
		t.Fatalf("retry schedule=%+v", repository)
	}
	if len(repository.transitions) != 1 {
		t.Fatalf("retry path fell back to ordinary transition: %+v", repository.transitions)
	}
}

func TestRunnerSchedulesFinalizationRetryWithoutNormalFailureTransition(t *testing.T) {
	now := time.Now()
	repository := &finalizationRetryRunnerRepository{fakeRunnerRepository: fakeRunnerRepository{claim: &Claim{
		Command: Command{ID: 48, ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "finalization-retry", State: StateClaimed, AttemptCount: 1, MaxAttempts: 3},
		Owner:   "worker-a", FencingToken: 2,
	}, renewal: Renewal{Alive: true}}}
	executor := &fakeReplyExecutor{err: fmt.Errorf("%w: transaction unavailable", aichat.ErrPaidFinalizationRetry)}
	runner := NewRunner(RunnerOptions{Repository: repository, Executor: executor, Owner: "worker-a", LeaseTTL: time.Minute, Now: func() time.Time { return now }})

	worked, err := runner.RunOnce(context.Background())
	if !worked || !errors.Is(err, aichat.ErrPaidFinalizationRetry) {
		t.Fatalf("worked=%v err=%v", worked, err)
	}
	if repository.finalizationRetryID != 48 || repository.retryCode != "" || len(repository.transitions) != 1 {
		t.Fatalf("retry id=%d code=%q transitions=%+v", repository.finalizationRetryID, repository.retryCode, repository.transitions)
	}
}

func TestRunnerRechecksDurableCancelAfterAssistantPublicationIsRejected(t *testing.T) {
	now := time.Now()
	repository := &fakeRunnerRepository{claim: &Claim{
		Command: Command{ID: 46, ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "request-6", State: StateClaimed, AttemptCount: 1, MaxAttempts: 3},
		Owner:   "worker-a", FencingToken: 1, LeaseExpiresAt: now.Add(time.Minute),
	}, renewals: []Renewal{{Alive: true}, {Alive: true, CancelRequested: true}}}
	executor := &fakeReplyExecutor{err: aichat.ErrAssistantPublicationRejected}
	runner := NewRunner(RunnerOptions{Repository: repository, Executor: executor, Owner: "worker-a", LeaseTTL: time.Minute, Now: time.Now})

	worked, err := runner.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("RunOnce worked=%v err=%v", worked, err)
	}
	if len(repository.transitions) != 2 || repository.transitions[1].from != StateRunning || repository.transitions[1].to != StateCanceled {
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
	renewed     chan int
	transitions []stateTransition
}

type retryRunnerRepository struct {
	fakeRunnerRepository
	retryCommandID uint64
	retryCode      string
	retryNext      time.Time
}

type finalizationRetryRunnerRepository struct {
	fakeRunnerRepository
	finalizationRetryID uint64
	retryCode           string
}

func (r *finalizationRetryRunnerRepository) ScheduleFinalizationRetry(_ context.Context, commandID uint64, _ string, _ uint64, _ time.Time, _ time.Time) (bool, error) {
	r.finalizationRetryID = commandID
	return true, nil
}

func (r *retryRunnerRepository) ScheduleRetry(_ context.Context, commandID uint64, _ string, _ uint64, _ time.Time, next time.Time, code string, _ string) (bool, error) {
	r.retryCommandID, r.retryNext, r.retryCode = commandID, next, code
	return true, nil
}

func (f *fakeRunnerRepository) ClaimNext(context.Context, string, time.Time, time.Duration) (*Claim, error) {
	return f.claim, nil
}
func (f *fakeRunnerRepository) ClaimByID(context.Context, uint64, string, time.Time, time.Duration) (*Claim, error) {
	return f.claim, nil
}
func (f *fakeRunnerRepository) Renew(context.Context, uint64, string, uint64, time.Time) (Renewal, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.renewIndex++
	call := f.renewIndex
	if f.renewed != nil {
		select {
		case f.renewed <- call:
		default:
		}
	}
	if call <= len(f.renewals) {
		result := f.renewals[call-1]
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
	input                      aichat.ConversationReplyInput
	result                     *aichat.ConversationReplyResult
	err                        error
	blockUntilCanceled         bool
	waitForDeliveryStop        bool
	releaseAfterStop           <-chan struct{}
	calls                      int
	cancelCause                error
	deliveryCause              error
	drainCanceledBeforeRelease bool
}

type fakeReplyFinalizer struct {
	result *aichat.ConversationReplyResult
	err    error
	calls  int
}

func (f *fakeReplyFinalizer) FinalizeConversationReply(context.Context, aichat.ConversationReplyInput) (*aichat.ConversationReplyResult, error) {
	f.calls++
	return f.result, f.err
}

func (f *fakeReplyExecutor) ExecuteConversationReply(ctx context.Context, input aichat.ConversationReplyInput) (*aichat.ConversationReplyResult, error) {
	f.calls++
	f.input = input
	if f.blockUntilCanceled {
		<-ctx.Done()
		f.cancelCause = context.Cause(ctx)
		return nil, ctx.Err()
	}
	if f.waitForDeliveryStop {
		<-input.DeliveryContext.Done()
		f.deliveryCause = context.Cause(input.DeliveryContext)
		select {
		case <-ctx.Done():
			f.drainCanceledBeforeRelease = true
			f.cancelCause = context.Cause(ctx)
			return nil, ctx.Err()
		case <-f.releaseAfterStop:
			return nil, nil
		}
	}
	return f.result, f.err
}
