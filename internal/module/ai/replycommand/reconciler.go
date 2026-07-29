package replycommand

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrReconcilerNotReady = errors.New("reply command reconciler is not ready")

type OutcomeUnknownWork struct {
	CommandID uint64
}

type OutcomeRepository interface {
	ClaimOutcomeUnknown(context.Context, ClaimSource, time.Time) (*OutcomeUnknownWork, error)
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
	work, err := r.repository.ClaimOutcomeUnknown(ctx, ClaimSourceRecovery, r.now())
	if err != nil || work == nil {
		return false, err
	}
	return true, r.finalizer.FinalizeOutcomeUnknown(ctx, work.CommandID)
}

func (r *GormRepository) ClaimOutcomeUnknown(ctx context.Context, source ClaimSource, now time.Time) (*OutcomeUnknownWork, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if source != ClaimSourceRecovery || now.IsZero() {
		return nil, ErrCreateInputInvalid
	}
	var work *OutcomeUnknownWork
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var command Command
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Select("ai_reply_commands.*").
			Joins("JOIN ai_runs ON ai_runs.user_id = ai_reply_commands.user_id AND ai_runs.request_id = ai_reply_commands.request_id").
			Where("ai_reply_commands.state = ? AND ai_runs.billing_status IN ?", StateOutcomeUnknown, []string{"pending", "held"}).
			Order("ai_reply_commands.outcome_unknown_at ASC, ai_reply_commands.id ASC").
			First(&command).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		result := tx.Model(&Command{}).Where("id = ? AND state = ?", command.ID, StateOutcomeUnknown).
			Updates(map[string]any{"claimed_at": now, "claim_source": source, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			work = &OutcomeUnknownWork{CommandID: command.ID}
		}
		return nil
	})
	return work, err
}
