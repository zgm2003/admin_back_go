package realtime

import "encoding/json"

// Envelope is the project-owned WebSocket message contract. Business modules
// must exchange versioned envelopes instead of raw WebSocket frames.
type Envelope struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	Data      json.RawMessage `json:"data"`
}

// NewEnvelope builds a versioned realtime envelope with JSON object data.
func NewEnvelope(messageType string, requestID string, data any) (Envelope, error) {
	if data == nil {
		data = map[string]any{}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		Type:      messageType,
		RequestID: requestID,
		Data:      raw,
	}, nil
}

// DecodeEnvelope decodes a client WebSocket JSON message.
func DecodeEnvelope(payload []byte) (Envelope, error) {
	var envelope Envelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return Envelope{}, err
	}
	if len(envelope.Data) == 0 {
		envelope.Data = json.RawMessage(`{}`)
	}
	return envelope, nil
}

// EncodeEnvelope encodes a server WebSocket message.
func EncodeEnvelope(envelope Envelope) ([]byte, error) {
	if len(envelope.Data) == 0 {
		envelope.Data = json.RawMessage(`{}`)
	}
	return json.Marshal(envelope)
}
