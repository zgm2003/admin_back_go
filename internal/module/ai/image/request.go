package aiimage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"admin_back_go/internal/module/ai/requestidentity"
)

const imageInputSnapshotVersion = "ai_image_input_v1"

var ErrImageInputSnapshotInvalid = errors.New("AI image input snapshot is invalid")

type AttachmentSnapshot struct {
	Role             string `json:"role"`
	SortOrder        int    `json:"sort_order"`
	StorageProvider  string `json:"storage_provider"`
	StorageKey       string `json:"storage_key"`
	SHA256           string `json:"sha256"`
	MimeType         string `json:"mime_type"`
	SizeBytes        int64  `json:"size_bytes"`
	RelatedSortOrder int    `json:"related_sort_order,omitempty"`
}

type ProviderInputSnapshot struct {
	Version           string               `json:"version"`
	Operation         string               `json:"operation"`
	Modality          string               `json:"modality"`
	Model             string               `json:"model"`
	Prompt            string               `json:"prompt"`
	Size              string               `json:"size"`
	Quality           string               `json:"quality"`
	OutputFormat      string               `json:"output_format"`
	OutputCompression *int                 `json:"output_compression,omitempty"`
	Moderation        string               `json:"moderation"`
	N                 int                  `json:"n"`
	MaxOutputTokens   int64                `json:"max_output_tokens"`
	Attachments       []AttachmentSnapshot `json:"attachments,omitempty"`
}

func EncodeProviderInputSnapshot(input ProviderInputSnapshot) (string, error) {
	input.Version = imageInputSnapshotVersion
	input.Operation = strings.TrimSpace(input.Operation)
	input.Modality = strings.TrimSpace(input.Modality)
	input.Model = strings.TrimSpace(input.Model)
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.Size = strings.TrimSpace(input.Size)
	input.Quality = strings.TrimSpace(input.Quality)
	input.OutputFormat = strings.TrimSpace(input.OutputFormat)
	input.Moderation = strings.TrimSpace(input.Moderation)
	input.Attachments = append([]AttachmentSnapshot(nil), input.Attachments...)
	if err := validateProviderInputSnapshot(input); err != nil {
		return "", err
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func DecodeProviderInputSnapshot(raw string) (ProviderInputSnapshot, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	var input ProviderInputSnapshot
	if err := decoder.Decode(&input); err != nil {
		return ProviderInputSnapshot{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ProviderInputSnapshot{}, ErrImageInputSnapshotInvalid
	}
	if err := validateProviderInputSnapshot(input); err != nil {
		return ProviderInputSnapshot{}, err
	}
	return input, nil
}

func validateProviderInputSnapshot(input ProviderInputSnapshot) error {
	if input.Version != imageInputSnapshotVersion || input.Operation != "image.generate" || input.Modality != "image" ||
		input.Model == "" || input.Prompt == "" || input.Size == "" || input.Quality == "" || input.OutputFormat == "" ||
		input.Moderation == "" || input.N <= 0 || input.MaxOutputTokens <= 0 {
		return ErrImageInputSnapshotInvalid
	}
	for index, attachment := range input.Attachments {
		attachment.StorageProvider = strings.TrimSpace(attachment.StorageProvider)
		attachment.StorageKey = strings.TrimSpace(attachment.StorageKey)
		attachment.SHA256 = strings.ToLower(strings.TrimSpace(attachment.SHA256))
		attachment.MimeType = strings.TrimSpace(attachment.MimeType)
		if (attachment.Role != FileRoleInput && attachment.Role != FileRoleMask) || attachment.SortOrder <= 0 ||
			attachment.StorageProvider == "" || attachment.StorageKey == "" || attachment.MimeType == "" || attachment.SizeBytes <= 0 ||
			len(attachment.SHA256) != sha256.Size*2 {
			return fmt.Errorf("%w: attachment %d", ErrImageInputSnapshotInvalid, index)
		}
		if decoded, err := hex.DecodeString(attachment.SHA256); err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("%w: attachment %d digest", ErrImageInputSnapshotInvalid, index)
		}
		if attachment.Role == FileRoleMask && attachment.RelatedSortOrder <= 0 {
			return fmt.Errorf("%w: mask target", ErrImageInputSnapshotInvalid)
		}
	}
	return nil
}

func imageRequestFingerprint(userID uint64, agentID uint64, input ProviderInputSnapshot) ([sha256.Size]byte, error) {
	identity, err := RequestIdentityInput(userID, agentID, input)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return requestidentity.BuildFingerprint(identity)
}

func RequestIdentityInput(userID uint64, agentID uint64, input ProviderInputSnapshot) (requestidentity.Input, error) {
	if userID == 0 || agentID == 0 {
		return requestidentity.Input{}, ErrImageInputSnapshotInvalid
	}
	if err := validateProviderInputSnapshot(input); err != nil {
		return requestidentity.Input{}, err
	}
	attachments := make([]requestidentity.AttachmentIdentity, 0, len(input.Attachments))
	for _, attachment := range input.Attachments {
		attachments = append(attachments, requestidentity.AttachmentIdentity{
			StorageProvider: attachment.StorageProvider,
			StorageKey:      attachment.StorageKey,
			SHA256:          attachment.SHA256,
		})
	}
	manifest, err := json.Marshal(input.Attachments)
	if err != nil {
		return requestidentity.Input{}, err
	}
	manifestDigest := sha256.Sum256(manifest)
	compression := ""
	if input.OutputCompression != nil {
		compression = strconv.Itoa(*input.OutputCompression)
	}
	return requestidentity.Input{
		UserID:         int64(userID),
		Operation:      input.Operation,
		Modality:       input.Modality,
		AgentID:        int64(agentID),
		ModelID:        input.Model,
		NormalizedText: input.Prompt,
		Attachments:    attachments,
		Options: requestidentity.GenerationOptions{
			MaxOutputTokens: input.MaxOutputTokens,
			Size:            input.Size,
			Count:           int64(input.N),
			Extra: map[string]string{
				"quality":                    input.Quality,
				"output_format":              input.OutputFormat,
				"output_compression":         compression,
				"moderation":                 input.Moderation,
				"attachment_manifest_sha256": hex.EncodeToString(manifestDigest[:]),
			},
		},
	}, nil
}
