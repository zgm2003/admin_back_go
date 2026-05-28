package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"admin_back_go/internal/infra/taskqueue"
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
	return taskqueue.Task{Type: TypeSyncPendingOrderV1, Payload: data, Queue: taskqueue.QueueDefault, UniqueTTL: 55 * time.Second}, nil
}

func NewCloseExpiredOrderTask(payload CloseExpiredOrderPayload) (taskqueue.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeCloseExpiredOrderV1, err)
	}
	return taskqueue.Task{Type: TypeCloseExpiredOrderV1, Payload: data, Queue: taskqueue.QueueDefault, UniqueTTL: 55 * time.Second}, nil
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

func RegisterHandlers(mux *taskqueue.Mux, service JobService, logger *slog.Logger) {
	if mux == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	mux.HandleFunc(TypeSyncPendingOrderV1, func(ctx context.Context, task taskqueue.Task) error {
		if service == nil {
			return ErrRepositoryNotConfigured
		}
		payload, err := DecodeSyncPendingOrderPayload(task.Payload)
		if err != nil {
			return err
		}
		result, err := service.SyncPendingOrders(ctx, SyncPendingOrderInput{Limit: payload.Limit})
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "processed payment sync pending orders task", "scanned", result.Scanned, "paid", result.Paid, "closed", result.Closed, "waiting", result.Waiting, "failed", result.Failed)
		return nil
	})
	mux.HandleFunc(TypeCloseExpiredOrderV1, func(ctx context.Context, task taskqueue.Task) error {
		if service == nil {
			return ErrRepositoryNotConfigured
		}
		payload, err := DecodeCloseExpiredOrderPayload(task.Payload)
		if err != nil {
			return err
		}
		result, err := service.CloseExpiredOrders(ctx, CloseExpiredOrderInput{Limit: payload.Limit})
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "processed payment close expired orders task", "scanned", result.Scanned, "paid", result.Paid, "closed", result.Closed, "waiting", result.Waiting, "failed", result.Failed)
		return nil
	})
}
