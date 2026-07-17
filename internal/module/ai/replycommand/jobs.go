package replycommand

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/shared/apperror"
)

const TypeReplyCommandV1 = "ai:reply-command:v1"

type WakePayload struct {
	CommandID uint64 `json:"command_id"`
}

type JobRunner interface {
	RunCommand(context.Context, uint64) (bool, error)
}

func NewWakeTask(commandID uint64) (taskqueue.Task, error) {
	if commandID == 0 {
		return taskqueue.Task{}, errors.New("reply command id is required")
	}
	payload, err := json.Marshal(WakePayload{CommandID: commandID})
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeReplyCommandV1, err)
	}
	return taskqueue.Task{Type: TypeReplyCommandV1, Payload: payload}, nil
}

func RegisterTaskDefinition(registry *taskqueue.Registry, runner JobRunner) error {
	if registry == nil {
		return taskqueue.ErrRegistryRequired
	}
	return registry.Register(taskqueue.Definition{
		Type:      TypeReplyCommandV1,
		Queue:     taskqueue.QueueDefault,
		Timeout:   15 * time.Minute,
		MaxRetry:  0,
		UniqueTTL: time.Second,
		Decode: func(data []byte) (any, *apperror.Error) {
			var payload WakePayload
			if err := json.Unmarshal(data, &payload); err != nil {
				return nil, taskqueue.PayloadError(TypeReplyCommandV1, err)
			}
			if payload.CommandID == 0 {
				return nil, taskqueue.PayloadError(TypeReplyCommandV1, errors.New("command_id is required"))
			}
			return payload, nil
		},
		Handle: func(ctx context.Context, decoded any) *apperror.Error {
			if runner == nil {
				return taskqueue.InvariantError(TypeReplyCommandV1, ErrRunnerNotReady)
			}
			payload, ok := decoded.(WakePayload)
			if !ok {
				return taskqueue.InvariantError(TypeReplyCommandV1, fmt.Errorf("decoded payload type %T", decoded))
			}
			if _, err := runner.RunCommand(ctx, payload.CommandID); err != nil {
				return taskqueue.HandlerError(TypeReplyCommandV1, err)
			}
			return nil
		},
	})
}

type WakeupEnqueuer struct {
	queue taskqueue.Enqueuer
}

func NewWakeupEnqueuer(queue taskqueue.Enqueuer) *WakeupEnqueuer {
	return &WakeupEnqueuer{queue: queue}
}

func (e *WakeupEnqueuer) WakeReply(ctx context.Context, commandID uint64) error {
	if e == nil || e.queue == nil {
		return taskqueue.ErrClientNotReady
	}
	task, err := NewWakeTask(commandID)
	if err != nil {
		return err
	}
	_, err = e.queue.Enqueue(ctx, task)
	return err
}
