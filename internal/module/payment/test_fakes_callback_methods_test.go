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
	event.ID = 1
	r.callbackEvent = event
	return event.ID, nil
}

func (r *fakeRechargeRepo) UpdateCallbackEventProcessed(ctx context.Context, id int64, signatureValid int, status string, message string, processedAt time.Time) error {
	r.callbackEvent.SignatureValid = signatureValid
	r.callbackEvent.ProcessStatus = status
	r.callbackEvent.ProcessMessage = message
	r.callbackEvent.ProcessedAt = &processedAt
	return nil
}
