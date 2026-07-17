package replycommand

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/shared/apperror"
)

var (
	ErrLeaseLost      = errors.New("reply command lease lost")
	ErrRunnerNotReady = errors.New("reply command runner is not ready")
	ErrResultInvalid  = errors.New("reply command result is invalid")
)

const defaultLeaseTTL = 30 * time.Second

type RunnerRepository interface {
	ClaimNext(context.Context, string, time.Time, time.Duration) (*Claim, error)
	ClaimByID(context.Context, uint64, string, time.Time, time.Duration) (*Claim, error)
	Renew(context.Context, uint64, string, uint64, time.Time) (Renewal, error)
	Transition(context.Context, uint64, string, uint64, State, State, map[string]any) (bool, error)
}

type ReplyExecutor interface {
	ExecuteConversationReply(context.Context, aichat.ConversationReplyInput) (*aichat.ConversationReplyResult, error)
}

type RunnerOptions struct {
	Repository       RunnerRepository
	Executor         ReplyExecutor
	CancelSubscriber CancelSubscriber
	Owner            string
	LeaseTTL         time.Duration
	Now              func() time.Time
	Logger           *slog.Logger
}

type Runner struct {
	repository       RunnerRepository
	executor         ReplyExecutor
	cancelSubscriber CancelSubscriber
	owner            string
	leaseTTL         time.Duration
	now              func() time.Time
	logger           *slog.Logger
}

