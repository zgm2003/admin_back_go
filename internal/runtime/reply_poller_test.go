package runtime

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

type fakeReplyPollRunner struct {
	calls atomic.Int64
}

func (f *fakeReplyPollRunner) RunOnce(context.Context) (bool, error) {
	f.calls.Add(1)
	return false, nil
}

func TestReplyCommandPollerRunsAndStops(t *testing.T) {
	runner := &fakeReplyPollRunner{}
	cleanup, err := startReplyCommandPoller(context.Background(), runner, 5*time.Millisecond, 2, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for runner.calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if runner.calls.Load() < 2 {
		t.Fatalf("poll calls=%d", runner.calls.Load())
	}
	if err := cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	stoppedAt := runner.calls.Load()
	time.Sleep(20 * time.Millisecond)
	if runner.calls.Load() != stoppedAt {
		t.Fatalf("poller continued after cleanup: before=%d after=%d", stoppedAt, runner.calls.Load())
	}
}
