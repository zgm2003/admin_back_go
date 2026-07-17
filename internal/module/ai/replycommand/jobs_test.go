package replycommand

import (
	"context"
	"testing"

	"admin_back_go/internal/infra/taskqueue"
)

func TestReplyCommandWakeTaskUsesRegisteredTypeAndRunsCommand(t *testing.T) {
	task, err := NewWakeTask(77)
	if err != nil {
		t.Fatal(err)
	}
	if task.Type != TypeReplyCommandV1 || task.Queue != "" || task.MaxRetry != 0 || task.Timeout != 0 {
		t.Fatalf("task duplicated policy: %+v", task)
	}

	runner := &fakeJobRunner{}
	registry := taskqueue.NewRegistry()
	if err := RegisterTaskDefinition(registry, runner); err != nil {
		t.Fatal(err)
	}
	_, policy, err := registry.Task(TypeReplyCommandV1, task.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Queue != taskqueue.QueueDefault || policy.MaxRetry != 0 || policy.Timeout <= 0 {
		t.Fatalf("unexpected reply policy: %+v", policy)
	}
	if err := registry.Handle(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if runner.commandID != 77 {
		t.Fatalf("command id=%d", runner.commandID)
	}
}

type fakeJobRunner struct {
	commandID uint64
}

func (f *fakeJobRunner) RunCommand(_ context.Context, commandID uint64) (bool, error) {
	f.commandID = commandID
	return true, nil
}
