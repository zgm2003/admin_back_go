package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/shared/apperror"
)

const (
	TypeSyncPendingOrderV1  = "payment:sync-pending-order:v1"
	TypeCloseExpiredOrderV1 = "payment:close-expired-order:v1"
)

type SyncPendingOrderPayload struct {
	Limit int `json:"limit,omitempty"`
}

type CloseExpiredOrderPayload struct {
	Limit int `json:"limit,omitempty"`
}

type JobService interface {
	SyncPendingOrders(ctx context.Context, input SyncPendingOrderInput) (*SyncPendingOrderResult, error)
	CloseExpiredOrders(ctx context.Context, input CloseExpiredOrderInput) (*CloseExpiredOrderResult, error)
}

func NewSyncPendingOrderTask(payload SyncPendingOrderPayload) (taskqueue.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeSyncPendingOrderV1, err)
	}
	return taskqueue.Task{Type: TypeSyncPendingOrderV1, Payload: data}, nil
}

func NewCloseExpiredOrderTask(payload CloseExpiredOrderPayload) (taskqueue.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeCloseExpiredOrderV1, err)
	}
	return taskqueue.Task{Type: TypeCloseExpiredOrderV1, Payload: data}, nil
}

func DecodeSyncPendingOrderPayload(payload []byte) (SyncPendingOrderPayload, error) {
	var row SyncPendingOrderPayload
	if len(payload) == 0 {
		return row, nil
	}
	if err := json.Unmarshal(payload, &row); err != nil {
		return SyncPendingOrderPayload{}, fmt.Errorf("decode %s payload: %w", TypeSyncPendingOrderV1, err)
	}
	return row, nil
}

func DecodeCloseExpiredOrderPayload(payload []byte) (CloseExpiredOrderPayload, error) {
	var row CloseExpiredOrderPayload
	if len(payload) == 0 {
		return row, nil
	}
	if err := json.Unmarshal(payload, &row); err != nil {
		return CloseExpiredOrderPayload{}, fmt.Errorf("decode %s payload: %w", TypeCloseExpiredOrderV1, err)
	}
	return row, nil
}

func RegisterTaskDefinitions(registry *taskqueue.Registry, service JobService, logger *slog.Logger) error {
	if registry == nil {
		return taskqueue.ErrRegistryRequired
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := registry.Register(paymentDefinition(
		TypeSyncPendingOrderV1,
		func(data []byte) (any, error) { return DecodeSyncPendingOrderPayload(data) },
		func(ctx context.Context, decoded any) *apperror.Error {
			if service == nil {
				return taskqueue.InvariantError(TypeSyncPendingOrderV1, ErrRepositoryNotConfigured)
			}
			payload, ok := decoded.(SyncPendingOrderPayload)
			if !ok {
				return taskqueue.InvariantError(TypeSyncPendingOrderV1, fmt.Errorf("decoded payload type %T", decoded))
			}
			result, err := service.SyncPendingOrders(ctx, SyncPendingOrderInput(payload))
			if err != nil {
				return taskqueue.HandlerError(TypeSyncPendingOrderV1, err)
			}
			if result == nil {
				return taskqueue.InvariantError(TypeSyncPendingOrderV1, fmt.Errorf("nil sync result"))
			}
			logger.InfoContext(ctx, "processed payment sync pending orders task", "scanned", result.Scanned, "paid", result.Paid, "closed", result.Closed, "waiting", result.Waiting, "failed", result.Failed)
			return nil
		},
	)); err != nil {
		return err
	}
	return registry.Register(paymentDefinition(
		TypeCloseExpiredOrderV1,
		func(data []byte) (any, error) { return DecodeCloseExpiredOrderPayload(data) },
		func(ctx context.Context, decoded any) *apperror.Error {
			if service == nil {
				return taskqueue.InvariantError(TypeCloseExpiredOrderV1, ErrRepositoryNotConfigured)
			}
			payload, ok := decoded.(CloseExpiredOrderPayload)
			if !ok {
				return taskqueue.InvariantError(TypeCloseExpiredOrderV1, fmt.Errorf("decoded payload type %T", decoded))
			}
			result, err := service.CloseExpiredOrders(ctx, CloseExpiredOrderInput(payload))
			if err != nil {
				return taskqueue.HandlerError(TypeCloseExpiredOrderV1, err)
			}
			if result == nil {
				return taskqueue.InvariantError(TypeCloseExpiredOrderV1, fmt.Errorf("nil close result"))
			}
			logger.InfoContext(ctx, "processed payment close expired orders task", "scanned", result.Scanned, "paid", result.Paid, "closed", result.Closed, "waiting", result.Waiting, "failed", result.Failed)
			return nil
		},
	))
}

func paymentDefinition(taskType string, decode func([]byte) (any, error), handle func(context.Context, any) *apperror.Error) taskqueue.Definition {
	return taskqueue.Definition{
		Type:      taskType,
		Queue:     taskqueue.QueueDefault,
		Timeout:   taskqueue.DefaultTimeout,
		MaxRetry:  taskqueue.DefaultMaxRetry,
		UniqueTTL: 55 * time.Second,
		Decode: func(data []byte) (any, *apperror.Error) {
			payload, err := decode(data)
			if err != nil {
				return nil, taskqueue.PayloadError(taskType, err)
			}
			return payload, nil
		},
		Handle: func(ctx context.Context, payload any) *apperror.Error {
			return handle(ctx, payload)
		},
	}
}

func RegisterHandlers(mux *taskqueue.Mux, service JobService, logger *slog.Logger) {
	registry := taskqueue.NewRegistry()
	if err := RegisterTaskDefinitions(registry, service, logger); err != nil {
		panic(err)
	}
	if err := mux.RegisterRegistry(registry); err != nil {
		panic(err)
	}
}
