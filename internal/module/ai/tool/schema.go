package aitool

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const toolSchemaURN = "urn:admin:ai-tool-schema"

type denyExternalSchemaLoader struct{}

func (denyExternalSchemaLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external JSON Schema reference %q is not allowed", url)
}

func validateJSONAgainstSchema(schema map[string]any, raw json.RawMessage) error {
	if schema == nil {
		return fmt.Errorf("JSON Schema is missing")
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.UseLoader(denyExternalSchemaLoader{})
	if err := compiler.AddResource(toolSchemaURN, schema); err != nil {
		return fmt.Errorf("add JSON Schema resource: %w", err)
	}
	compiled, err := compiler.Compile(toolSchemaURN)
	if err != nil {
		return fmt.Errorf("compile JSON Schema: %w", err)
	}
	instance, err := decodeSingleJSON(raw)
	if err != nil {
		return err
	}
	if err := compiled.Validate(instance); err != nil {
		return fmt.Errorf("validate JSON instance: %w", err)
	}
	return nil
}

func decodeSingleJSON(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode JSON instance: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode JSON instance: trailing JSON value")
		}
		return nil, fmt.Errorf("decode JSON instance: %w", err)
	}
	return value, nil
}

func invalidJSONAuditEnvelope(raw json.RawMessage) json.RawMessage {
	digest := sha256.Sum256(raw)
	encoded, _ := json.Marshal(struct {
		InvalidJSON bool   `json:"invalid_json"`
		SHA256      string `json:"sha256"`
		ByteLength  int    `json:"byte_length"`
	}{
		InvalidJSON: true,
		SHA256:      hex.EncodeToString(digest[:]),
		ByteLength:  len(raw),
	})
	return json.RawMessage(encoded)
}
