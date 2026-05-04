package taskqueue

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/config"

	"github.com/hibiken/asynq"
)

var (
	ErrRedisAddrRequired    = errors.New("queue redis addr is required")
	ErrTaskTypeRequired     = errors.New("task type is required")
	ErrQueueRequired        = errors.New("task queue is required")
	ErrClientNotReady       = errors.New("task queue client is not ready")
	ErrQueueWeightRequired  = errors.New("at least one queue weight is required")
	ErrHandlerRequired      = errors.New("task handler is required")
	ErrHandlerNotRegistered = errors.New("task handler is not registered")
	ErrQueueNotFound        = errors.New("queue not found")
)

// Task is the project-owned queue contract. Business code should build this
// type instead of importing Asynq directly.
type Task struct {
	Type      string
	Payload   []byte
	Queue     string
	MaxRetry  int
	Timeout   time.Duration
	UniqueTTL time.Duration
}

// EnqueueResult is the stable result returned by queue producers.
type EnqueueResult struct {
	ID    string
	Queue string
	Type  string
}

// Enqueuer is the producer boundary used by services and scheduled jobs.
type Enqueuer interface {
	Enqueue(ctx context.Context, task Task) (EnqueueResult, error)
}

// Client owns the Asynq producer and hides Asynq options from business code.
type Client struct {
	client          *asynq.Client
	redisOpt        asynq.RedisClientOpt
	defaultQueue    string
	defaultMaxRetry int
	defaultTimeout  time.Duration
}

// NewClient builds a queue producer without pinging Redis. Runtime connectivity
// belongs to worker startup/readiness, not config loading.
func NewClient(redisCfg config.RedisConfig, queueCfg config.QueueConfig) (*Client, error) {
	redisOpt, err := redisOpt(redisCfg, queueCfg.RedisDB)
	if err != nil {
		return nil, err
	}

	return &Client{
		client:          asynq.NewClient(redisOpt),
		redisOpt:        redisOpt,
		defaultQueue:    strings.TrimSpace(queueCfg.DefaultQueue),
		defaultMaxRetry: queueCfg.DefaultMaxRetry,
		defaultTimeout:  queueCfg.DefaultTimeout,
	}, nil
}

// Enqueue publishes a task to Redis-backed Asynq.
func (c *Client) Enqueue(ctx context.Context, task Task) (EnqueueResult, error) {
	if c == nil || c.client == nil {
		return EnqueueResult{}, ErrClientNotReady
	}
	if ctx == nil {
		ctx = context.Background()
	}

	asynqTask, opts, err := c.normalize(task)
	if err != nil {
		return EnqueueResult{}, err
	}

	info, err := c.client.EnqueueContext(ctx, asynqTask, opts...)
	if err != nil {
		return EnqueueResult{}, err
	}
	return EnqueueResult{ID: info.ID, Queue: info.Queue, Type: info.Type}, nil
}

// Close releases the producer resources.
func (c *Client) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

func (c *Client) normalize(task Task) (*asynq.Task, []asynq.Option, error) {
	task.Type = strings.TrimSpace(task.Type)
	if task.Type == "" {
		return nil, nil, ErrTaskTypeRequired
	}

	queue := strings.TrimSpace(task.Queue)
	if queue == "" {
		queue = c.defaultQueue
	}
	if queue == "" {
		return nil, nil, ErrQueueRequired
	}

	maxRetry := task.MaxRetry
	if maxRetry == 0 {
		maxRetry = c.defaultMaxRetry
	}
	timeout := task.Timeout
	if timeout == 0 {
		timeout = c.defaultTimeout
	}

	opts := []asynq.Option{asynq.Queue(queue)}
	if maxRetry >= 0 {
		opts = append(opts, asynq.MaxRetry(maxRetry))
	}
	if timeout > 0 {
		opts = append(opts, asynq.Timeout(timeout))
	}
	if task.UniqueTTL > 0 {
		opts = append(opts, asynq.Unique(task.UniqueTTL))
	}

	return asynq.NewTask(task.Type, task.Payload), opts, nil
}

func redisOpt(redisCfg config.RedisConfig, db int) (asynq.RedisClientOpt, error) {
	addr := strings.TrimSpace(redisCfg.Addr)
	if addr == "" {
		return asynq.RedisClientOpt{}, ErrRedisAddrRequired
	}
	return asynq.RedisClientOpt{
		Addr:     addr,
		Password: redisCfg.Password,
		DB:       db,
	}, nil
}

// RedisConnOpt returns the Asynq Redis connection option for components that
// must integrate with official Asynq tooling, such as asynqmon. Keep this as a
// platform boundary; modules should not build Asynq options directly.
func RedisConnOpt(redisCfg config.RedisConfig, queueCfg config.QueueConfig) (asynq.RedisClientOpt, error) {
	return redisOpt(redisCfg, queueCfg.RedisDB)
}
