package aichat

import (
	"time"

	infrarealtime "admin_back_go/internal/infra/realtime"
	modulerealtime "admin_back_go/internal/module/realtime"
)

const (
	EventAIResponseStart     = modulerealtime.TypeAIResponseStartV1
	EventAIResponseDelta     = modulerealtime.TypeAIResponseDeltaV2
	EventAIResponseCompleted = modulerealtime.TypeAIResponseCompletedV1
	EventAIResponseFailed    = modulerealtime.TypeAIResponseFailedV1
	EventAIResponseCanceled  = modulerealtime.TypeAIResponseCanceledV2
)

type StartPayload = modulerealtime.AIResponseStartPayload
type DeltaPayload = modulerealtime.AIResponseDeltaPayload
type CanceledPayload = modulerealtime.AIResponseCanceledPayload

func BuildStartEvent(payload StartPayload) (infrarealtime.Envelope, error) {
	return buildEvent(EventAIResponseStart, payload.RequestID, payload)
}

func BuildDeltaEvent(payload DeltaPayload) (infrarealtime.Envelope, error) {
	return buildEvent(EventAIResponseDelta, payload.RequestID, payload)
}

func buildEvent(eventType string, requestID string, payload any) (infrarealtime.Envelope, error) {
	return modulerealtime.DefaultRegistry().NewEphemeral(eventType, requestID, payload, time.Now().UTC())
}
