package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	PreparedChatSchemaInlineV1                = "openai_chat_inline_v1"
	PreparedChatSchemaFileManifestV1          = "openai_chat_file_manifest_v1"
	PreparedChatSchemaResponsesInlineV1       = "openai_responses_inline_v1"
	PreparedChatSchemaResponsesFileManifestV1 = "openai_responses_file_manifest_v1"
)

type PreparedFileRef struct {
	Ref       string `json:"ref"`
	ObjectKey string `json:"object_key"`
	ETag      string `json:"etag"`
	Size      int64  `json:"size"`
	MIMEType  string `json:"mime_type"`
	Filename  string `json:"filename"`
}

type PreparedChatFileManifest struct {
	Schema        string            `json:"schema"`
	APIProtocol   string            `json:"api_protocol,omitempty"`
	FileInputMode string            `json:"file_input_mode,omitempty"`
	Request       json.RawMessage   `json:"request"`
	Files         []PreparedFileRef `json:"files"`
}

type PreparedChatInlineEnvelope struct {
	Schema      string          `json:"schema"`
	APIProtocol string          `json:"api_protocol"`
	Request     json.RawMessage `json:"request"`
}

type PreparedFileOpenInput struct {
	ObjectKey string
	ETag      string
	Size      int64
}

type PreparedFileObjectMetadata struct {
	ETag     string
	Size     int64
	MIMEType string
}

type PreparedFileOpener interface {
	Head(context.Context, PreparedFileOpenInput) (PreparedFileObjectMetadata, error)
	Open(context.Context, PreparedFileOpenInput) (io.ReadCloser, PreparedFileObjectMetadata, error)
}

type FileInputMetrics struct {
	COSHeadMS                int64 `json:"cos_head_ms"`
	COSStreamMS              int64 `json:"cos_stream_ms"`
	MaterializedRequestBytes int64 `json:"materialized_request_bytes"`
}

func (manifest PreparedChatFileManifest) Validate() error {
	switch manifest.Schema {
	case PreparedChatSchemaFileManifestV1:
		if manifest.APIProtocol != "" || manifest.FileInputMode != APIProtocolChatCompletions {
			return errors.New("legacy prepared file manifest mode is invalid")
		}
	case PreparedChatSchemaResponsesFileManifestV1:
		if manifest.APIProtocol != APIProtocolResponses || manifest.FileInputMode != "" {
			return errors.New("prepared file manifest API protocol is invalid")
		}
	default:
		return errors.New("prepared file manifest schema is invalid")
	}
	if len(manifest.Request) == 0 || !json.Valid(manifest.Request) || len(manifest.Files) == 0 {
		return errors.New("prepared file manifest request and files are required")
	}
	files := make(map[string]PreparedFileRef, len(manifest.Files))
	for index, file := range manifest.Files {
		wantRef := "file-" + strconv.Itoa(index+1)
		if file.Ref != wantRef || strings.TrimSpace(file.ObjectKey) == "" || strings.TrimSpace(file.ETag) == "" || file.Size <= 0 || strings.TrimSpace(file.MIMEType) == "" || strings.TrimSpace(file.Filename) == "" {
			return fmt.Errorf("prepared file manifest file %d is invalid", index+1)
		}
		if _, exists := files[file.Ref]; exists {
			return errors.New("prepared file manifest contains duplicate refs")
		}
		files[file.Ref] = file
	}
	refs, err := requestFileRefs(manifest.Request)
	if err != nil {
		return err
	}
	if len(refs) != len(manifest.Files) {
		return errors.New("prepared file manifest refs do not match files")
	}
	for index, ref := range refs {
		if ref != manifest.Files[index].Ref {
			return errors.New("prepared file manifest ref order does not match files")
		}
		if _, exists := files[ref]; !exists {
			return errors.New("prepared file manifest contains an unknown ref")
		}
	}
	return nil
}

func (manifest PreparedChatFileManifest) Protocol() string {
	if manifest.Schema == PreparedChatSchemaResponsesFileManifestV1 {
		return manifest.APIProtocol
	}
	if manifest.Schema == PreparedChatSchemaFileManifestV1 && manifest.FileInputMode == APIProtocolChatCompletions {
		return APIProtocolChatCompletions
	}
	return ""
}

