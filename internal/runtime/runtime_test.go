package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestCleanupRunsOnceInReverseOrderAndJoinsFailures(t *testing.T) {
	var got []string
	dbErr := errors.New("db close")
	queueErr := errors.New("queue close")
	stack := NewCleanup()
	stack.Add("db", func(context.Context) error {
		got = append(got, "db")
		return dbErr
	})
	stack.Add("redis", func(context.Context) error {
		got = append(got, "redis")
		return nil
	})
	stack.Add("queue", func(context.Context) error {
		got = append(got, "queue")
		return queueErr
	})

	err := stack.Close(context.Background())
	secondErr := stack.Close(context.Background())

	if !reflect.DeepEqual(got, []string{"queue", "redis", "db"}) {
		t.Fatalf("cleanup order/call count mismatch: %v", got)
	}
	if !errors.Is(err, dbErr) || !errors.Is(err, queueErr) {
		t.Fatalf("joined causes missing: %v", err)
	}
	if !strings.Contains(err.Error(), "queue: queue close") || !strings.Contains(err.Error(), "db: db close") {
		t.Fatalf("resource annotations missing: %v", err)
	}
	if secondErr != nil {
		t.Fatalf("subsequent close should be a no-op, got %v", secondErr)
	}
}

func TestCleanupRejectsInvalidOrLateEntries(t *testing.T) {
	stack := NewCleanup()
	if err := stack.Add("", func(context.Context) error { return nil }); !errors.Is(err, ErrCleanupNameRequired) {
		t.Fatalf("empty name error=%v", err)
	}
	if err := stack.Add("db", nil); !errors.Is(err, ErrCleanupFuncRequired) {
		t.Fatalf("nil function error=%v", err)
	}
	if err := stack.Close(context.Background()); err != nil {
		t.Fatalf("empty cleanup close=%v", err)
	}
	if err := stack.Add("redis", func(context.Context) error { return nil }); !errors.Is(err, ErrCleanupClosed) {
		t.Fatalf("late add error=%v", err)
	}
}

func TestCleanupConcurrentCloseExecutesEntriesOnce(t *testing.T) {
	stack := NewCleanup()
	var mu sync.Mutex
	calls := 0
	stack.Add("db", func(context.Context) error {
		mu.Lock()
		calls++
		mu.Unlock()
		return nil
	})

	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_ = stack.Close(context.Background())
		}()
	}
	wait.Wait()

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("cleanup called %d times", calls)
	}
}

type testRuntime struct{}

func (testRuntime) Start(context.Context) error    { return nil }
func (testRuntime) Shutdown(context.Context) error { return nil }
func (testRuntime) Health(context.Context) Report {
	return Report{Status: "ready", Checks: map[string]Check{"database": {Status: "up"}}}
}

func TestRuntimeContractRemainsNarrow(t *testing.T) {
	var process Runtime = testRuntime{}
	report := process.Health(context.Background())
	if report.Status != "ready" || report.Checks["database"].Status != "up" {
		t.Fatalf("unexpected health report: %+v", report)
	}
}
