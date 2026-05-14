package queuemonitor

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeInspector struct {
	queues       []string
	queueInfos   map[string]QueueSnapshot
	retryTasks   map[string][]TaskSnapshot
	archiveTasks map[string][]TaskSnapshot
	err          error
	queuesErr    error
	queueInfoErr error
	retryErr     error
	archiveErr   error
}

func (f *fakeInspector) Queues(ctx context.Context) ([]string, error) {
	if f.queuesErr != nil {
		return nil, f.queuesErr
	}
	return f.queues, f.err
}

func (f *fakeInspector) QueueInfo(ctx context.Context, queue string) (QueueSnapshot, error) {
	if f.queueInfoErr != nil {
		return QueueSnapshot{}, f.queueInfoErr
	}
	if f.queueInfos == nil {
		return QueueSnapshot{}, f.err
	}
	return f.queueInfos[queue], f.err
}

func (f *fakeInspector) RetryTasks(ctx context.Context, queue string, page int, pageSize int) ([]TaskSnapshot, error) {
	if f.retryErr != nil {
		return nil, f.retryErr
	}
	return f.retryTasks[queue], f.err
}

func (f *fakeInspector) ArchivedTasks(ctx context.Context, queue string, page int, pageSize int) ([]TaskSnapshot, error) {
	if f.archiveErr != nil {
		return nil, f.archiveErr
	}
	return f.archiveTasks[queue], f.err
}

func TestServiceListReturnsConfiguredQueueLanesEvenWhenAsynqHasNoKeys(t *testing.T) {
	inspector := &fakeInspector{
		queues: []string{},
		queueInfos: map[string]QueueSnapshot{
			"critical": {Name: "critical", Pending: 1, Scheduled: 2, Retry: 3, Archived: 4, Active: 5, Completed: 6, Processed: 7, Failed: 8, Paused: true, Latency: 2 * time.Second},
		},
	}
	svc := NewService(inspector, Options{QueueNames: []string{"critical", "default", "low"}})

	got, appErr := svc.List(context.Background())
	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if len(got) != 3 {
		t.Fatalf("expected three queue lanes, got %#v", got)
	}
	if got[0].Name != "critical" || got[0].Label != "高优先级队列" || got[0].Group != "critical" {
		t.Fatalf("critical queue metadata mismatch: %#v", got[0])
	}
	if got[0].Waiting != 1 || got[0].Delayed != 2 || got[0].Failed != 7 || got[0].Retry != 3 || got[0].Archived != 4 || !got[0].Paused || got[0].LatencyMs != 2000 {
		t.Fatalf("critical queue counts mismatch: %#v", got[0])
	}
	if got[1].Name != "default" || got[1].Group != "default" || got[2].Name != "low" || got[2].Group != "low" {
		t.Fatalf("default/low metadata mismatch: %#v", got)
	}
}

func TestServiceFailedListCombinesRetryAndArchivedTasks(t *testing.T) {
	inspector := &fakeInspector{
		queueInfos: map[string]QueueSnapshot{
			"critical": {Name: "critical", Retry: 1, Archived: 1},
		},
		retryTasks: map[string][]TaskSnapshot{
			"critical": {{ID: "retry-1", Type: "auth:login-log:v1", Payload: []byte(`{"user_id":1}`), MaxRetry: 3, Retried: 2, LastErr: "boom", LastFailedAt: mustQueueTime(t, "2026-05-04 10:00:00")}},
		},
		archiveTasks: map[string][]TaskSnapshot{
			"critical": {{ID: "archived-1", Type: "system:no-op:v1", Payload: []byte(`not-json`), MaxRetry: 1, Retried: 1, LastErr: "dead", LastFailedAt: mustQueueTime(t, "2026-05-04 10:01:00")}},
		},
	}
	svc := NewService(inspector, Options{QueueNames: []string{"critical"}})

	got, appErr := svc.FailedList(context.Background(), FailedListQuery{Queue: "critical", CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if got.Page.Total != 2 || got.Page.CurrentPage != 1 || got.Page.PageSize != 20 || got.Page.TotalPage != 1 {
		t.Fatalf("page mismatch: %#v", got.Page)
	}
	if len(got.List) != 2 {
		t.Fatalf("expected two failed tasks, got %#v", got.List)
	}
	if got.List[0].ID != "retry-1" || got.List[0].State != "retry" || got.List[0].Data == nil || got.List[0].Raw != `{"user_id":1}` {
		t.Fatalf("retry task mismatch: %#v", got.List[0])
	}
	if got.List[1].ID != "archived-1" || got.List[1].State != "archived" || got.List[1].Data != nil || got.List[1].Raw != "not-json" {
		t.Fatalf("archived task mismatch: %#v", got.List[1])
	}
}

func TestServiceFailedListRejectsUnknownQueue(t *testing.T) {
	svc := NewService(&fakeInspector{}, Options{QueueNames: []string{"critical"}})

	_, appErr := svc.FailedList(context.Background(), FailedListQuery{Queue: "evil", CurrentPage: 1, PageSize: 20})
	if appErr == nil || appErr.MessageID != "queuemonitor.queue.invalid" {
		t.Fatalf("expected keyed unknown queue error, got %#v", appErr)
	}
}

func TestServiceListWrapsQueueListError(t *testing.T) {
	svc := NewService(&fakeInspector{queuesErr: errors.New("redis down")}, Options{QueueNames: []string{"critical"}})

	_, appErr := svc.List(context.Background())
	if appErr == nil || appErr.MessageID != "queuemonitor.queue_list_failed" {
		t.Fatalf("expected keyed queue list error, got %#v", appErr)
	}
}

func TestServiceFailedListWrapsTaskReadErrors(t *testing.T) {
	tests := []struct {
		name      string
		inspector *fakeInspector
		messageID string
	}{
		{
			name: "queue status",
			inspector: &fakeInspector{
				queueInfoErr: errors.New("status down"),
			},
			messageID: "queuemonitor.queue_status_failed",
		},
		{
			name: "retry tasks",
			inspector: &fakeInspector{
				queueInfos: map[string]QueueSnapshot{"critical": {Name: "critical", Retry: 1}},
				retryErr:   errors.New("retry down"),
			},
			messageID: "queuemonitor.retry_tasks_failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(tt.inspector, Options{QueueNames: []string{"critical"}})

			_, appErr := svc.FailedList(context.Background(), FailedListQuery{Queue: "critical", CurrentPage: 1, PageSize: 20})
			if appErr == nil || appErr.MessageID != tt.messageID {
				t.Fatalf("expected keyed error %q, got %#v", tt.messageID, appErr)
			}
		})
	}
}

func mustQueueTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation(timeLayout, value, time.Local)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	return parsed
}
