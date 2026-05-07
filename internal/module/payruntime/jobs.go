package payruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"admin_back_go/internal/platform/taskqueue"
)

const (
	TypeCloseExpiredOrderV1      = "pay:close-expired-order:v1"
	TypeSyncPendingTransactionV1 = "pay:sync-pending-transaction:v1"
	TypeFulfillmentRetryV1       = "pay:fulfillment-retry:v1"
)

type CloseExpiredOrderPayload struct {
	Limit int `json:"limit,omitempty"`
}

type SyncPendingTransactionPayload struct {
	Limit int `json:"limit,omitempty"`
}

type FulfillmentRetryPayload struct {
	Limit int `json:"limit,omitempty"`
}

type JobService interface {
	CloseExpiredOrders(ctx context.Context, input CloseExpiredOrderInput) (*CloseExpiredOrderResult, error)
	SyncPendingTransactions(ctx context.Context, input SyncPendingTransactionInput) (*SyncPendingTransactionResult, error)
	RetryFailedFulfillments(ctx context.Context, input FulfillmentRetryInput) (*FulfillmentRetryResult, error)
}

func NewCloseExpiredOrderTask(payload CloseExpiredOrderPayload) (taskqueue.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeCloseExpiredOrderV1, err)
	}
	return taskqueue.Task{Type: TypeCloseExpiredOrderV1, Payload: data, Queue: taskqueue.QueueDefault, UniqueTTL: 55 * time.Second}, nil
}

func NewSyncPendingTransactionTask(payload SyncPendingTransactionPayload) (taskqueue.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeSyncPendingTransactionV1, err)
	}
	return taskqueue.Task{Type: TypeSyncPendingTransactionV1, Payload: data, Queue: taskqueue.QueueDefault, UniqueTTL: 4*time.Minute + 55*time.Second}, nil
}

func NewFulfillmentRetryTask(payload FulfillmentRetryPayload) (taskqueue.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeFulfillmentRetryV1, err)
	}
	return taskqueue.Task{Type: TypeFulfillmentRetryV1, Payload: data, Queue: taskqueue.QueueDefault, UniqueTTL: 2*time.Minute + 55*time.Second}, nil
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

func DecodeSyncPendingTransactionPayload(payload []byte) (SyncPendingTransactionPayload, error) {
	var row SyncPendingTransactionPayload
	if len(payload) == 0 {
		return row, nil
	}
	if err := json.Unmarshal(payload, &row); err != nil {
		return SyncPendingTransactionPayload{}, fmt.Errorf("decode %s payload: %w", TypeSyncPendingTransactionV1, err)
	}
	return row, nil
}

func DecodeFulfillmentRetryPayload(payload []byte) (FulfillmentRetryPayload, error) {
	var row FulfillmentRetryPayload
	if len(payload) == 0 {
		return row, nil
	}
	if err := json.Unmarshal(payload, &row); err != nil {
		return FulfillmentRetryPayload{}, fmt.Errorf("decode %s payload: %w", TypeFulfillmentRetryV1, err)
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
		payload, err := DecodeCloseExpiredOrderPayload(task.Payload)
		if err != nil {
			return err
		}
		result, err := service.CloseExpiredOrders(ctx, CloseExpiredOrderInput{Limit: payload.Limit})
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "processed pay close expired order task", "scanned", result.Scanned, "closed", result.Closed, "paid", result.Paid, "deferred", result.Deferred, "skipped", result.Skipped)
		return nil
	})

	mux.HandleFunc(TypeSyncPendingTransactionV1, func(ctx context.Context, task taskqueue.Task) error {
		if service == nil {
			return ErrRepositoryNotConfigured
		}
		payload, err := DecodeSyncPendingTransactionPayload(task.Payload)
		if err != nil {
			return err
		}
		result, err := service.SyncPendingTransactions(ctx, SyncPendingTransactionInput{Limit: payload.Limit})
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "processed pay sync pending transaction task", "scanned", result.Scanned, "paid", result.Paid, "unpaid", result.Unpaid, "deferred", result.Deferred, "skipped", result.Skipped)
		return nil
	})

	mux.HandleFunc(TypeFulfillmentRetryV1, func(ctx context.Context, task taskqueue.Task) error {
		if service == nil {
			return ErrRepositoryNotConfigured
		}
		payload, err := DecodeFulfillmentRetryPayload(task.Payload)
		if err != nil {
			return err
		}
		result, err := service.RetryFailedFulfillments(ctx, FulfillmentRetryInput{Limit: payload.Limit})
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "processed pay fulfillment retry task", "scanned", result.Scanned, "retried", result.Retried, "success", result.Success, "failed", result.Failed, "skipped", result.Skipped)
		return nil
	})
}
