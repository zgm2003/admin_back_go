package aiimage

import (
	"context"
	"log/slog"
	"testing"

	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/shared/enum"
)

type fakeImageJobService struct {
	input GenerateInput
}

func (f *fakeImageJobService) ExecuteGenerate(ctx context.Context, input GenerateInput) (*GenerateResult, error) {
	f.input = input
	return &GenerateResult{TaskID: input.TaskID, Status: StatusSuccess}, nil
}

func TestNewGenerateTaskBuildsStablePayload(t *testing.T) {
	task, err := NewGenerateTask(GeneratePayload{Platform: enum.PlatformAdmin, TaskID: 12, UserID: 34})
	if err != nil {
		t.Fatalf("NewGenerateTask returned error: %v", err)
	}
	if task.Type != TypeGenerateV1 || task.Queue != "" || task.MaxRetry != 0 {
		t.Fatalf("unexpected task metadata: %#v", task)
	}
	payload, err := DecodeGeneratePayload(task.Payload)
	if err != nil {
		t.Fatalf("DecodeGeneratePayload returned error: %v", err)
	}
	if payload.Platform != enum.PlatformAdmin || payload.TaskID != 12 || payload.UserID != 34 {
		t.Fatalf("payload mismatch: %#v", payload)
	}
}

func TestNewGenerateTaskRejectsMissingOrUnregisteredPlatform(t *testing.T) {
	for _, platform := range []string{"", "partner_portal"} {
		if _, err := NewGenerateTask(GeneratePayload{Platform: platform, TaskID: 12, UserID: 34}); err == nil {
			t.Fatalf("expected platform %q to be rejected", platform)
		}
	}
}

func TestRegisterHandlersDispatchesGenerateTask(t *testing.T) {
	service := &fakeImageJobService{}
	mux := taskqueue.NewMux()
	RegisterHandlers(mux, service, slog.Default())

	task, err := NewGenerateTask(GeneratePayload{Platform: enum.PlatformAdmin, TaskID: 12, UserID: 34})
	if err != nil {
		t.Fatalf("NewGenerateTask returned error: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessProjectTask returned error: %v", err)
	}
	if service.input.Platform != enum.PlatformAdmin || service.input.TaskID != 12 || service.input.UserID != 34 {
		t.Fatalf("expected job payload to be dispatched, got %#v", service.input)
	}
}
