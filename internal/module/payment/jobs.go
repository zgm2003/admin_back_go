package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"admin_back_go/internal/platform/taskqueue"
)

const (
	TypeCloseExpiredOrderV1 = "payment:close-expired-order:v1"
	TypeSyncPendingOrderV1  = "payment:sync-pending-order:v1"
)

type CloseExpiredPayload struct {
	Limit int `json:"limit,omitempty"`
}

type SyncPendingPayload struct {
	Limit int `json:"limit,omitempty"`
}

type JobService interface {
	CloseExpiredOrders(ctx context.Context, input CloseExpiredInput) (*JobResult, error)
	SyncPendingOrders(ctx context.Context, input SyncPendingInput) (*JobResult, error)
}

func NewCloseExpiredOrderTask(payload CloseExpiredPayload) (taskqueue.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeCloseExpiredOrderV1, err)
	}
	return taskqueue.Task{
		Type:      TypeCloseExpiredOrderV1,
		Payload:   data,
		Queue:     taskqueue.QueueDefault,
		UniqueTTL: 55 * time.Second,
	}, nil
}

func NewSyncPendingOrderTask(payload SyncPendingPayload) (taskqueue.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeSyncPendingOrderV1, err)
	}
	return taskqueue.Task{
		Type:      TypeSyncPendingOrderV1,
		Payload:   data,
		Queue:     taskqueue.QueueDefault,
		UniqueTTL: 4*time.Minute + 55*time.Second,
	}, nil
}

func DecodeCloseExpiredPayload(payload []byte) (CloseExpiredPayload, error) {
	var row CloseExpiredPayload
	if len(payload) == 0 {
		return row, nil
	}
	if err := json.Unmarshal(payload, &row); err != nil {
		return CloseExpiredPayload{}, fmt.Errorf("decode %s payload: %w", TypeCloseExpiredOrderV1, err)
	}
	return row, nil
}

func DecodeSyncPendingPayload(payload []byte) (SyncPendingPayload, error) {
	var row SyncPendingPayload
	if len(payload) == 0 {
		return row, nil
	}
	if err := json.Unmarshal(payload, &row); err != nil {
		return SyncPendingPayload{}, fmt.Errorf("decode %s payload: %w", TypeSyncPendingOrderV1, err)
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

	mux.HandleFunc(TypeCloseExpiredOrderV1, func(ctx context.Context, task taskqueue.Task) error {
		if service == nil {
			return ErrRepositoryNotConfigured
		}
		payload, err := DecodeCloseExpiredPayload(task.Payload)
		if err != nil {
			return err
		}
		result, err := service.CloseExpiredOrders(ctx, CloseExpiredInput{Limit: payload.Limit})
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "processed payment close expired order task", "scanned", result.Scanned, "closed", result.Closed, "paid", result.Paid, "deferred", result.Deferred, "skipped", result.Skipped)
		return nil
	})

	mux.HandleFunc(TypeSyncPendingOrderV1, func(ctx context.Context, task taskqueue.Task) error {
		if service == nil {
			return ErrRepositoryNotConfigured
		}
		payload, err := DecodeSyncPendingPayload(task.Payload)
		if err != nil {
			return err
		}
		result, err := service.SyncPendingOrders(ctx, SyncPendingInput{Limit: payload.Limit})
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "processed payment sync pending order task", "scanned", result.Scanned, "closed", result.Closed, "paid", result.Paid, "deferred", result.Deferred, "skipped", result.Skipped)
		return nil
	})
}
