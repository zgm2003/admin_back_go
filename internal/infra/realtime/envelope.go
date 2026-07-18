package realtime

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"
)

type Durability string

const (
	Ephemeral Durability = "ephemeral"
	Durable   Durability = "durable"
)

const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var (
	ErrEnvelopeInvalid = errors.New("realtime envelope is invalid")
	ErrEventIDInvalid  = errors.New("realtime event id is invalid")
)

// Envelope is the project-owned WebSocket message contract. Business modules
// exchange versioned envelopes instead of raw WebSocket frames.
type Envelope struct {
	EventID    string          `json:"event_id"`
	Type       string          `json:"type"`
	RequestID  string          `json:"request_id,omitempty"`
	Sequence   uint64          `json:"sequence"`
	OccurredAt time.Time       `json:"occurred_at"`
	Durability Durability      `json:"durability"`
	Data       json.RawMessage `json:"data"`
}

// NewEnvelope builds an ephemeral server envelope.
func NewEnvelope(messageType string, requestID string, data any) (Envelope, error) {
	return NewEnvelopeAt(messageType, requestID, data, time.Now().UTC())
}

// NewEnvelopeAt is NewEnvelope with an explicit occurrence time for durable
// tests and deterministic adapters.
func NewEnvelopeAt(messageType string, requestID string, data any, occurredAt time.Time) (Envelope, error) {
	if occurredAt.IsZero() {
		return Envelope{}, fmt.Errorf("%w: occurred_at is required", ErrEnvelopeInvalid)
	}
	eventID, err := NewEventID(occurredAt)
	if err != nil {
		return Envelope{}, err
	}
	raw, err := marshalObject(data)
	if err != nil {
		return Envelope{}, err
	}
	envelope := Envelope{
		EventID:    eventID,
		Type:       strings.TrimSpace(messageType),
		RequestID:  strings.TrimSpace(requestID),
		Sequence:   0,
		OccurredAt: occurredAt,
		Durability: Ephemeral,
		Data:       raw,
	}
	if err := ValidateServerEnvelope(envelope); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

// NewDurableEnvelope builds a persisted server envelope using the sequence
// assigned by MySQL.
func NewDurableEnvelope(eventID string, messageType string, requestID string, sequence uint64, occurredAt time.Time, data any) (Envelope, error) {
	raw, err := marshalObject(data)
	if err != nil {
		return Envelope{}, err
	}
	envelope := Envelope{
		EventID:    strings.TrimSpace(eventID),
		Type:       strings.TrimSpace(messageType),
		RequestID:  strings.TrimSpace(requestID),
		Sequence:   sequence,
		OccurredAt: occurredAt,
		Durability: Durable,
		Data:       raw,
	}
	if err := ValidateServerEnvelope(envelope); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

// DecodeEnvelope strictly decodes a client WebSocket JSON message. Client
// controls may omit server-owned event metadata; their payload is validated by
// the realtime module registry.
func DecodeEnvelope(payload []byte) (Envelope, error) {
	var client struct {
		Type      string          `json:"type"`
		RequestID string          `json:"request_id,omitempty"`
		Data      json.RawMessage `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&client); err != nil {
		return Envelope{}, fmt.Errorf("%w: %v", ErrEnvelopeInvalid, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Envelope{}, fmt.Errorf("%w: trailing JSON content", ErrEnvelopeInvalid)
	}
	envelope := Envelope{
		Type:      strings.TrimSpace(client.Type),
		RequestID: strings.TrimSpace(client.RequestID),
		Data:      client.Data,
	}
	if envelope.Type == "" {
		return Envelope{}, fmt.Errorf("%w: type is required", ErrEnvelopeInvalid)
	}
	if !validOptionalRequestID(envelope.RequestID) {
		return Envelope{}, fmt.Errorf("%w: request_id exceeds 128 characters", ErrEnvelopeInvalid)
	}
	if len(envelope.Data) == 0 {
		return Envelope{}, fmt.Errorf("%w: data is required", ErrEnvelopeInvalid)
	}
	if err := validateJSONObject(envelope.Data); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

// EncodeEnvelope validates and encodes a server WebSocket message.
func EncodeEnvelope(envelope Envelope) ([]byte, error) {
	if err := ValidateServerEnvelope(envelope); err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}

func ValidateServerEnvelope(envelope Envelope) error {
	if !ValidEventID(envelope.EventID) {
		return fmt.Errorf("%w: %w", ErrEnvelopeInvalid, ErrEventIDInvalid)
	}
	if strings.TrimSpace(envelope.Type) == "" {
		return fmt.Errorf("%w: type is required", ErrEnvelopeInvalid)
	}
	if !validOptionalRequestID(strings.TrimSpace(envelope.RequestID)) {
		return fmt.Errorf("%w: request_id exceeds 128 characters", ErrEnvelopeInvalid)
	}
	if envelope.OccurredAt.IsZero() {
		return fmt.Errorf("%w: occurred_at is required", ErrEnvelopeInvalid)
	}
	switch envelope.Durability {
	case Ephemeral:
		if envelope.Sequence != 0 {
			return fmt.Errorf("%w: ephemeral sequence must be zero", ErrEnvelopeInvalid)
		}
	case Durable:
		if envelope.Sequence == 0 {
			return fmt.Errorf("%w: durable sequence is required", ErrEnvelopeInvalid)
		}
	default:
		return fmt.Errorf("%w: durability is invalid", ErrEnvelopeInvalid)
	}
	return validateJSONObject(envelope.Data)
}

func validOptionalRequestID(value string) bool {
	return utf8.RuneCountInString(value) <= 128
}

// NewEventID generates a ULID-compatible 26-character identifier from a
// 48-bit millisecond timestamp and 80 bits of cryptographic randomness.
func NewEventID(now time.Time) (string, error) {
	if now.IsZero() || now.UnixMilli() < 0 || uint64(now.UnixMilli()) > (1<<48)-1 {
		return "", ErrEventIDInvalid
	}
	var value [16]byte
	milliseconds := uint64(now.UnixMilli())
	for index := 5; index >= 0; index-- {
		value[index] = byte(milliseconds)
		milliseconds >>= 8
	}
	if _, err := rand.Read(value[6:]); err != nil {
		return "", fmt.Errorf("generate realtime event randomness: %w", err)
	}

	number := new(big.Int).SetBytes(value[:])
	base := big.NewInt(32)
	remainder := new(big.Int)
	encoded := make([]byte, 26)
	for index := len(encoded) - 1; index >= 0; index-- {
		number.QuoRem(number, base, remainder)
		encoded[index] = crockfordAlphabet[remainder.Int64()]
	}
	return string(encoded), nil
}

func ValidEventID(value string) bool {
	if len(value) != 26 || value[0] < '0' || value[0] > '7' {
		return false
	}
	for index := 1; index < len(value); index++ {
		if !strings.ContainsRune(crockfordAlphabet, rune(value[index])) {
			return false
		}
	}
	return true
}

func marshalObject(data any) (json.RawMessage, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	if err := validateJSONObject(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func validateJSONObject(raw []byte) error {
	var object map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &object) != nil || object == nil {
		return fmt.Errorf("%w: data must be a JSON object", ErrEnvelopeInvalid)
	}
	return nil
}
