package requestidentity

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	ErrRequestIdentityConflict      = errors.New("canonical request identity fingerprint conflict")
	ErrRequestIdentityNotReplayable = errors.New("request identity is a validated non-replayable legacy marker")
	errRawInputSerialization        = errors.New("request identity Input is not canonical JSON; use BuildFingerprint")
)

type IdentityStatus string

const (
	FingerprintVersion                               = "v1"
	IdentityStatusReplayable          IdentityStatus = "replayable"
	IdentityStatusLegacyNonReplayable IdentityStatus = "legacy_non_replayable"
)

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

// Input contains the raw typed facts accepted by BuildFingerprint. Runtime
// timestamps, leases, provider request IDs and provider responses are excluded.
type Input struct {
	UserID          int64
	Operation       string
	Modality        string
	AgentID         int64
	ModelID         string
	NormalizedText  string
	Attachments     []AttachmentIdentity
	Options         GenerationOptions
	ConversationID  int64
	SourceMessageID int64
}

func (Input) MarshalJSON() ([]byte, error) {
	return nil, errRawInputSerialization
}

type canonicalPayload struct {
	FingerprintVersion string               `json:"fingerprint_version"`
	UserID             int64                `json:"user_id"`
	Operation          string               `json:"operation"`
	Modality           string               `json:"modality"`
	AgentID            int64                `json:"agent_id"`
	ModelID            string               `json:"model_id"`
	NormalizedText     string               `json:"normalized_text"`
	Attachments        []AttachmentIdentity `json:"attachments,omitempty"`
	Options            GenerationOptions    `json:"generation_options"`
	ConversationID     int64                `json:"conversation_id,omitempty"`
	SourceMessageID    int64                `json:"source_message_id,omitempty"`
}

// Fingerprint preserves the original API while using the canonical builder.
func Fingerprint(input Input) ([32]byte, error) {
	return BuildFingerprint(input)
}

// BuildFingerprint normalizes and validates semantic facts without mutating
// caller-owned slices or maps, then hashes the versioned canonical payload.
func BuildFingerprint(input Input) ([32]byte, error) {
	normalized, err := normalizeInput(input)
	if err != nil {
		return [32]byte{}, err
	}
	if err := validateInput(normalized); err != nil {
		return [32]byte{}, err
	}
	encoded, err := json.Marshal(canonicalPayload{
		FingerprintVersion: FingerprintVersion,
		UserID:             normalized.UserID,
		Operation:          normalized.Operation,
		Modality:           normalized.Modality,
		AgentID:            normalized.AgentID,
		ModelID:            normalized.ModelID,
		NormalizedText:     normalized.NormalizedText,
		Attachments:        normalized.Attachments,
		Options:            normalized.Options,
		ConversationID:     normalized.ConversationID,
		SourceMessageID:    normalized.SourceMessageID,
	})
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

func validateInput(input Input) error {
	if input.UserID <= 0 {
		return errors.New("user_id must be positive")
	}
	if input.AgentID <= 0 {
		return errors.New("agent_id must be positive")
	}
	if input.ConversationID < 0 {
		return errors.New("conversation_id must not be negative")
	}
	if input.SourceMessageID < 0 {
		return errors.New("source_message_id must not be negative")
	}
	if !isStableASCIIIdentifier(input.Operation) {
		return errors.New("operation must be a stable ASCII identifier")
	}
	if !isStableASCIIIdentifier(input.Modality) {
		return errors.New("modality must be a stable ASCII identifier")
	}
	if input.ModelID == "" {
		return errors.New("model_id must not be blank")
	}
	if !utf8.ValidString(input.ModelID) {
		return errors.New("model_id must contain valid UTF-8")
	}
	if !utf8.ValidString(input.NormalizedText) {
		return errors.New("normalized_text must contain valid UTF-8")
	}
	if input.Options.MaxOutputTokens < 0 {
		return errors.New("max_output_tokens must not be negative")
	}
	if input.Options.DurationSeconds < 0 {
		return errors.New("duration_seconds must not be negative")
	}
	if input.Options.Count < 0 {
		return errors.New("count must not be negative")
	}
	if !utf8.ValidString(input.Options.Size) {
		return errors.New("size must contain valid UTF-8")
	}
	for key, value := range input.Options.Extra {
		if !utf8.ValidString(key) || !utf8.ValidString(value) {
			return errors.New("generation options must contain valid UTF-8")
		}
	}
	for index, attachment := range input.Attachments {
		if index > 0 {
			previous := input.Attachments[index-1]
			if attachment.StorageProvider == previous.StorageProvider && attachment.StorageKey == previous.StorageKey {
				if attachment.SHA256 == previous.SHA256 {
					return fmt.Errorf("duplicate attachment object identity at index %d", index)
				}
				return fmt.Errorf("conflicting attachment object identity at index %d", index)
			}
		}
		if attachment.StorageProvider == "" {
			return fmt.Errorf("attachments[%d].storage_provider must not be blank", index)
		}
		if attachment.StorageKey == "" {
			return fmt.Errorf("attachments[%d].storage_key must not be blank", index)
		}
		if !utf8.ValidString(attachment.StorageProvider) || !utf8.ValidString(attachment.StorageKey) {
			return fmt.Errorf("attachments[%d] identity must contain valid UTF-8", index)
		}
		if attachment.SHA256 == "" {
			continue
		}
		if len(attachment.SHA256) != sha256.Size*2 {
			return fmt.Errorf("attachments[%d].sha256 must contain 64 hexadecimal characters", index)
		}
		if _, err := hex.DecodeString(attachment.SHA256); err != nil {
			return fmt.Errorf("attachments[%d].sha256 must contain 64 hexadecimal characters", index)
		}
	}
	return nil
}

func isStableASCIIIdentifier(value string) bool {
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' {
			continue
		}
		if index > 0 && (character == '-' || character == '_' || character == '.' || character == ':') {
			continue
		}
		return false
	}
	return value != ""
}

