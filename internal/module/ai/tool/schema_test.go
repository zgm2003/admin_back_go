package aitool

import (
	"encoding/json"
	"testing"
)

func TestValidateJSONAgainstSchemaUsesDraft2020AndRejectsTrailingJSON(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count": map[string]any{"type": "integer"},
		},
		"required":             []any{"count"},
		"additionalProperties": false,
	}
	if err := validateJSONAgainstSchema(schema, json.RawMessage(`{"count":1}`)); err != nil {
		t.Fatalf("valid instance rejected: %v", err)
	}
	if err := validateJSONAgainstSchema(schema, json.RawMessage(`{"count":"1"}`)); err == nil {
		t.Fatal("schema mismatch accepted")
	}
	if err := validateJSONAgainstSchema(schema, json.RawMessage(`{"count":1} { }`)); err == nil {
		t.Fatal("trailing JSON accepted")
	}
}

func TestValidateJSONAgainstSchemaRejectsExternalReferences(t *testing.T) {
	schema := map[string]any{"$ref": "https://example.test/schema.json"}
	if err := validateJSONAgainstSchema(schema, json.RawMessage(`{}`)); err == nil {
		t.Fatal("external schema reference accepted")
	}
}

func TestInvalidJSONAuditEnvelopeContainsOnlyDigestAndLength(t *testing.T) {
	raw := json.RawMessage(`{"secret":"oops"`)
	envelope := invalidJSONAuditEnvelope(raw)
	if !json.Valid(envelope) || string(envelope) == string(raw) {
		t.Fatalf("invalid audit envelope: %s", envelope)
	}
	var value map[string]any
	if err := json.Unmarshal(envelope, &value); err != nil {
		t.Fatal(err)
	}
	if value["invalid_json"] != true || value["byte_length"] != float64(len(raw)) || len(value["sha256"].(string)) != 64 {
		t.Fatalf("unexpected envelope: %s", envelope)
	}
}
