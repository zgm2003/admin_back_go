package replycommand

import (
	"context"
	"testing"
	"time"
)

type fakeOutcomeRepository struct {
	work   *OutcomeUnknownWork
	source ClaimSource
	now    time.Time
}

type fakeCleanupRepository struct {
	fakeOutcomeRepository
	candidates []DeliveryCleanupCandidate
	deleted    []uint64
	limit      int
}

func (f *fakeCleanupRepository) ListDeliveryCleanupCandidates(_ context.Context, limit int) ([]DeliveryCleanupCandidate, error) {
	f.limit = limit
	return append([]DeliveryCleanupCandidate(nil), f.candidates...), nil
}

func (f *fakeCleanupRepository) DeleteDeliveryChunks(_ context.Context, commandID uint64, _ int) (int64, error) {
	f.deleted = append(f.deleted, commandID)
	return 1, nil
}

func (f *fakeOutcomeRepository) ClaimOutcomeUnknown(_ context.Context, source ClaimSource, now time.Time) (*OutcomeUnknownWork, error) {
	f.source, f.now = source, now
	return f.work, nil
}

type fakeOutcomeFinalizer struct {
	commandID uint64
	err       error
}

func (f *fakeOutcomeFinalizer) FinalizeOutcomeUnknown(_ context.Context, commandID uint64) error {
	f.commandID = commandID
	return f.err
}

func TestReconcilerFinalizesOutcomeUnknownViaFinalizer(t *testing.T) {
	repository := &fakeOutcomeRepository{work: &OutcomeUnknownWork{CommandID: 44}}
	finalizer := &fakeOutcomeFinalizer{}
	reconciler := NewReconciler(ReconcilerOptions{Repository: repository, Finalizer: finalizer})

	worked, err := reconciler.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("worked=%v err=%v", worked, err)
	}
	if finalizer.commandID != 44 {
		t.Fatalf("finalizer command=%d", finalizer.commandID)
	}
}

func TestOutcomeReconcilerMarksRecoverySource(t *testing.T) {
	now := time.Date(2026, 7, 28, 10, 0, 2, 0, time.UTC)
	repository := &fakeOutcomeRepository{work: &OutcomeUnknownWork{CommandID: 45}}
	reconciler := NewReconciler(ReconcilerOptions{
		Repository: repository,
		Finalizer:  &fakeOutcomeFinalizer{},
		Now:        func() time.Time { return now },
	})

	worked, err := reconciler.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("worked=%v err=%v", worked, err)
	}
	if repository.source != ClaimSourceRecovery || !repository.now.Equal(now) {
		t.Fatalf("source=%q now=%v", repository.source, repository.now)
	}
}

func TestReconcilerRequiresFinalizer(t *testing.T) {
	reconciler := NewReconciler(ReconcilerOptions{Repository: &fakeOutcomeRepository{}})
	if worked, err := reconciler.RunOnce(context.Background()); worked || err != ErrReconcilerNotReady {
		t.Fatalf("worked=%v err=%v", worked, err)
	}
}

func TestReconcilerCleansDeliveryChunksOnlyForTerminalOrStoppedCommands(t *testing.T) {
	repository := &fakeCleanupRepository{candidates: []DeliveryCleanupCandidate{
		{CommandID: 41, State: StateRunning},
		{CommandID: 42, State: StateRunning, HasStoppedMessage: true},
		{CommandID: 43, State: StateSucceeded},
	}}
	reconciler := NewReconciler(ReconcilerOptions{Repository: repository, Finalizer: &fakeOutcomeFinalizer{}})

	worked, err := reconciler.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("worked=%v err=%v", worked, err)
	}
	if repository.limit <= 0 || len(repository.deleted) != 2 || repository.deleted[0] != 42 || repository.deleted[1] != 43 {
		t.Fatalf("limit=%d deleted=%v", repository.limit, repository.deleted)
	}
}