func normalizeInput(input Input) (Input, error) {
	normalized := input
	normalized.Operation = strings.TrimSpace(input.Operation)
	normalized.Modality = strings.TrimSpace(input.Modality)
	normalized.ModelID = strings.TrimSpace(input.ModelID)
	normalized.NormalizedText = normalizeText(input.NormalizedText)
	normalized.Attachments = make([]AttachmentIdentity, len(input.Attachments))
	for index, attachment := range input.Attachments {
		normalized.Attachments[index] = AttachmentIdentity{
			StorageProvider: strings.TrimSpace(attachment.StorageProvider),
			StorageKey:      strings.TrimSpace(attachment.StorageKey),
			SHA256:          strings.ToLower(strings.TrimSpace(attachment.SHA256)),
		}
	}
	sort.SliceStable(normalized.Attachments, func(left, right int) bool {
		leftAttachment := normalized.Attachments[left]
		rightAttachment := normalized.Attachments[right]
		if leftAttachment.StorageProvider != rightAttachment.StorageProvider {
			return leftAttachment.StorageProvider < rightAttachment.StorageProvider
		}
		if leftAttachment.StorageKey != rightAttachment.StorageKey {
			return leftAttachment.StorageKey < rightAttachment.StorageKey
		}
		return leftAttachment.SHA256 < rightAttachment.SHA256
	})
	normalized.Options.Size = strings.TrimSpace(input.Options.Size)
	if input.Options.Extra != nil {
		normalized.Options.Extra = make(map[string]string, len(input.Options.Extra))
		for key, value := range input.Options.Extra {
			normalizedKey := strings.TrimSpace(key)
			if normalizedKey == "" {
				return Input{}, fmt.Errorf("generation option key must not be blank")
			}
			if _, exists := normalized.Options.Extra[normalizedKey]; exists {
				return Input{}, fmt.Errorf("duplicate generation option key %q after normalization", normalizedKey)
			}
			normalized.Options.Extra[normalizedKey] = strings.TrimSpace(value)
		}
	}
	return normalized, nil
}

func normalizeText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return value
}

func CompareForReplay(status IdentityStatus, stored, incoming [32]byte) error {
	if status != IdentityStatusReplayable {
		return ErrRequestIdentityNotReplayable
	}
	if subtle.ConstantTimeCompare(stored[:], incoming[:]) != 1 {
		return ErrRequestIdentityConflict
	}
	return nil
}
