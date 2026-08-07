package taskqueue

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/telemetry"

	"github.com/hibiken/asynq"
)

var (
	ErrRedisAddrRequired    = errors.New("queue redis addr is required")
	ErrTaskTypeRequired     = errors.New("task type is required")
	ErrClientNotReady       = errors.New("task queue client is not ready")
	ErrQueueWeightRequired  = errors.New("at least one queue weight is required")
	ErrHandlerRequired      = errors.New("task handler is required")
	ErrHandlerNotRegistered = errors.New("task handler is not registered")
	ErrQueueNotFound        = errors.New("queue not found")
	ErrTaskPolicyOverride   = errors.New("task delivery policy must come from the registry")
	ErrRegistryRequired     = errors.New("task registry is required")
)

func IsDuplicateTask(err error) bool {
	return errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict)
}

// Task is the project-owned queue contract. Business code should build this
// type instead of importing Asynq directly.
type Task struct {
	ID        string
	Type      string
	Payload   []byte
	Queue     string
	Retry     int
	MaxRetry  int
	Timeout   time.Duration
	UniqueTTL time.Duration
}

const (
	DefaultMaxRetry = 3
	DefaultTimeout  = 30 * time.Second
)

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
	client   *asynq.Client
	enqueue  func(context.Context, *asynq.Task, ...asynq.Option) (*asynq.TaskInfo, error)
	redisOpt asynq.RedisClientOpt
	registry *Registry
	recorder telemetry.Recorder
}

type options struct {
	recorder telemetry.Recorder
	registry *Registry
}

type Option func(*options)

func WithTelemetry(recorder telemetry.Recorder) Option {
	return func(options *options) {
		options.recorder = recorder
	}
}

// WithRegistry makes the client accept only registered task types and apply
// their centrally owned delivery policy.
func WithRegistry(registry *Registry) Option {
	return func(options *options) {
		options.registry = registry
	}
}

// NewClient builds a queue producer without pinging Redis. Runtime connectivity
// belongs to worker startup/readiness, not config loading.
func NewClient(redisCfg config.RedisConfig, queueCfg config.QueueConfig, optionValues ...Option) (*Client, error) {
	redisOpt, err := redisOpt(redisCfg, queueCfg.RedisDB)
	if err != nil {
		return nil, err
	}

	settings := queueOptions(optionValues)
	if settings.registry == nil {
		return nil, ErrRegistryRequired
	}
	client := asynq.NewClient(redisOpt)
	return &Client{
		client:   client,
		enqueue:  client.EnqueueContext,
		redisOpt: redisOpt,
		registry: settings.registry,
		recorder: settings.recorder,
	}, nil
}

// Enqueue publishes a task to Redis-backed Asynq.
func (c *Client) Enqueue(ctx context.Context, task Task) (EnqueueResult, error) {
	if c == nil || c.enqueue == nil {
		return EnqueueResult{}, ErrClientNotReady
	}
	if ctx == nil {
		ctx = context.Background()
	}

	asynqTask, opts, err := c.normalize(task)
	if err != nil {
		return EnqueueResult{}, err
	}

	queue := ""
	if definition, ok := c.registry.definition(task.Type); ok {
		queue = definition.Queue
	}
	startedAt := time.Now()
	info, err := c.enqueue(ctx, asynqTask, opts...)
	if err != nil {
		c.recordEnqueue(task.Type, queue, "error", time.Since(startedAt))
		return EnqueueResult{}, err
	}
	c.recordEnqueue(info.Type, info.Queue, "enqueued", time.Since(startedAt))
	return EnqueueResult{ID: info.ID, Queue: info.Queue, Type: info.Type}, nil
}

func (c *Client) recordEnqueue(taskType string, queue string, outcome string, duration time.Duration) {
	recorder := c.recorder
	if recorder == nil {
		recorder = telemetry.Noop()
	}
	attributes := telemetry.Attributes{
		"queue.type":    taskType,
		"queue.lane":    queue,
		"queue.outcome": outcome,
	}
	recorder.Count("queue.enqueues", 1, attributes)
	recorder.Observe("queue.enqueue.duration_seconds", duration.Seconds(), attributes)
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
	if c.registry == nil {
		return nil, nil, ErrRegistryRequired
	}
	if strings.TrimSpace(task.Queue) != "" || task.Retry != 0 || task.MaxRetry != 0 || task.Timeout != 0 || task.UniqueTTL != 0 {
		return nil, nil, ErrTaskPolicyOverride
	}
	registeredTask, policy, err := c.registry.Task(task.Type, task.Payload)
	if err != nil {
		return nil, nil, err
	}
	registeredTask.ID = task.ID
	opts := []asynq.Option{
		asynq.Queue(policy.Queue),
		asynq.MaxRetry(policy.MaxRetry),
		asynq.Timeout(policy.Timeout),
	}
	if policy.UniqueTTL > 0 {
		opts = append(opts, asynq.Unique(policy.UniqueTTL))
	}
	if strings.TrimSpace(registeredTask.ID) != "" {
		opts = append(opts, asynq.TaskID(strings.TrimSpace(registeredTask.ID)))
	}
	return asynq.NewTask(registeredTask.Type, registeredTask.Payload), opts, nil
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

func queueOptions(optionValues []Option) options {
	settings := options{recorder: telemetry.Noop()}
	for _, option := range optionValues {
		if option != nil {
			option(&settings)
		}
	}
	if settings.recorder == nil {
		settings.recorder = telemetry.Noop()
	}
	return settings
}