func (envelope PreparedChatInlineEnvelope) Validate() error {
	if envelope.Schema != PreparedChatSchemaResponsesInlineV1 || envelope.APIProtocol != APIProtocolResponses {
		return errors.New("prepared inline envelope API protocol is invalid")
	}
	if len(envelope.Request) == 0 || !json.Valid(envelope.Request) {
		return errors.New("prepared inline envelope request is invalid")
	}
	var request struct {
		Input    json.RawMessage `json:"input"`
		Messages json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(envelope.Request, &request); err != nil || len(bytes.TrimSpace(request.Input)) == 0 || len(bytes.TrimSpace(request.Messages)) != 0 {
		return errors.New("prepared inline envelope request protocol is invalid")
	}
	return nil
}

func MarshalPreparedChatInlineEnvelope(envelope PreparedChatInlineEnvelope) ([]byte, error) {
	if err := envelope.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode prepared inline envelope: %w", err)
	}
	return encoded, nil
}

func ParsePreparedChatInlineEnvelope(body []byte) (PreparedChatInlineEnvelope, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var envelope PreparedChatInlineEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return PreparedChatInlineEnvelope{}, fmt.Errorf("decode prepared inline envelope: %w", err)
	}
	if err := requirePreparedJSONEnd(decoder); err != nil {
		return PreparedChatInlineEnvelope{}, err
	}
	if err := envelope.Validate(); err != nil {
		return PreparedChatInlineEnvelope{}, err
	}
	return envelope, nil
}

func MarshalPreparedChatFileManifest(manifest PreparedChatFileManifest) ([]byte, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("encode prepared file manifest: %w", err)
	}
	return encoded, nil
}

func ParsePreparedChatFileManifest(body []byte) (PreparedChatFileManifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var manifest PreparedChatFileManifest
	if err := decoder.Decode(&manifest); err != nil {
		return PreparedChatFileManifest{}, fmt.Errorf("decode prepared file manifest: %w", err)
	}
	if err := requirePreparedJSONEnd(decoder); err != nil {
		return PreparedChatFileManifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return PreparedChatFileManifest{}, err
	}
	return manifest, nil
}

func DetectPreparedChatSchema(body []byte) (string, error) {
	if len(body) == 0 || !json.Valid(body) {
		return "", errors.New("prepared chat request must be valid JSON")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		return PreparedChatSchemaInlineV1, nil
	}
	rawSchema, exists := object["schema"]
	if !exists {
		return PreparedChatSchemaInlineV1, nil
	}
	var schema string
	if err := json.Unmarshal(rawSchema, &schema); err != nil {
		return "", errors.New("prepared chat schema must be a string")
	}
	switch schema {
	case PreparedChatSchemaResponsesInlineV1:
		if _, err := ParsePreparedChatInlineEnvelope(body); err != nil {
			return "", err
		}
	case PreparedChatSchemaFileManifestV1, PreparedChatSchemaResponsesFileManifestV1:
		if _, err := ParsePreparedChatFileManifest(body); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported prepared chat schema %q", schema)
	}
	return schema, nil
}

func requestFileRefs(request json.RawMessage) ([]string, error) {
	var envelope struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		Input []struct {
			Content json.RawMessage `json:"content"`
		} `json:"input"`
	}
	if err := json.Unmarshal(request, &envelope); err != nil {
		return nil, fmt.Errorf("decode prepared file request: %w", err)
	}
	refs := make([]string, 0)
	seen := make(map[string]struct{})
	messages := append(envelope.Messages, envelope.Input...)
	for _, message := range messages {
		var parts []json.RawMessage
		if err := json.Unmarshal(message.Content, &parts); err != nil {
			continue
		}
		for _, rawPart := range parts {
			var kind struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(rawPart, &kind); err != nil || kind.Type != "file_ref" {
				continue
			}
			decoder := json.NewDecoder(bytes.NewReader(rawPart))
			decoder.DisallowUnknownFields()
			var part struct {
				Type string `json:"type"`
				Ref  string `json:"ref"`
			}
			if err := decoder.Decode(&part); err != nil || requirePreparedJSONEnd(decoder) != nil || part.Ref == "" {
				return nil, errors.New("prepared file request contains an invalid file_ref")
			}
			if _, exists := seen[part.Ref]; exists {
				return nil, errors.New("prepared file request contains duplicate file_ref")
			}
			seen[part.Ref] = struct{}{}
			refs = append(refs, part.Ref)
		}
	}
	return refs, nil
}

func requirePreparedJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("prepared chat request contains trailing JSON")
		}
		return err
	}
	return nil
}
