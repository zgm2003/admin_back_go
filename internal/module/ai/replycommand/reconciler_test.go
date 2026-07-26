package replycommand

import (
	"context"
	"testing"
)

type fakeOutcomeRepository struct{ work *OutcomeUnknownWork }

func (f *fakeOutcomeRepository) NextOutcomeUnknown(context.Context) (*OutcomeUnknownWork, error) {
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

func TestReconcilerRequiresFinalizer(t *testing.T) {
	reconciler := NewReconciler(ReconcilerOptions{Repository: &fakeOutcomeRepository{}})
	if worked, err := reconciler.RunOnce(context.Background()); worked || err != ErrReconcilerNotReady {
		t.Fatalf("worked=%v err=%v", worked, err)
	}
}
