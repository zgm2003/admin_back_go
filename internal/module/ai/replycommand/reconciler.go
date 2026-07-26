package replycommand

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrReconcilerNotReady = errors.New("reply command reconciler is not ready")

type OutcomeUnknownWork struct {
	CommandID uint64
}

type OutcomeRepository interface {
	NextOutcomeUnknown(context.Context) (*OutcomeUnknownWork, error)
}

// OutcomeFinalizer closes an acknowledged-but-unresolved paid attempt through
// the same Run/Charge/wallet/Hold settlement transaction as live execution.
type OutcomeFinalizer interface {
	FinalizeOutcomeUnknown(context.Context, uint64) error
}

type ReconcilerOptions struct {
	Repository OutcomeRepository
	Finalizer  OutcomeFinalizer
	Now        func() time.Time
}

type Reconciler struct {
	repository OutcomeRepository
	finalizer  OutcomeFinalizer
	now        func() time.Time
}

func NewReconciler(options ReconcilerOptions) *Reconciler {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Reconciler{repository: options.Repository, finalizer: options.Finalizer, now: now}
}

func (r *Reconciler) RunOnce(ctx context.Context) (bool, error) {
	if r == nil || r.repository == nil || r.finalizer == nil || r.now == nil {
		return false, ErrReconcilerNotReady
	}
	if ctx == nil {
		ctx = context.Background()
	}
	work, err := r.repository.NextOutcomeUnknown(ctx)
	if err != nil || work == nil {
		return false, err
	}
	return true, r.finalizer.FinalizeOutcomeUnknown(ctx, work.CommandID)
}

func (r *GormRepository) NextOutcomeUnknown(ctx context.Context) (*OutcomeUnknownWork, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var command Command
	err := r.db.WithContext(ctx).
		Select("ai_reply_commands.*").
		Joins("JOIN ai_runs ON ai_runs.user_id = ai_reply_commands.user_id AND ai_runs.request_id = ai_reply_commands.request_id").
		Where("ai_reply_commands.state = ? AND ai_runs.billing_status IN ?", StateOutcomeUnknown, []string{"pending", "held"}).
		Order("ai_reply_commands.outcome_unknown_at ASC, ai_reply_commands.id ASC").
		First(&command).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &OutcomeUnknownWork{CommandID: command.ID}, nil
}
