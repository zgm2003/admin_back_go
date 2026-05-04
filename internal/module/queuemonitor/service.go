package queuemonitor

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/platform/taskqueue"
)

const timeLayout = "2006-01-02 15:04:05"

// Inspector is the read-only queue inspection dependency.
type Inspector interface {
	Queues(ctx context.Context) ([]string, error)
	QueueInfo(ctx context.Context, queue string) (QueueSnapshot, error)
	RetryTasks(ctx context.Context, queue string, page int, pageSize int) ([]TaskSnapshot, error)
	ArchivedTasks(ctx context.Context, queue string, page int, pageSize int) ([]TaskSnapshot, error)
}

type taskqueueInspector struct {
	inner *taskqueue.Inspector
}

func NewTaskqueueInspector(inner *taskqueue.Inspector) Inspector {
	if inner == nil {
		return nil
	}
	return taskqueueInspector{inner: inner}
}

func (i taskqueueInspector) Queues(ctx context.Context) ([]string, error) {
	return i.inner.Queues(ctx)
}

func (i taskqueueInspector) QueueInfo(ctx context.Context, queue string) (QueueSnapshot, error) {
	info, err := i.inner.QueueInfo(ctx, queue)
	if err != nil {
		return QueueSnapshot{}, err
	}
	return QueueSnapshot{
		Name:           info.Name,
		Pending:        info.Pending,
		Active:         info.Active,
		Scheduled:      info.Scheduled,
		Retry:          info.Retry,
		Archived:       info.Archived,
		Completed:      info.Completed,
		Processed:      info.Processed,
		Failed:         info.Failed,
		ProcessedTotal: info.ProcessedTotal,
		FailedTotal:    info.FailedTotal,
		Paused:         info.Paused,
		Latency:        info.Latency,
	}, nil
}

func (i taskqueueInspector) RetryTasks(ctx context.Context, queue string, page int, pageSize int) ([]TaskSnapshot, error) {
	tasks, err := i.inner.RetryTasks(ctx, queue, page, pageSize)
	if err != nil {
		return nil, err
	}
	return mapTaskqueueTasks(tasks), nil
}

func (i taskqueueInspector) ArchivedTasks(ctx context.Context, queue string, page int, pageSize int) ([]TaskSnapshot, error) {
	tasks, err := i.inner.ArchivedTasks(ctx, queue, page, pageSize)
	if err != nil {
		return nil, err
	}
	return mapTaskqueueTasks(tasks), nil
}

func mapTaskqueueTasks(tasks []taskqueue.TaskInfo) []TaskSnapshot {
	items := make([]TaskSnapshot, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, TaskSnapshot{
			ID:           task.ID,
			Type:         task.Type,
			Payload:      task.Payload,
			MaxRetry:     task.MaxRetry,
			Retried:      task.Retried,
			LastErr:      task.LastErr,
			LastFailedAt: task.LastFailedAt,
		})
	}
	return items
}

type QueueSnapshot struct {
	Name           string
	Pending        int
	Active         int
	Scheduled      int
	Retry          int
	Archived       int
	Completed      int
	Processed      int
	Failed         int
	ProcessedTotal int
	FailedTotal    int
	Paused         bool
	Latency        time.Duration
}

type TaskSnapshot struct {
	ID           string
	Type         string
	Payload      []byte
	MaxRetry     int
	Retried      int
	LastErr      string
	LastFailedAt time.Time
}

type Options struct {
	QueueNames []string
}

type Service struct {
	inspector  Inspector
	queueNames []string
	queueSet   map[string]struct{}
}

func NewService(inspector Inspector, options Options) *Service {
	queueNames := normalizeQueueNames(options.QueueNames)
	queueSet := make(map[string]struct{}, len(queueNames))
	for _, name := range queueNames {
		queueSet[name] = struct{}{}
	}
	return &Service{inspector: inspector, queueNames: queueNames, queueSet: queueSet}
}

func (s *Service) List(ctx context.Context) ([]QueueItem, *apperror.Error) {
	if s == nil || s.inspector == nil {
		return nil, apperror.Internal("队列监控服务未配置")
	}
	queueNames := append([]string{}, s.queueNames...)
	knownQueues, err := s.inspector.Queues(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "读取队列列表失败", err)
	}
	queueNames = mergeQueueNames(queueNames, knownQueues)

	items := make([]QueueItem, 0, len(queueNames))
	for _, name := range queueNames {
		snapshot, err := s.inspector.QueueInfo(ctx, name)
		if err != nil {
			if errors.Is(err, taskqueue.ErrClientNotReady) {
				return nil, apperror.Internal("队列监控服务未配置")
			}
			if errors.Is(err, taskqueue.ErrQueueNotFound) {
				// Asynq may not create empty queue keys yet. Keep configured lanes visible.
				snapshot = QueueSnapshot{Name: name}
			} else {
				return nil, apperror.Wrap(apperror.CodeInternal, 500, "读取队列状态失败", err)
			}
		}
		items = append(items, queueItem(name, snapshot))
	}
	return items, nil
}

