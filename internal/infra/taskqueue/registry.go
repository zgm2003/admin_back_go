package taskqueue

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"admin_back_go/internal/shared/apperror"

	"github.com/hibiken/asynq"
)

var (
	ErrTaskTypeUnversioned   = errors.New("task type must end with a positive version")
	ErrTaskTypeDuplicate     = errors.New("task type is already registered")
	ErrTaskTypeNotRegistered = errors.New("task type is not registered")
	ErrQueueUnknown          = errors.New("task queue is unknown")
	ErrTaskTimeoutRequired   = errors.New("task timeout must be positive")
	ErrTaskMaxRetryInvalid   = errors.New("task max retry cannot be negative")
	ErrTaskUniqueTTLInvalid  = errors.New("task unique ttl cannot be negative")
	ErrTaskDecodeRequired    = errors.New("task decoder is required")
	ErrTaskHandleRequired    = errors.New("task handler is required")
)

var versionedTaskType = regexp.MustCompile(`^[a-z0-9]+(?::[a-z0-9][a-z0-9-]*)*:v[1-9][0-9]*$`)

// Definition is the single executable contract for one versioned task type.
// Producers and consumers share its queue, retry, timeout, uniqueness, payload,
// and handler policy instead of duplicating Asynq options across modules.
type Definition struct {
	Type              string
	Queue             string
	Timeout           time.Duration
	MaxRetry          int
	UniqueTTL         time.Duration
	Decode            func([]byte) (any, *apperror.Error)
	Handle            func(context.Context, any) *apperror.Error
	FinalizeExhausted ExhaustedFinalizer
}

// Attempt is the one-based execution number derived from Asynq's zero-based
// retry count. Limit includes the initial execution.
type Attempt struct {
	Number int
	Limit  int
}

type ExhaustedFinalizer func(context.Context, any, *apperror.Error, Attempt) *apperror.Error

// Policy is the immutable producer-facing part of a task definition.
type Policy struct {
	Queue     string
	Timeout   time.Duration
	MaxRetry  int
	UniqueTTL time.Duration
}

// Registry owns every executable queue task definition in one Worker graph.
type Registry struct {
	mu          sync.RWMutex
	definitions map[string]Definition
}

func NewRegistry() *Registry {
	return &Registry{definitions: make(map[string]Definition)}
}

