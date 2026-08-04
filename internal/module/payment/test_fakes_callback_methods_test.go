package payment

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

func (r *fakeConfigRepo) CreateCallbackEvent(ctx context.Context, event CallbackEvent) (int64, error) {
	return 0, nil
}

func (r *fakeConfigRepo) UpdateCallbackEventProcessed(ctx context.Context, id int64, signatureValid int, status string, message string, processedAt time.Time) error {
	return nil
}

func (r *fakeOrderRepo) CreateCallbackEvent(ctx context.Context, event CallbackEvent) (int64, error) {
	return 0, nil
}

func (r *fakeOrderRepo) UpdateCallbackEventProcessed(ctx context.Context, id int64, signatureValid int, status string, message string, processedAt time.Time) error {
	return nil
}

func (r *fakeRechargeRepo) CreateCallbackEvent(ctx context.Context, event CallbackEvent) (int64, error) {
	if r.callbackCreateErr != nil {
		return 0, r.callbackCreateErr
	}
	if r.rejectInvalidCallbackJSON && !json.Valid([]byte(event.RawPayloadJSON)) {
		return 0, errors.New("invalid callback json")
	}
	if r.callbackEventExists {
		return r.callbackEvent.ID, nil
	}
	event.ID = 1
	r.callbackEvent = event
	r.callbackEventExists = true
	r.callbackCreateCount++
	return event.ID, nil
}

func (r *fakeRechargeRepo) UpdateCallbackEventProcessed(ctx context.Context, id int64, signatureValid int, status string, message string, processedAt time.Time) error {
	if r.callbackResolveErr != nil {
		return r.callbackResolveErr
	}
	r.callbackEvent.SignatureValid = signatureValid
	r.callbackEvent.ProcessStatus = status
	r.callbackEvent.ProcessMessage = message
	r.callbackEvent.ProcessedAt = &processedAt
	return nil
}

func (r *fakeRechargeRepo) AcquireCallbackEvent(ctx context.Context, event CallbackEvent) (*CallbackEvent, bool, error) {
	if r.callbackCreateErr != nil {
		return nil, false, r.callbackCreateErr
	}
	if r.rejectInvalidCallbackJSON && !json.Valid([]byte(event.RawPayloadJSON)) {
		return nil, false, errors.New("invalid callback json")
	}
	if r.callbackEventExists {
		copy := r.callbackEvent
		return &copy, false, nil
	}
	event.ID = 1
	r.callbackEvent = event
	r.callbackEventExists = true
	r.callbackCreateCount++
	copy := event
	return &copy, true, nil
}

func (r *fakeRechargeRepo) ResolveCallbackEvent(ctx context.Context, resolution CallbackEventResolution) (*CallbackEventResolutionResult, error) {
	if r.callbackResolveErr != nil {
		return nil, r.callbackResolveErr
	}
	if r.callbackEvent.ProcessStatus == callbackProcessSuccess || r.callbackEvent.ProcessStatus == callbackProcessIgnored {
		copy := r.callbackEvent
		return &CallbackEventResolutionResult{Event: &copy, Replay: true}, nil
	}
	var paidOrder *PaidOrderFinalization
	if resolution.PaidOrderID > 0 {
		fact, err := r.FinalizePaidOrder(ctx, resolution.PaidOrderID, resolution.AlipayTradeNo, resolution.PaidAt, resolution.ProcessedAt)
		if err != nil {
			return nil, err
		}
		paidOrder = fact
	}
	if err := r.UpdateCallbackEventProcessed(
		ctx,
		resolution.EventID,
		resolution.SignatureValid,
		resolution.ProcessStatus,
		resolution.ProcessMessage,
		resolution.ProcessedAt,
	); err != nil {
		return nil, err
	}
	copy := r.callbackEvent
	return &CallbackEventResolutionResult{Event: &copy, PaidOrder: paidOrder}, nil
}
