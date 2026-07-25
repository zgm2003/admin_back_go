package aichat

import (
	"context"
	"errors"

	infraai "admin_back_go/internal/infra/ai"
)

type drainSink struct {
	deliveryCtx context.Context
	downstream  infraai.EventSink
}

func newDrainSink(deliveryCtx context.Context, downstream infraai.EventSink) infraai.EventSink {
	return drainSink{deliveryCtx: deliveryCtx, downstream: downstream}
}

func (s drainSink) Emit(_ context.Context, event infraai.Event) error {
	if deliveryStopped(s.deliveryCtx) || s.downstream == nil {
		return nil
	}
	err := s.downstream.Emit(s.deliveryCtx, event)
	if deliveryStopped(s.deliveryCtx) {
		return nil
	}
	return err
}

func deliveryStopped(ctx context.Context) bool {
	return ctx != nil && errors.Is(context.Cause(ctx), infraai.ErrCanceled)
}
