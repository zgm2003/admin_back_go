package aiimage

import (
	"context"
	"errors"
)

var ErrReconcilerNotConfigured = errors.New("aiimage reconciler not configured")

type PendingStore interface {
	FindPendingImages(context.Context, int) ([]ImageTask, error)
}

type TaskWaker interface {
	WakeImageTask(context.Context, ImageTask) error
}

type Reconciler struct {
	store PendingStore
	waker TaskWaker
	limit int
}

func NewReconciler(store PendingStore, waker TaskWaker, limit int) *Reconciler {
	if limit <= 0 {
		limit = 25
	}
	return &Reconciler{store: store, waker: waker, limit: limit}
}

func (reconciler *Reconciler) RunOnce(ctx context.Context) (bool, error) {
	if reconciler == nil || reconciler.store == nil || reconciler.waker == nil {
		return false, ErrReconcilerNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tasks, err := reconciler.store.FindPendingImages(ctx, reconciler.limit)
	if err != nil || len(tasks) == 0 {
		return false, err
	}
	for _, task := range tasks {
		if task.ID == 0 || task.RunID <= 0 || task.UserID == 0 || (task.Status != StatusPending && task.Status != StatusRunning) {
			continue
		}
		if err := reconciler.waker.WakeImageTask(ctx, task); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}