func (s *Service) FailedList(ctx context.Context, query FailedListQuery) (*FailedListResponse, *apperror.Error) {
	if s == nil || s.inspector == nil {
		return nil, apperror.Internal("队列监控服务未配置")
	}
	queue := strings.TrimSpace(query.Queue)
	if !s.isAllowedQueue(queue) {
		return nil, apperror.BadRequest("无效的队列名称")
	}
	page := query.CurrentPage
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	snapshot, err := s.inspector.QueueInfo(ctx, queue)
	if err != nil {
		if errors.Is(err, taskqueue.ErrQueueNotFound) {
			return &FailedListResponse{
				List: []FailedTaskItem{},
				Page: Page{CurrentPage: page, PageSize: pageSize, TotalPage: 0, Total: 0},
			}, nil
		}
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "读取队列状态失败", err)
	}
	total := int64(snapshot.Retry + snapshot.Archived)
	limit := page * pageSize
	if limit < pageSize {
		limit = pageSize
	}

	retryTasks, err := s.inspector.RetryTasks(ctx, queue, 1, limit)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "读取重试任务失败", err)
	}
	archivedTasks, err := s.inspector.ArchivedTasks(ctx, queue, 1, limit)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "读取归档任务失败", err)
	}

	items := make([]FailedTaskItem, 0, len(retryTasks)+len(archivedTasks))
	for index, task := range retryTasks {
		items = append(items, failedTaskItem(task, "retry", index+1))
	}
	for index, task := range archivedTasks {
		items = append(items, failedTaskItem(task, "archived", len(items)+index+1))
	}
	items = paginateFailedItems(items, page, pageSize)

	return &FailedListResponse{
		List: items,
		Page: Page{CurrentPage: page, PageSize: pageSize, TotalPage: totalPage(total, pageSize), Total: total},
	}, nil
}

func paginateFailedItems(items []FailedTaskItem, page int, pageSize int) []FailedTaskItem {
	if page <= 0 || pageSize <= 0 {
		return items
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []FailedTaskItem{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func (s *Service) isAllowedQueue(queue string) bool {
	if queue == "" {
		return false
	}
	if len(s.queueSet) == 0 {
		return true
	}
	_, ok := s.queueSet[queue]
	return ok
}

func normalizeQueueNames(names []string) []string {
	if len(names) == 0 {
		names = []string{taskqueue.QueueCritical, taskqueue.QueueDefault, taskqueue.QueueLow}
	}
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func mergeQueueNames(configured []string, discovered []string) []string {
	result := append([]string{}, configured...)
	seen := make(map[string]struct{}, len(result)+len(discovered))
	for _, name := range result {
		seen[name] = struct{}{}
	}
	extra := make([]string, 0, len(discovered))
	for _, name := range discovered {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		extra = append(extra, name)
	}
	sort.Strings(extra)
	return append(result, extra...)
}

func queueItem(name string, snapshot QueueSnapshot) QueueItem {
	return QueueItem{
		Name:           name,
		Label:          queueLabel(name),
		Group:          queueGroup(name),
		Waiting:        snapshot.Pending,
		Delayed:        snapshot.Scheduled,
		Failed:         snapshot.Retry + snapshot.Archived,
		Pending:        snapshot.Pending,
		Active:         snapshot.Active,
		Scheduled:      snapshot.Scheduled,
		Retry:          snapshot.Retry,
		Archived:       snapshot.Archived,
		Completed:      snapshot.Completed,
		Processed:      snapshot.Processed,
		FailedToday:    snapshot.Failed,
		ProcessedTotal: snapshot.ProcessedTotal,
		FailedTotal:    snapshot.FailedTotal,
		Paused:         snapshot.Paused,
		LatencyMs:      snapshot.Latency.Milliseconds(),
	}
}

func queueLabel(name string) string {
	switch name {
	case taskqueue.QueueCritical:
		return "高优先级队列"
	case taskqueue.QueueDefault:
		return "默认队列"
	case taskqueue.QueueLow:
		return "低优先级队列"
	default:
		return name
	}
}

func queueGroup(name string) string {
	switch name {
	case taskqueue.QueueCritical:
		return "critical"
	case taskqueue.QueueDefault:
		return "default"
	case taskqueue.QueueLow:
		return "low"
	default:
		return "custom"
	}
}

func failedTaskItem(task TaskSnapshot, state string, index int) FailedTaskItem {
	var data map[string]any
	if len(task.Payload) > 0 {
		var decoded map[string]any
		if err := json.Unmarshal(task.Payload, &decoded); err == nil {
			data = decoded
		}
	}
	return FailedTaskItem{
		Index:        index,
		ID:           task.ID,
		Type:         task.Type,
		State:        state,
		Data:         data,
		MaxAttempts:  task.MaxRetry,
		Attempts:     task.Retried,
		Error:        task.LastErr,
		Raw:          string(task.Payload),
		LastFailedAt: formatTime(task.LastFailedAt),
	}
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int((total + int64(pageSize) - 1) / int64(pageSize))
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(timeLayout)
}
