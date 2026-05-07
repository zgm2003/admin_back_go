package exporttask

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/platform/taskqueue"
)

type fakeJobService struct {
	input RunInput
	err   error
}

func (f *fakeJobService) Run(ctx context.Context, input RunInput) error {
	f.input = input
	return f.err
}

func TestNewRunTaskUsesVersionedTypeLowQueueAndLeanPayload(t *testing.T) {
	task, err := NewRunTask(RunPayload{TaskID: 7, Kind: KindUserList, UserID: 9, Platform: "admin", IDs: []int64{3, 2}})
	if err != nil {
		t.Fatalf("NewRunTask returned error: %v", err)
	}
	if task.Type != TypeRunV1 || task.Queue != taskqueue.QueueLow || task.MaxRetry != 3 || task.Timeout != 5*time.Minute {
		t.Fatalf("unexpected task metadata: %#v", task)
	}
	payload, err := DecodeRunPayload(task.Payload)
	if err != nil {
		t.Fatalf("DecodeRunPayload returned error: %v", err)
	}
	if payload.TaskID != 7 || payload.Kind != KindUserList || payload.UserID != 9 || len(payload.IDs) != 2 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestDecodeRunPayloadRejectsMissingRequiredFields(t *testing.T) {
	cases := [][]byte{
		[]byte(`{}`),
		[]byte(`{"task_id":7,"kind":"user_list","user_id":9,"ids":[]}`),
		[]byte(`{"task_id":7,"kind":"","user_id":9,"ids":[1]}`),
		[]byte(`{"task_id":7,"kind":"user_list","user_id":0,"ids":[1]}`),
	}
	for _, payload := range cases {
		if _, err := DecodeRunPayload(payload); err == nil {
			t.Fatalf("expected DecodeRunPayload to reject %s", string(payload))
		}
	}
}

func TestRegisterHandlersProcessesRunTaskThroughMux(t *testing.T) {
	service := &fakeJobService{}
	mux := taskqueue.NewMux()
	RegisterHandlers(mux, service, nil)
	task, err := NewRunTask(RunPayload{TaskID: 7, Kind: KindUserList, UserID: 9, Platform: "admin", IDs: []int64{3}})
	if err != nil {
		t.Fatalf("NewRunTask returned error: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessProjectTask returned error: %v", err)
	}
	if service.input.TaskID != 7 || service.input.Kind != KindUserList || service.input.UserID != 9 || len(service.input.IDs) != 1 {
		t.Fatalf("unexpected service input: %#v", service.input)
	}
}
