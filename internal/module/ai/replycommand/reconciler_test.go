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
