package requestidentity

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
)

var ErrRequestIdentityConflict = errors.New("canonical request identity fingerprint conflict")

type AttachmentIdentity struct {
	StorageProvider string `json:"storage_provider"`
	StorageKey      string `json:"storage_key"`
	SHA256          string `json:"sha256,omitempty"`
}

type GenerationOptions struct {
	MaxOutputTokens int64             `json:"max_output_tokens,omitempty"`
	Size            string            `json:"size,omitempty"`
	DurationSeconds int64             `json:"duration_seconds,omitempty"`
	Count           int64             `json:"count,omitempty"`
	Extra           map[string]string `json:"extra,omitempty"`
}

// Input is the normalized, typed request identity. Runtime timestamps, leases,
// provider request IDs and provider responses intentionally have no fields here.
type Input struct {
	UserID         int64                `json:"user_id"`
	Operation      string               `json:"operation"`
	Modality       string               `json:"modality"`
	AgentID        int64                `json:"agent_id"`
	ModelID        string               `json:"model_id"`
	NormalizedText string               `json:"normalized_text"`
	Attachments    []AttachmentIdentity `json:"attachments,omitempty"`
	Options        GenerationOptions    `json:"generation_options"`
}

func Fingerprint(input Input) ([32]byte, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

func CompareForReplay(stored, incoming [32]byte) error {
	if subtle.ConstantTimeCompare(stored[:], incoming[:]) != 1 {
		return ErrRequestIdentityConflict
	}
	return nil
}
