package taskqueue

import (
	"context"
	"fmt"
	"strings"

	"admin_back_go/internal/config"

	"github.com/hibiken/asynq"
)

const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueLow      = "low"
)

type HandlerFunc func(ctx context.Context, task Task) error

// Mux maps versioned task types to project-owned handlers.
type Mux struct {
	mux      *asynq.ServeMux
	handlers map[string]struct{}
}

// NewMux creates an empty task handler registry.
func NewMux() *Mux {
	return &Mux{
		mux:      asynq.NewServeMux(),
		handlers: make(map[string]struct{}),
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
		m.handlers = make(map[string]struct{})
	}
	m.handlers[pattern] = struct{}{}

	m.mux.HandleFunc(pattern, func(ctx context.Context, task *asynq.Task) error {
		return handler(ctx, Task{
			Type:    task.Type(),
			Payload: task.Payload(),
		})
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
	return m.ProcessTask(ctx, asynq.NewTask(task.Type, task.Payload))
}

func (m *Mux) hasHandler(taskType string) bool {
	if m == nil || len(m.handlers) == 0 {
		return false
	}
	_, ok := m.handlers[strings.TrimSpace(taskType)]
	return ok
}

// Server owns the Asynq consumer process.
type Server struct {
	server *asynq.Server
}

// NewServer builds a queue consumer without pinging Redis.
func NewServer(redisCfg config.RedisConfig, queueCfg config.QueueConfig) (*Server, error) {
	redisOpt, err := redisOpt(redisCfg, queueCfg.RedisDB)
	if err != nil {
		return nil, err
	}

	queues := queueWeights(queueCfg)
	if len(queues) == 0 {
		return nil, ErrQueueWeightRequired
	}

	return &Server{
		server: asynq.NewServer(redisOpt, asynq.Config{
			Concurrency:     queueCfg.Concurrency,
			Queues:          queues,
			ShutdownTimeout: queueCfg.ShutdownTimeout,
		}),
	}, nil
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

func queueWeights(cfg config.QueueConfig) map[string]int {
	queues := make(map[string]int, 3)
	if cfg.CriticalWeight > 0 {
		queues[QueueCritical] = cfg.CriticalWeight
	}
	if cfg.DefaultWeight > 0 {
		queues[QueueDefault] = cfg.DefaultWeight
	}
	if cfg.LowWeight > 0 {
		queues[QueueLow] = cfg.LowWeight
	}
	return queues
}
