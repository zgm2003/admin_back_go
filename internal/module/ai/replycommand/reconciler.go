package replycommand

import (
	"context"
	"errors"
	"time"

	"admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/shared/enum"

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

type OrphanedPreDispatchWork struct {
	CommandID uint64
}

type OrphanedPreDispatchRepository interface {
	ClaimOrphanedPreDispatch(context.Context, ClaimSource, time.Time) (*OrphanedPreDispatchWork, error)
}

type OrphanedPreDispatchFinalizer interface {
	FinalizeOrphanedPreDispatch(context.Context, uint64) error
}

type DeliveryCleanupCandidate struct {
	CommandID         uint64 `gorm:"column:command_id"`
	State             State  `gorm:"column:state"`
	HasStoppedMessage bool   `gorm:"column:has_stopped_message"`
}

type DeliveryCleanupRepository interface {
	DeliveryCleaner
	ListDeliveryCleanupCandidates(context.Context, int) ([]DeliveryCleanupCandidate, error)
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
	if err != nil {
		return false, err
	}
	worked := false
	if work != nil {
		if err := r.finalizer.FinalizeOutcomeUnknown(ctx, work.CommandID); err != nil {
			return true, err
		}
		worked = true
	}
	if repository, ok := r.repository.(OrphanedPreDispatchRepository); ok {
		if finalizer, finalizerOK := r.finalizer.(OrphanedPreDispatchFinalizer); finalizerOK {
			orphan, claimErr := repository.ClaimOrphanedPreDispatch(ctx, ClaimSourceRecovery, r.now())
			if claimErr != nil {
				return worked, claimErr
			}
			if orphan != nil {
				if err := finalizer.FinalizeOrphanedPreDispatch(ctx, orphan.CommandID); err != nil {
					return true, err
				}
				worked = true
			}
		}
	}

	cleanupRepository, ok := r.repository.(DeliveryCleanupRepository)
	if !ok {
		return worked, nil
	}
	candidates, err := cleanupRepository.ListDeliveryCleanupCandidates(ctx, 32)
	if err != nil {
		return worked, err
	}
	for _, candidate := range candidates {
		if candidate.CommandID == 0 || (!terminalDeliveryState(candidate.State) && !candidate.HasStoppedMessage) {
			continue
		}
		if err := CleanupDeliveryChunks(ctx, cleanupRepository, candidate.CommandID, 4); err != nil {
			return worked, err
		}
		worked = true
	}
	return worked, nil
}

func terminalDeliveryState(state State) bool {
	switch state {
	case StateSucceeded, StateFailed, StateCanceled, StateOutcomeUnknown, StateTimedOut:
		return true
	default:
		return false
	}
}

func (r *GormRepository) ListDeliveryCleanupCandidates(ctx context.Context, limit int) ([]DeliveryCleanupCandidate, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 || limit > 64 {
		limit = 32
	}
	terminalStates := []State{StateSucceeded, StateFailed, StateCanceled, StateOutcomeUnknown, StateTimedOut}
	var candidates []DeliveryCleanupCandidate
	err := r.db.WithContext(ctx).Table("ai_reply_delivery_chunks AS chunks").
		Select("DISTINCT chunks.command_id, commands.state, (stopped.id IS NOT NULL) AS has_stopped_message").
		Joins("JOIN ai_reply_commands AS commands ON commands.id = chunks.command_id").
		Joins("LEFT JOIN ai_messages AS stopped ON stopped.reply_command_id = commands.id AND stopped.delivery_state = ? AND stopped.is_del = ?", DeliveryStateStopped, enum.CommonNo).
		Where("commands.state IN ? OR stopped.id IS NOT NULL", terminalStates).
		Order("chunks.command_id ASC").
		Limit(limit).
		Scan(&candidates).Error
	return candidates, err
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

func (r *GormRepository) ClaimOrphanedPreDispatch(ctx context.Context, source ClaimSource, now time.Time) (*OrphanedPreDispatchWork, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if source != ClaimSourceRecovery || now.IsZero() {
		return nil, ErrCreateInputInvalid
	}
	var work *OrphanedPreDispatchWork
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var command Command
		err := tx.Table("ai_reply_commands AS commands").
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Select("commands.*").
			Joins("JOIN ai_runs AS runs ON runs.user_id = commands.user_id AND runs.request_id = commands.request_id").
			Joins("JOIN ai_usage_charges AS charges ON charges.run_id = runs.id").
			Where("commands.state = ? AND commands.finished_at IS NOT NULL AND commands.assistant_message_id IS NULL", StateFailed).
			Where("runs.status = ? AND runs.billing_status IN ?", enum.AIRunStatusRunning, []billing.BillingStatus{billing.BillingStatusPending, billing.BillingStatusHeld}).
			Where("charges.status = ? AND charges.finalized_at IS NULL", billing.ChargeStatusOpen).
			Where("NOT EXISTS (SELECT 1 FROM ai_provider_attempts AS attempts WHERE attempts.command_id = commands.id)").
			Order("commands.finished_at ASC, commands.id ASC").
			First(&command).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		result := tx.Model(&Command{}).Where("id = ? AND state = ?", command.ID, StateFailed).
			Updates(map[string]any{"claimed_at": now, "claim_source": source, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			work = &OrphanedPreDispatchWork{CommandID: command.ID}
		}
		return nil
	})
	return work, err
}
