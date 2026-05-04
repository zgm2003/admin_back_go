package taskqueue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"admin_back_go/internal/config"

	"github.com/hibiken/asynq"
)

// QueueInfo is the project-owned read model for one Asynq queue snapshot.
type QueueInfo struct {
	Name           string
	MemoryUsage    int64
	Latency        time.Duration
	Size           int
	Groups         int
	Pending        int
	Active         int
	Scheduled      int
	Retry          int
	Archived       int
	Completed      int
	Aggregating    int
	Processed      int
	Failed         int
	ProcessedTotal int
	FailedTotal    int
	Paused         bool
	Timestamp      time.Time
}

// TaskInfo is the project-owned read model for one Asynq task snapshot.
type TaskInfo struct {
	ID            string
	Queue         string
	Type          string
	Payload       []byte
	State         string
	MaxRetry      int
	Retried       int
	LastErr       string
	LastFailedAt  time.Time
	Timeout       time.Duration
	Deadline      time.Time
	Group         string
	NextProcessAt time.Time
}

// Inspector is the read-only queue inspection boundary used by admin modules.
type Inspector struct {
	inspector *asynq.Inspector
}

// NewInspector builds a read-only Asynq inspector without pinging Redis.
func NewInspector(redisCfg config.RedisConfig, queueCfg config.QueueConfig) (*Inspector, error) {
	redisOpt, err := redisOpt(redisCfg, queueCfg.RedisDB)
	if err != nil {
		return nil, err
	}
	return &Inspector{inspector: asynq.NewInspector(redisOpt)}, nil
}

// Queues returns queue names currently known by Asynq.
func (i *Inspector) Queues(ctx context.Context) ([]string, error) {
	if i == nil || i.inspector == nil {
		return nil, ErrClientNotReady
	}
	return i.inspector.Queues()
}

// QueueInfo returns one queue snapshot.
func (i *Inspector) QueueInfo(ctx context.Context, queue string) (QueueInfo, error) {
	if i == nil || i.inspector == nil {
		return QueueInfo{}, ErrClientNotReady
	}
	info, err := i.inspector.GetQueueInfo(queue)
	if err != nil {
		if err = normalizeQueueInfoError(queue, err); errors.Is(err, ErrQueueNotFound) {
			return QueueInfo{}, ErrQueueNotFound
		}
		return QueueInfo{}, err
	}
	return mapQueueInfo(info), nil
}

func normalizeQueueInfoError(queue string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, asynq.ErrQueueNotFound) || isAsynqCurrentStatsQueueNotFound(queue, err) {
		return fmt.Errorf("%w: queue=%q", ErrQueueNotFound, strings.TrimSpace(queue))
	}
	return err
}

func isAsynqCurrentStatsQueueNotFound(queue string, err error) bool {
	queue = strings.TrimSpace(queue)
	if queue == "" || err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "NOT_FOUND:") &&
		strings.Contains(message, fmt.Sprintf("queue %q does not exist", queue))
}

// RetryTasks lists tasks waiting for retry.
func (i *Inspector) RetryTasks(ctx context.Context, queue string, page int, pageSize int) ([]TaskInfo, error) {
	if i == nil || i.inspector == nil {
		return nil, ErrClientNotReady
	}
	tasks, err := i.inspector.ListRetryTasks(queue, asynq.Page(page), asynq.PageSize(pageSize))
	if err != nil {
		return nil, err
	}
	return mapTaskInfos(tasks), nil
}

// ArchivedTasks lists archived/dead failed tasks.
func (i *Inspector) ArchivedTasks(ctx context.Context, queue string, page int, pageSize int) ([]TaskInfo, error) {
	if i == nil || i.inspector == nil {
		return nil, ErrClientNotReady
	}
	tasks, err := i.inspector.ListArchivedTasks(queue, asynq.Page(page), asynq.PageSize(pageSize))
	if err != nil {
		return nil, err
	}
	return mapTaskInfos(tasks), nil
}

// Close releases inspector resources.
func (i *Inspector) Close() error {
	if i == nil || i.inspector == nil {
		return nil
	}
	return i.inspector.Close()
}

func mapQueueInfo(info *asynq.QueueInfo) QueueInfo {
	if info == nil {
		return QueueInfo{}
	}
	return QueueInfo{
		Name:           info.Queue,
		MemoryUsage:    info.MemoryUsage,
		Latency:        info.Latency,
		Size:           info.Size,
		Groups:         info.Groups,
		Pending:        info.Pending,
		Active:         info.Active,
		Scheduled:      info.Scheduled,
		Retry:          info.Retry,
		Archived:       info.Archived,
		Completed:      info.Completed,
		Aggregating:    info.Aggregating,
		Processed:      info.Processed,
		Failed:         info.Failed,
		ProcessedTotal: info.ProcessedTotal,
		FailedTotal:    info.FailedTotal,
		Paused:         info.Paused,
		Timestamp:      info.Timestamp,
	}
}

func mapTaskInfos(tasks []*asynq.TaskInfo) []TaskInfo {
	items := make([]TaskInfo, 0, len(tasks))
	for _, task := range tasks {
		if task == nil {
			continue
		}
		items = append(items, TaskInfo{
			ID:            task.ID,
			Queue:         task.Queue,
			Type:          task.Type,
			Payload:       task.Payload,
			State:         task.State.String(),
			MaxRetry:      task.MaxRetry,
			Retried:       task.Retried,
			LastErr:       task.LastErr,
			LastFailedAt:  task.LastFailedAt,
			Timeout:       task.Timeout,
			Deadline:      task.Deadline,
			Group:         task.Group,
			NextProcessAt: task.NextProcessAt,
		})
	}
	return items
}
