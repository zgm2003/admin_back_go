package payruntime

import (
	"context"
	"testing"

	"admin_back_go/internal/platform/taskqueue"
)

func TestNewCloseExpiredOrderTaskEncodesLimitAndUniqueTTL(t *testing.T) {
	task, err := NewCloseExpiredOrderTask(CloseExpiredOrderPayload{Limit: 25})
	if err != nil {
		t.Fatalf("NewCloseExpiredOrderTask returned error: %v", err)
	}
	if task.Type != TypeCloseExpiredOrderV1 {
		t.Fatalf("unexpected task type: %s", task.Type)
	}
	if task.Queue != taskqueue.QueueDefault {
		t.Fatalf("unexpected queue: %s", task.Queue)
	}
	if task.UniqueTTL <= 0 {
		t.Fatalf("expected unique ttl")
	}
	payload, err := DecodeCloseExpiredOrderPayload(task.Payload)
	if err != nil {
		t.Fatalf("DecodeCloseExpiredOrderPayload returned error: %v", err)
	}
	if payload.Limit != 25 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestRegisterHandlersWiresPayRuntimeCronTasks(t *testing.T) {
	mux := taskqueue.NewMux()
	service := &fakeJobService{}
	RegisterHandlers(mux, service, nil)

	closeTask, err := NewCloseExpiredOrderTask(CloseExpiredOrderPayload{Limit: 3})
	if err != nil {
		t.Fatalf("build close expired task: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), closeTask); err != nil {
		t.Fatalf("close expired handler returned error: %v", err)
	}
	syncTask, err := NewSyncPendingTransactionTask(SyncPendingTransactionPayload{Limit: 4})
	if err != nil {
		t.Fatalf("build sync pending task: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), syncTask); err != nil {
		t.Fatalf("sync pending handler returned error: %v", err)
	}
	if service.closeLimit != 3 || service.syncLimit != 4 {
		t.Fatalf("unexpected service calls: %#v", service)
	}
}

type fakeJobService struct {
	closeLimit int
	syncLimit  int
}

func (s *fakeJobService) CloseExpiredOrders(ctx context.Context, input CloseExpiredOrderInput) (*CloseExpiredOrderResult, error) {
	s.closeLimit = input.Limit
	return &CloseExpiredOrderResult{}, nil
}

func (s *fakeJobService) SyncPendingTransactions(ctx context.Context, input SyncPendingTransactionInput) (*SyncPendingTransactionResult, error) {
	s.syncLimit = input.Limit
	return &SyncPendingTransactionResult{}, nil
}
