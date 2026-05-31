package exporttask

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/infra/taskqueue"
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
	task, err := NewRunTask(RunPayload{TaskID: 7, Kind: " user_list ", UserID: 9, Platform: " admin ", Scope: ScopeSelected, IDs: []int64{3, 2, 3, 0}})
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
	if payload.TaskID != 7 || payload.Kind != KindUserList || payload.UserID != 9 || payload.Platform != "admin" || payload.Scope != ScopeSelected || !reflect.DeepEqual(payload.IDs, []int64{2, 3}) {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	jsonPayload := string(task.Payload)
	if strings.Contains(jsonPayload, "rows") {
		t.Fatalf("payload must not contain rows: %s", jsonPayload)
	}
}

func TestDecodeRunPayloadRejectsMissingRequiredFields(t *testing.T) {
	cases := [][]byte{
		[]byte(`{}`),
		[]byte(`{"task_id":7,"kind":"user_list","user_id":9,"scope":"selected","ids":[]}`),
		[]byte(`{"task_id":7,"kind":"","user_id":9,"scope":"selected","ids":[1]}`),
		[]byte(`{"task_id":7,"kind":"user_list","user_id":0,"scope":"selected","ids":[1]}`),
		[]byte(`{"task_id":7,"kind":"user_list","user_id":9,"scope":"all","ids":[1]}`),
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
	task, err := NewRunTask(RunPayload{TaskID: 7, Kind: KindUserList, UserID: 9, Platform: "admin", Scope: ScopeSelected, IDs: []int64{3}})
	if err != nil {
		t.Fatalf("NewRunTask returned error: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessProjectTask returned error: %v", err)
	}
	if service.input.TaskID != 7 || service.input.Kind != KindUserList || service.input.UserID != 9 || service.input.Scope != ScopeSelected || len(service.input.IDs) != 1 {
		t.Fatalf("unexpected service input: %#v", service.input)
	}
}

func TestDecodeRunPayloadDefaultsOldUserListScopeToSelected(t *testing.T) {
	payload, err := DecodeRunPayload([]byte(`{"task_id":7,"kind":"user_list","user_id":9,"platform":"admin","ids":[3]}`))
	if err != nil {
		t.Fatalf("DecodeRunPayload returned error: %v", err)
	}
	if payload.Scope != ScopeSelected {
		t.Fatalf("expected old user_list payload to default selected scope, got %#v", payload)
	}
}
