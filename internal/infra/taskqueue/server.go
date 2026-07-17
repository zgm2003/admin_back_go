package taskqueue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/telemetry"

	"github.com/hibiken/asynq"
)

const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueLow      = "low"

	DefaultCriticalWeight  = 6
	DefaultQueueWeight     = 3
	DefaultLowWeight       = 1
	DefaultShutdownTimeout = 10 * time.Second
)

type HandlerFunc func(ctx context.Context, task Task) error

// Mux maps versioned task types to project-owned handlers.
type Mux struct {
	mux      *asynq.ServeMux
	handlers map[string]HandlerFunc
	recorder telemetry.Recorder
}

// NewMux creates an empty task handler registry.

func NewMux(optionValues ...Option) *Mux {
	settings := queueOptions(optionValues)
	return &Mux{
		mux:      asynq.NewServeMux(),
		handlers: make(map[string]HandlerFunc),
		recorder: settings.recorder,
	}
}

// HandleFunc registers a handler for one task type or type prefix.
func (m *Mux) HandleFunc(pattern string, handler HandlerFunc) {
	if m == nil || m.mux == nil {
		panic("taskqueue: nil mux")
	}
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		panic("taskqueue: task pattern is required")
	}
	if handler == nil {
		panic("taskqueue: task handler is required")
	}
	if m.handlers == nil {
		m.handlers = make(map[string]HandlerFunc)
	}
	m.handlers[pattern] = handler

	m.mux.HandleFunc(pattern, func(ctx context.Context, task *asynq.Task) error {
		queue, _ := asynq.GetQueueName(ctx)
		retry, _ := asynq.GetRetryCount(ctx)
		maxRetry, _ := asynq.GetMaxRetry(ctx)
		return m.process(ctx, Task{
			Type:     task.Type(),
			Payload:  task.Payload(),
			Queue:    queue,
			Retry:    retry,
			MaxRetry: maxRetry,
		}, handler)
	})
}

// ProcessTask runs an Asynq task through the mux. It is mainly useful in tests.
func (m *Mux) ProcessTask(ctx context.Context, task *asynq.Task) error {
	if m == nil || m.mux == nil {
		return ErrHandlerRequired
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if task == nil || strings.TrimSpace(task.Type()) == "" {
		return ErrTaskTypeRequired
	}
	if !m.hasHandler(task.Type()) {
		return fmt.Errorf("%w: %s", ErrHandlerNotRegistered, task.Type())
	}
	return m.mux.ProcessTask(ctx, task)
}

// ProcessProjectTask runs a project task through the mux without hitting Redis.
func (m *Mux) ProcessProjectTask(ctx context.Context, task Task) error {
	if m == nil || m.mux == nil {
		return ErrHandlerRequired
	}
	if ctx == nil {
		ctx = context.Background()
	}
	task.Type = strings.TrimSpace(task.Type)
	if task.Type == "" {
		return ErrTaskTypeRequired
	}
	handler := m.handlers[task.Type]
	if handler == nil {
		return fmt.Errorf("%w: %s", ErrHandlerNotRegistered, task.Type)
	}
	return m.process(ctx, task, handler)
}

func (m *Mux) hasHandler(taskType string) bool {
	if m == nil || len(m.handlers) == 0 {
		return false
	}
	return m.handlers[strings.TrimSpace(taskType)] != nil
}

func (m *Mux) process(ctx context.Context, task Task, handler HandlerFunc) error {
	queue := strings.TrimSpace(task.Queue)
	if queue == "" {
		queue = QueueDefault
	}
	startedAt := time.Now()
	err := handler(ctx, task)
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	attributes := telemetry.Attributes{
		"queue.type":      task.Type,
		"queue.lane":      queue,
		"queue.retry":     task.Retry,
		"queue.outcome":   outcome,
		"queue.exhausted": err != nil && task.MaxRetry >= 0 && task.Retry >= task.MaxRetry,
	}
	recorder := m.recorder
	if recorder == nil {
		recorder = telemetry.Noop()
	}
	recorder.Count("queue.handlers", 1, attributes)
	recorder.Observe("queue.handler.duration_seconds", time.Since(startedAt).Seconds(), attributes)
	return err
}

// Server owns the Asynq consumer process.
type Server struct {
	server       *asynq.Server
	errorHandler asynq.ErrorHandler
}

// NewServer builds a queue consumer without pinging Redis.
func NewServer(redisCfg config.RedisConfig, queueCfg config.QueueConfig, optionValues ...Option) (*Server, error) {
	redisOpt, err := redisOpt(redisCfg, queueCfg.RedisDB)
	if err != nil {
		return nil, err
	}

	queues := queueWeights()
	if len(queues) == 0 {
		return nil, ErrQueueWeightRequired
	}

	settings := queueOptions(optionValues)
	errorHandler := asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
		recordQueueFailure(settings.recorder, ctx, task, err)
	})
	return &Server{
		errorHandler: errorHandler,
		server: asynq.NewServer(redisOpt, asynq.Config{
			Concurrency:     queueCfg.Concurrency,
			Queues:          queues,
			ShutdownTimeout: DefaultShutdownTimeout,
			ErrorHandler:    errorHandler,
		}),
	}, nil
}

func recordQueueFailure(recorder telemetry.Recorder, ctx context.Context, task *asynq.Task, err error) {
	if recorder == nil {
		recorder = telemetry.Noop()
	}
	queue, ok := asynq.GetQueueName(ctx)
	if !ok || strings.TrimSpace(queue) == "" {
		queue = QueueDefault
	}
	retry, _ := asynq.GetRetryCount(ctx)
	maxRetry, _ := asynq.GetMaxRetry(ctx)
	taskType := "unknown"
	if task != nil && strings.TrimSpace(task.Type()) != "" {
		taskType = task.Type()
	}
	recorder.Count("queue.failures", 1, telemetry.Attributes{
		"queue.type":          taskType,
		"queue.lane":          queue,
		"queue.retry":         retry,
		"queue.outcome":       "error",
		"queue.lease_expired": errors.Is(err, asynq.ErrLeaseExpired),
		"queue.exhausted":     retry >= maxRetry,
	})
}

// Start starts background consumption. It returns after the Asynq server starts.
func (s *Server) Start(mux *Mux) error {
	if s == nil || s.server == nil {
		return ErrClientNotReady
	}
	if mux == nil || mux.mux == nil {
		return ErrHandlerRequired
	}
	return s.server.Start(mux)
}

// Shutdown stops task consumption and waits for in-flight tasks up to Asynq's
// configured shutdown timeout.
func (s *Server) Shutdown() {
	if s == nil || s.server == nil {
		return
	}
	s.server.Shutdown()
}

func queueWeights() map[string]int {
	return map[string]int{
		QueueCritical: DefaultCriticalWeight,
		QueueDefault:  DefaultQueueWeight,
		QueueLow:      DefaultLowWeight,
	}
}
