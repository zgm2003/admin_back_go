package taskqueue

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"admin_back_go/internal/shared/apperror"

	"github.com/hibiken/asynq"
)

func TestRegistryMapsPermanentFailureToSkipRetry(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(Definition{
		Type:      "widget:run:v1",
		Queue:     QueueDefault,
		Timeout:   time.Minute,
		MaxRetry:  3,
		UniqueTTL: time.Minute,
		Decode: func([]byte) (any, *apperror.Error) {
			return nil, apperror.BadRequest("bad payload").WithCode("task.payload_invalid")
		},
		Handle: func(context.Context, any) *apperror.Error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	got := registry.Handle(context.Background(), Task{Type: "widget:run:v1", Payload: []byte("{")})
	if !errors.Is(got, asynq.SkipRetry) {
		t.Fatalf("err=%v", got)
	}
	var appErr *apperror.Error
	if !errors.As(got, &appErr) || appErr.Code != "task.payload_invalid" {
		t.Fatalf("expected stable payload error, got %v", got)
	}
}

func TestRegistryKeepsRetryableFailureEligibleForRetry(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(Definition{
		Type:     "widget:run:v1",
		Queue:    QueueDefault,
		Timeout:  time.Minute,
		MaxRetry: 3,
		Decode:   func([]byte) (any, *apperror.Error) { return "decoded", nil },
		Handle: func(context.Context, any) *apperror.Error {
			return apperror.New(
				"dependency.widget",
				apperror.CategoryDependency,
				http.StatusServiceUnavailable,
				apperror.Retryable,
				"",
				nil,
				"widget unavailable",
			)
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := registry.Handle(context.Background(), Task{Type: "widget:run:v1"})
	if got == nil || errors.Is(got, asynq.SkipRetry) {
		t.Fatalf("expected retryable application error, got %v", got)
	}
	var appErr *apperror.Error
	if !errors.As(got, &appErr) || !appErr.Retryable() {
		t.Fatalf("expected retryable application error, got %v", got)
	}
}

func TestRegistryRejectsDuplicateAndInvalidDefinitions(t *testing.T) {
	valid := Definition{
		Type:     "widget:run:v1",
		Queue:    QueueDefault,
		Timeout:  time.Minute,
		MaxRetry: 3,
		Decode:   func([]byte) (any, *apperror.Error) { return nil, nil },
		Handle:   func(context.Context, any) *apperror.Error { return nil },
	}

	tests := []struct {
		name       string
		definition Definition
		want       error
	}{
		{name: "empty type", definition: withDefinition(valid, func(d *Definition) { d.Type = "" }), want: ErrTaskTypeRequired},
		{name: "unversioned type", definition: withDefinition(valid, func(d *Definition) { d.Type = "widget:run" }), want: ErrTaskTypeUnversioned},
		{name: "unknown queue", definition: withDefinition(valid, func(d *Definition) { d.Queue = "bulk" }), want: ErrQueueUnknown},
		{name: "zero timeout", definition: withDefinition(valid, func(d *Definition) { d.Timeout = 0 }), want: ErrTaskTimeoutRequired},
		{name: "negative retry", definition: withDefinition(valid, func(d *Definition) { d.MaxRetry = -1 }), want: ErrTaskMaxRetryInvalid},
		{name: "negative unique ttl", definition: withDefinition(valid, func(d *Definition) { d.UniqueTTL = -time.Second }), want: ErrTaskUniqueTTLInvalid},
		{name: "nil decode", definition: withDefinition(valid, func(d *Definition) { d.Decode = nil }), want: ErrTaskDecodeRequired},
		{name: "nil handle", definition: withDefinition(valid, func(d *Definition) { d.Handle = nil }), want: ErrTaskHandleRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			err := registry.Register(tt.definition)
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}

	registry := NewRegistry()
	if err := registry.Register(valid); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(valid); !errors.Is(err, ErrTaskTypeDuplicate) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestRegistryTaskAppliesRegisteredPolicy(t *testing.T) {
	registry := NewRegistry()
	definition := Definition{
		Type:      "widget:run:v1",
		Queue:     QueueLow,
		Timeout:   15 * time.Second,
		MaxRetry:  7,
		UniqueTTL: time.Minute,
		Decode:    func([]byte) (any, *apperror.Error) { return nil, nil },
		Handle:    func(context.Context, any) *apperror.Error { return nil },
	}
	if err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}

	task, policy, err := registry.Task("widget:run:v1", []byte(`{"id":7}`))
	if err != nil {
		t.Fatal(err)
	}
	if task.Type != definition.Type || string(task.Payload) != `{"id":7}` {
		t.Fatalf("unexpected task: %+v", task)
	}
	if policy.Queue != QueueLow || policy.MaxRetry != 7 || policy.Timeout != 15*time.Second || policy.UniqueTTL != time.Minute {
		t.Fatalf("unexpected policy: %+v", policy)
	}
	if _, _, err := registry.Task("widget:missing:v1", nil); !errors.Is(err, ErrTaskTypeNotRegistered) {
		t.Fatalf("expected unknown type rejection, got %v", err)
	}
}

func TestTaskErrorsClassifyPayloadInvariantAndDependencyFailures(t *testing.T) {
	payloadCause := errors.New("private malformed body")
	payloadErr := PayloadError("widget:run:v1", payloadCause)
	if payloadErr.Code != "task.payload_invalid" || payloadErr.Retryable() || !errors.Is(payloadErr, payloadCause) {
		t.Fatalf("unexpected payload classification: %+v", payloadErr)
	}
	if payloadErr.Error() == payloadCause.Error() {
		t.Fatalf("payload cause must not become the public queue error message")
	}

	invariantCause := errors.New("repository missing")
	invariantErr := InvariantError("widget.run", invariantCause)
	if invariantErr.Code != "task.invariant_failed" || invariantErr.Retryable() || !errors.Is(invariantErr, invariantCause) {
		t.Fatalf("unexpected invariant classification: %+v", invariantErr)
	}

	dependencyCause := errors.New("redis down")
	dependencyErr := HandlerError("widget.run", dependencyCause)
	if dependencyErr.Code != "task.handler_failed" || !dependencyErr.Retryable() || !errors.Is(dependencyErr, dependencyCause) {
		t.Fatalf("unexpected handler classification: %+v", dependencyErr)
	}

	declared := apperror.BadRequest("declared").WithCode("widget.invalid")
	if got := HandlerError("widget.run", declared); got.Code != "widget.invalid" || got.Retryable() {
		t.Fatalf("expected declared application classification to survive, got %+v", got)
	}
}

func withDefinition(definition Definition, change func(*Definition)) Definition {
	change(&definition)
	return definition
}