func NewRunner(options RunnerOptions) *Runner {
	owner := strings.TrimSpace(options.Owner)
	if owner == "" {
		owner = newRunnerOwner()
	}
	leaseTTL := options.LeaseTTL
	if leaseTTL <= 0 {
		leaseTTL = defaultLeaseTTL
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{repository: options.Repository, executor: options.Executor, cancelSubscriber: options.CancelSubscriber, owner: owner, leaseTTL: leaseTTL, now: now, logger: logger}
}

func (r *Runner) RunOnce(ctx context.Context) (bool, error) {
	if err := r.ready(); err != nil {
		return false, err
	}
	claim, err := r.repository.ClaimNext(ctx, r.owner, r.now(), r.leaseTTL)
	if err != nil || claim == nil {
		return false, err
	}
	return true, r.runClaim(ctx, claim)
}

func (r *Runner) RunCommand(ctx context.Context, commandID uint64) (bool, error) {
	if err := r.ready(); err != nil {
		return false, err
	}
	claim, err := r.repository.ClaimByID(ctx, commandID, r.owner, r.now(), r.leaseTTL)
	if err != nil || claim == nil {
		return false, err
	}
	return true, r.runClaim(ctx, claim)
}

func (r *Runner) runClaim(ctx context.Context, claim *Claim) error {
	if ctx == nil {
		ctx = context.Background()
	}
	command := claim.Command
	ok, err := r.repository.Transition(ctx, command.ID, claim.Owner, claim.FencingToken, StateClaimed, StateRunning, map[string]any{
		"started_at": r.now(),
	})
	if err != nil {
		return err
	}
	if !ok {
		return ErrLeaseLost
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var cancelSignal <-chan struct{}
	if r.cancelSubscriber != nil {
		subscription, subscribeErr := r.cancelSubscriber.SubscribeCancel(runCtx, command.ID)
		if subscribeErr != nil {
			r.logger.WarnContext(ctx, "reply cancel subscription unavailable", "command_id", command.ID, "error", subscribeErr)
		} else if subscription != nil {
			cancelSignal = subscription.Signal()
			defer func() {
				if err := subscription.Close(); err != nil {
					r.logger.WarnContext(context.WithoutCancel(ctx), "close reply cancel subscription", "command_id", command.ID, "error", err)
				}
			}()
		}
	}
	initialRenewal, err := r.repository.Renew(context.WithoutCancel(runCtx), command.ID, claim.Owner, claim.FencingToken, r.now().Add(r.leaseTTL))
	if err != nil {
		return err
	}
	if !initialRenewal.Alive {
		return ErrLeaseLost
	}
	if initialRenewal.CancelRequested {
		return r.finishCancellation(context.WithoutCancel(ctx), claim)
	}
	stopRenew := make(chan struct{})
	renewDone := make(chan struct{})
	var leaseLost atomic.Bool
	var cancelRequested atomic.Bool
	var renewErr atomic.Value
	go func() {
		defer close(renewDone)
		interval := r.leaseTTL / 3
		if interval < time.Millisecond {
			interval = time.Millisecond
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		renew := func() bool {
			renewal, err := r.repository.Renew(context.WithoutCancel(runCtx), command.ID, claim.Owner, claim.FencingToken, r.now().Add(r.leaseTTL))
			if err != nil {
				renewErr.Store(err)
			}
			if err != nil || !renewal.Alive {
				leaseLost.Store(true)
				cancel()
				return false
			}
			if renewal.CancelRequested {
				cancelRequested.Store(true)
				cancel()
				return false
			}
			return true
		}
		for {
			select {
			case <-stopRenew:
				return
			case <-runCtx.Done():
				return
			case <-cancelSignal:
				if !renew() {
					return
				}
			case <-ticker.C:
				if !renew() {
					return
				}
			}
		}
	}()

	result, executeErr := r.executor.ExecuteConversationReply(runCtx, aichat.ConversationReplyInput{
		CommandID:      command.ID,
		LeaseOwner:     claim.Owner,
		LeaseToken:     claim.FencingToken,
		ConversationID: command.ConversationID,
		UserID:         command.UserID,
		UserMessageID:  command.UserMessageID,
		RequestID:      command.RequestID,
	})
	close(stopRenew)
	<-renewDone
	if leaseLost.Load() {
		if value := renewErr.Load(); value != nil {
			return errors.Join(ErrLeaseLost, value.(error))
		}
		return ErrLeaseLost
	}
	if cancelRequested.Load() {
		return r.finishCancellation(context.WithoutCancel(ctx), claim)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if executeErr != nil {
		return r.finishFailure(ctx, claim, executeErr)
	}
	if result == nil || result.AssistantMessageID <= 0 {
		return r.finishFailure(ctx, claim, ErrResultInvalid)
	}
	return nil
}

func (r *Runner) finishCancellation(ctx context.Context, claim *Claim) error {
	ok, err := r.repository.Transition(ctx, claim.Command.ID, claim.Owner, claim.FencingToken, StateRunning, StateCanceled, map[string]any{
		"finished_at": r.now(),
	})
	if err != nil {
		return err
	}
	if !ok {
		return ErrLeaseLost
	}
	return nil
}

func (r *Runner) finishFailure(ctx context.Context, claim *Claim, cause error) error {
	command := claim.Command
	code := "ai.reply_failed"
	message := "AI reply execution failed"
	retryable := true
	var appErr *apperror.Error
	if errors.As(cause, &appErr) {
		code = appErr.Code
		message = appErr.Message
		retryable = appErr.Retryable()
	}
	to := StatePending
	values := map[string]any{
		"last_error_code":    code,
		"last_error_message": message,
	}
	if !retryable || command.AttemptCount >= command.MaxAttempts {
		to = StateFailed
		values["finished_at"] = r.now()
	} else {
		values["next_attempt_at"] = r.now().Add(retryBackoff(command.AttemptCount))
	}
	ok, err := r.repository.Transition(ctx, command.ID, claim.Owner, claim.FencingToken, StateRunning, to, values)
	if err != nil {
		return errors.Join(cause, err)
	}
	if !ok {
		return errors.Join(cause, ErrLeaseLost)
	}
	return cause
}

func (r *Runner) ready() error {
	if r == nil || r.repository == nil || r.executor == nil || strings.TrimSpace(r.owner) == "" || r.leaseTTL <= 0 || r.now == nil {
		return ErrRunnerNotReady
	}
	return nil
}

func retryBackoff(attempt uint) time.Duration {
	if attempt == 0 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<(attempt-1)) * time.Second
}

func newRunnerOwner() string {
	var random [8]byte
	if _, err := rand.Read(random[:]); err == nil {
		return "worker-" + hex.EncodeToString(random[:])
	}
	return fmt.Sprintf("worker-%d", time.Now().UnixNano())
}
