package taskqueue

import (
	"context"

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
	mux *asynq.ServeMux
}

// NewMux creates an empty task handler registry.
func NewMux() *Mux {
	return &Mux{mux: asynq.NewServeMux()}
}

// HandleFunc registers a handler for one task type or type prefix.
func (m *Mux) HandleFunc(pattern string, handler HandlerFunc) {
	m.mux.HandleFunc(pattern, func(ctx context.Context, task *asynq.Task) error {
		return handler(ctx, Task{
			Type:    task.Type(),
			Payload: task.Payload(),
		})
	})
}

// ProcessTask runs an Asynq task through the mux. It is mainly useful in tests.
func (m *Mux) ProcessTask(ctx context.Context, task *asynq.Task) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return m.mux.ProcessTask(ctx, task)
}

// ProcessProjectTask runs a project task through the mux without hitting Redis.
func (m *Mux) ProcessProjectTask(ctx context.Context, task Task) error {
	return m.ProcessTask(ctx, asynq.NewTask(task.Type, task.Payload))
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
	return s.server.Start(mux.mux)
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
