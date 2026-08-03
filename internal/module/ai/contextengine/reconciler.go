package contextengine

import (
	"context"
	"errors"
	"time"
)

type DocumentIndexReconciler struct {
	repository  IngestionRepository
	enqueuer    *QueueDocumentVersionEnqueuer
	batchSize   int
	maxAttempts uint32
}

func NewDocumentIndexReconciler(repository IngestionRepository, enqueuer *QueueDocumentVersionEnqueuer, batchSize int, maxAttempts uint32) *DocumentIndexReconciler {
	return &DocumentIndexReconciler{repository: repository, enqueuer: enqueuer, batchSize: batchSize, maxAttempts: maxAttempts}
}

func (reconciler *DocumentIndexReconciler) Reconcile(ctx context.Context, now time.Time) error {
	_, err := reconciler.reconcile(ctx, now)
	return err
}

func (reconciler *DocumentIndexReconciler) RunOnce(ctx context.Context) (bool, error) {
	return reconciler.reconcile(ctx, time.Now().UTC())
}

func (reconciler *DocumentIndexReconciler) reconcile(ctx context.Context, now time.Time) (bool, error) {
	if reconciler == nil || reconciler.repository == nil || reconciler.enqueuer == nil || reconciler.enqueuer.queue == nil || reconciler.batchSize <= 0 || reconciler.maxAttempts == 0 {
		return false, errors.New("document index reconciler is not configured")
	}
	candidates, err := reconciler.repository.ListReconcileCandidates(ctx, now, reconciler.batchSize)
	if err != nil {
		return false, err
	}
	for _, candidate := range candidates {
		if candidate.State == DocumentVersionProcessing && candidate.AttemptCount >= reconciler.maxAttempts {
			attempt := DocumentIndexAttempt{VersionID: candidate.VersionID, LeaseToken: candidate.LeaseToken, AttemptCount: candidate.AttemptCount}
			if err := reconciler.repository.FinalizeVersion(ctx, attempt, "ai.context.index_retry_exhausted", true, now); err != nil && !errors.Is(err, ErrVersionLeaseLost) {
				return false, err
			}
			continue
		}
		if err := reconciler.enqueuer.EnqueueDocumentVersion(ctx, candidate.VersionID); err != nil {
			return false, err
		}
	}
	return len(candidates) > 0, nil
}
