package notificationtask

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"admin_back_go/internal/platform/taskqueue"
)

func TestTaskBuildersUseVersionedTypesAndDefaultQueue(t *testing.T) {
	dispatchTask, err := NewDispatchDueTask(DispatchDuePayload{Limit: 25})
	if err != nil {
		t.Fatalf("NewDispatchDueTask returned error: %v", err)
	}
	if dispatchTask.Type != TypeDispatchDueV1 || dispatchTask.Queue != taskqueue.QueueDefault || dispatchTask.UniqueTTL <= 0 {
		t.Fatalf("unexpected dispatch task: %#v", dispatchTask)
	}
	var dispatchPayload DispatchDuePayload
	if err := json.Unmarshal(dispatchTask.Payload, &dispatchPayload); err != nil {
		t.Fatalf("decode dispatch payload: %v", err)
	}
	if dispatchPayload.Limit != 25 {
		t.Fatalf("unexpected dispatch payload: %#v", dispatchPayload)
	}

	sendTask, err := NewSendTask(99)
	if err != nil {
		t.Fatalf("NewSendTask returned error: %v", err)
	}
	if sendTask.Type != TypeSendTaskV1 || sendTask.Queue != taskqueue.QueueDefault {
		t.Fatalf("unexpected send task: %#v", sendTask)
	}
	payload, err := DecodeSendTaskPayload(sendTask.Payload)
	if err != nil {
		t.Fatalf("DecodeSendTaskPayload returned error: %v", err)
	}
	if payload.TaskID != 99 {
		t.Fatalf("unexpected send payload: %#v", payload)
	}
}

func TestRegisterHandlersProcessesDispatchAndSendTasks(t *testing.T) {
	service := &fakeJobService{}
	mux := taskqueue.NewMux()
	RegisterHandlers(mux, service, slog.Default())

	dispatchTask, err := NewDispatchDueTask(DispatchDuePayload{Limit: 5})
	if err != nil {
		t.Fatalf("NewDispatchDueTask returned error: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), dispatchTask); err != nil {
		t.Fatalf("process dispatch task: %v", err)
	}
	if service.dispatchLimit != 5 {
		t.Fatalf("expected dispatch limit 5, got %d", service.dispatchLimit)
	}

	sendTask, err := NewSendTask(8)
	if err != nil {
		t.Fatalf("NewSendTask returned error: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), sendTask); err != nil {
		t.Fatalf("process send task: %v", err)
	}
	if service.sendTaskID != 8 {
		t.Fatalf("expected send task id 8, got %d", service.sendTaskID)
	}
}

type fakeJobService struct {
	dispatchLimit int
	sendTaskID    int64
}

func (f *fakeJobService) DispatchDue(ctx context.Context, input DispatchDueInput) (*DispatchDueResult, error) {
	f.dispatchLimit = input.Limit
	return &DispatchDueResult{Claimed: 1, Queued: 1}, nil
}

func (f *fakeJobService) SendTask(ctx context.Context, input SendTaskInput) (*SendTaskResult, error) {
	f.sendTaskID = input.TaskID
	return &SendTaskResult{TaskID: input.TaskID, Sent: 1}, nil
}
