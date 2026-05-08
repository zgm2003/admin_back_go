package payment

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"admin_back_go/internal/platform/taskqueue"
)

func TestPaymentTaskTypes(t *testing.T) {
	closeTask, err := NewCloseExpiredOrderTask(CloseExpiredPayload{Limit: 50})
	if err != nil {
		t.Fatalf("NewCloseExpiredOrderTask: %v", err)
	}
	if closeTask.Type != TypeCloseExpiredOrderV1 {
		t.Fatalf("unexpected close task type %s", closeTask.Type)
	}
	if closeTask.Queue != taskqueue.QueueDefault || closeTask.UniqueTTL <= 0 {
		t.Fatalf("unexpected close task queue/ttl: %#v", closeTask)
	}
	var closePayload CloseExpiredPayload
	if err := json.Unmarshal(closeTask.Payload, &closePayload); err != nil {
		t.Fatalf("decode close payload: %v", err)
	}
	if closePayload.Limit != 50 {
		t.Fatalf("expected close limit 50, got %d", closePayload.Limit)
	}

	syncTask, err := NewSyncPendingOrderTask(SyncPendingPayload{Limit: 100})
	if err != nil {
		t.Fatalf("NewSyncPendingOrderTask: %v", err)
	}
	if syncTask.Type != TypeSyncPendingOrderV1 {
		t.Fatalf("unexpected sync task type %s", syncTask.Type)
	}
	if syncTask.Queue != taskqueue.QueueDefault || syncTask.UniqueTTL <= 0 {
		t.Fatalf("unexpected sync task queue/ttl: %#v", syncTask)
	}
	var syncPayload SyncPendingPayload
	if err := json.Unmarshal(syncTask.Payload, &syncPayload); err != nil {
		t.Fatalf("decode sync payload: %v", err)
	}
	if syncPayload.Limit != 100 {
		t.Fatalf("expected sync limit 100, got %d", syncPayload.Limit)
	}
}

func TestRegisterHandlersProcessesPaymentCronTasks(t *testing.T) {
	service := &fakePaymentJobService{}
	mux := taskqueue.NewMux()
	RegisterHandlers(mux, service, slog.Default())

	closeTask, err := NewCloseExpiredOrderTask(CloseExpiredPayload{Limit: 11})
	if err != nil {
		t.Fatalf("NewCloseExpiredOrderTask: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), closeTask); err != nil {
		t.Fatalf("ProcessProjectTask close expired: %v", err)
	}
	if service.closeLimit != 11 {
		t.Fatalf("expected close limit 11, got %d", service.closeLimit)
	}

	syncTask, err := NewSyncPendingOrderTask(SyncPendingPayload{Limit: 12})
	if err != nil {
		t.Fatalf("NewSyncPendingOrderTask: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), syncTask); err != nil {
		t.Fatalf("ProcessProjectTask sync pending: %v", err)
	}
	if service.syncLimit != 12 {
		t.Fatalf("expected sync limit 12, got %d", service.syncLimit)
	}
}

type fakePaymentJobService struct {
	closeLimit int
	syncLimit  int
}

func (f *fakePaymentJobService) CloseExpiredOrders(ctx context.Context, input CloseExpiredInput) (*JobResult, error) {
	f.closeLimit = input.Limit
	return &JobResult{Scanned: 1, Closed: 1}, nil
}

func (f *fakePaymentJobService) SyncPendingOrders(ctx context.Context, input SyncPendingInput) (*JobResult, error) {
	f.syncLimit = input.Limit
	return &JobResult{Scanned: 1, Paid: 1}, nil
}
