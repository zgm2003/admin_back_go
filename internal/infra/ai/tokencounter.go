package ai

import (
	"encoding/json"
	"errors"
	"unicode/utf8"
)

var ErrTokenCounterNotRegistered = errors.New("AI token counter is not registered")

type TokenCounter interface {
	ID() string
	UpperBoundText(string) (int64, error)
	UpperBoundJSON(json.RawMessage) (int64, error)
}

const TokenCounterUTF8BytesV1 = "utf8_bytes_v1"

func ResolveTokenCounter(id string) (TokenCounter, error) {
	switch id {
	case TokenCounterUTF8BytesV1:
		return utf8BytesTokenCounter{}, nil
	default:
		return nil, ErrTokenCounterNotRegistered
	}
}

type utf8BytesTokenCounter struct{}

func (utf8BytesTokenCounter) ID() string { return TokenCounterUTF8BytesV1 }

func (utf8BytesTokenCounter) UpperBoundText(text string) (int64, error) {
	if !utf8.ValidString(text) {
		return 0, ErrInvalidConfig
	}
	return int64(len(text)), nil
}

func (utf8BytesTokenCounter) UpperBoundJSON(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || !utf8.Valid(raw) || !json.Valid(raw) {
		return 0, ErrInvalidConfig
	}
	return int64(len(raw)), nil
}