// Register adds one complete versioned task definition.
func (r *Registry) Register(definition Definition) error {
	if r == nil {
		return ErrClientNotReady
	}
	definition.Type = strings.TrimSpace(definition.Type)
	definition.Queue = strings.TrimSpace(definition.Queue)
	if definition.Type == "" {
		return ErrTaskTypeRequired
	}
	if !versionedTaskType.MatchString(definition.Type) {
		return fmt.Errorf("%w: %s", ErrTaskTypeUnversioned, definition.Type)
	}
	if !knownQueue(definition.Queue) {
		return fmt.Errorf("%w: %s", ErrQueueUnknown, definition.Queue)
	}
	if definition.Timeout <= 0 {
		return ErrTaskTimeoutRequired
	}
	if definition.MaxRetry < 0 {
		return ErrTaskMaxRetryInvalid
	}
	if definition.UniqueTTL < 0 {
		return ErrTaskUniqueTTLInvalid
	}
	if definition.Decode == nil {
		return ErrTaskDecodeRequired
	}
	if definition.Handle == nil {
		return ErrTaskHandleRequired
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.definitions == nil {
		r.definitions = make(map[string]Definition)
	}
	if _, exists := r.definitions[definition.Type]; exists {
		return fmt.Errorf("%w: %s", ErrTaskTypeDuplicate, definition.Type)
	}
	r.definitions[definition.Type] = definition
	return nil
}

// Task validates that a producer uses a registered type and returns the
// project task plus the exact immutable delivery policy.
func (r *Registry) Task(taskType string, payload []byte) (Task, Policy, error) {
	definition, ok := r.definition(taskType)
	if !ok {
		return Task{}, Policy{}, fmt.Errorf("%w: %s", ErrTaskTypeNotRegistered, strings.TrimSpace(taskType))
	}
	return Task{Type: definition.Type, Payload: append([]byte(nil), payload...)}, policyOf(definition), nil
}

// Handle decodes and executes a task, translating permanent application
// failures to Asynq's SkipRetry marker while preserving the stable error.
func (r *Registry) Handle(ctx context.Context, task Task) error {
	definition, ok := r.definition(task.Type)
	if !ok {
		appErr := apperror.New(
			"task.type_unregistered",
			apperror.CategoryValidation,
			http.StatusBadRequest,
			apperror.Permanent,
			"",
			nil,
			"task type is not registered",
		)
		return errors.Join(appErr, asynq.SkipRetry)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	payload, appErr := definition.Decode(append([]byte(nil), task.Payload...))
	if appErr == nil {
		appErr = definition.Handle(ctx, payload)
	}
	if appErr == nil {
		return nil
	}
	if appErr.Retryable() {
		if definition.FinalizeExhausted != nil && task.MaxRetry == definition.MaxRetry && task.Retry >= task.MaxRetry {
			attempt := Attempt{Number: task.Retry + 1, Limit: task.MaxRetry + 1}
			if finalizerErr := definition.FinalizeExhausted(ctx, payload, appErr, attempt); finalizerErr != nil {
				return finalizerErr
			}
			return errors.Join(appErr, asynq.SkipRetry)
		}
		return appErr
	}
	return errors.Join(appErr, asynq.SkipRetry)
}

// Types returns registered task types in deterministic order.
func (r *Registry) Types() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	types := make([]string, 0, len(r.definitions))
	for taskType := range r.definitions {
		types = append(types, taskType)
	}
	r.mu.RUnlock()
	sort.Strings(types)
	return types
}

func (r *Registry) definition(taskType string) (Definition, bool) {
	if r == nil {
		return Definition{}, false
	}
	r.mu.RLock()
	definition, ok := r.definitions[strings.TrimSpace(taskType)]
	r.mu.RUnlock()
	return definition, ok
}

func policyOf(definition Definition) Policy {
	return Policy{
		Queue:     definition.Queue,
		Timeout:   definition.Timeout,
		MaxRetry:  definition.MaxRetry,
		UniqueTTL: definition.UniqueTTL,
	}
}

func knownQueue(queue string) bool {
	switch queue {
	case QueueCritical, QueueDefault, QueueLow:
		return true
	default:
		return false
	}
}

// PayloadError classifies an invalid serialized payload as a permanent task
// failure while preserving the internal cause for diagnostics.
func PayloadError(taskType string, cause error) *apperror.Error {
	return apperror.Wrap(
		"task.payload_invalid",
		apperror.CategoryValidation,
		http.StatusBadRequest,
		apperror.Permanent,
		"",
		nil,
		"task payload is invalid",
		cause,
	).WithOperation(strings.TrimSpace(taskType) + ".decode")
}

// InvariantError classifies a missing executable dependency or impossible
// decoded payload as permanent so retrying cannot hide a broken Worker graph.
func InvariantError(operation string, cause error) *apperror.Error {
	return apperror.Wrap(
		"task.invariant_failed",
		apperror.CategoryInternal,
		http.StatusInternalServerError,
		apperror.Permanent,
		"",
		nil,
		"task invariant failed",
		cause,
	).WithOperation(operation)
}

// HandlerError preserves an explicitly classified application error. Unknown
// handler failures become bounded retryable internal errors under the
// definition's MaxRetry policy.
func HandlerError(operation string, cause error) *apperror.Error {
	if cause == nil {
		return nil
	}
	var appErr *apperror.Error
	if errors.As(cause, &appErr) {
		return appErr.WithOperation(operation)
	}
	category := apperror.CategoryInternal
	httpStatus := http.StatusInternalServerError
	code := "task.handler_failed"
	message := "task handler failed"
	if errors.Is(cause, context.DeadlineExceeded) {
		category = apperror.CategoryTimeout
		httpStatus = http.StatusGatewayTimeout
		code = "task.handler_timeout"
		message = "task handler timed out"
	}
	return apperror.Wrap(
		code,
		category,
		httpStatus,
		apperror.Retryable,
		"",
		nil,
		message,
		cause,
	).WithOperation(operation)
}
