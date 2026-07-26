package aitext

import (
	"context"
	"errors"
)

var ErrReconcilerNotConfigured = errors.New("aitext reconciler not configured")

type PendingStore interface {
	FindPending(context.Context, int) ([]TextTask, error)
}

type Reconciler struct {
	store PendingStore
	waker Waker
	limit int
}

func NewReconciler(store PendingStore, waker Waker, limit int) *Reconciler {
	if limit <= 0 {
		limit = 25
	}
	return &Reconciler{store: store, waker: waker, limit: limit}
}

func (r *Reconciler) RunOnce(ctx context.Context) (bool, error) {
	if r == nil || r.store == nil || r.waker == nil {
		return false, ErrReconcilerNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tasks, err := r.store.FindPending(ctx, r.limit)
	if err != nil || len(tasks) == 0 {
		return false, err
	}
	for _, task := range tasks {
		if task.ID == 0 || task.RunID <= 0 || task.Status != StatusRunning {
			continue
		}
		if err := r.waker.WakeTextTask(ctx, task.ID); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}
