package ai

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSafeInputUpperBoundFromRequestIsDeterministic(t *testing.T) {
	body := []byte("{ \"model\": \"gpt-test\" }")
	first, err := SafeInputUpperBoundFromRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	second, err := SafeInputUpperBoundFromRequest(append([]byte(nil), body...))
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first <= int64(len(body)) {
		t.Fatalf("bounds first=%d second=%d body=%d", first, second, len(body))
	}
	if _, err := SafeInputUpperBoundFromRequest(nil); err == nil {
		t.Fatal("expected empty prepared request to fail")
	}
}

func TestConnectionResultUsesDocumentedJSONNames(t *testing.T) {
	payload, err := json.Marshal(TestConnectionResult{OK: true, Status: "ok", LatencyMs: 12, Message: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"ok":true,"status":"ok","latency_ms":12,"message":"ready"}`
	if string(payload) != want {
		t.Fatalf("payload=%s, want %s", payload, want)
	}
}

func TestDefaultTransportCapabilitiesDeclareNativeFileProof(t *testing.T) {
	capabilities, ok := DefaultTransportCapabilities(EngineTypeOpenAI)
	if !ok {
		t.Fatal("OpenAI-compatible transport capabilities are missing")
	}
	wantModalities := []string{"text", "image", "file"}
	if stringListJSON(capabilities.InputModalities) != stringListJSON(wantModalities) {
		t.Fatalf("input modalities=%#v", capabilities.InputModalities)
	}
	wantStrategies := []string{SafeInputUpperBoundStrategyUTF8RequestBytesV1, SafeInputUpperBoundStrategyNativeFileContextWindowV1}
	if stringListJSON(capabilities.SafeInputUpperBoundStrategies) != stringListJSON(wantStrategies) {
		t.Fatalf("safe input strategies=%#v", capabilities.SafeInputUpperBoundStrategies)
	}
	if capabilities.SafeInputUpperBoundStrategy != SafeInputUpperBoundStrategyUTF8RequestBytesV1 {
		t.Fatalf("legacy inline strategy=%q", capabilities.SafeInputUpperBoundStrategy)
	}
}

func TestPreparedChatFileManifestRoundTripAndSchemaDetection(t *testing.T) {
	manifest := PreparedChatFileManifest{
		Schema:        PreparedChatSchemaFileManifestV1,
		FileInputMode: "chat_completions",
		Request:       json.RawMessage(`{"model":"gpt-test","messages":[{"role":"user","content":[{"type":"text","text":"read"},{"type":"file_ref","ref":"file-1"}]}]}`),
		Files: []PreparedFileRef{{
			Ref: "file-1", ObjectKey: "ai_chat_attachments/2026/07/report.pdf", ETag: `"etag-v1"`,
			Size: 4, MIMEType: "application/pdf", Filename: "report.pdf",
		}},
	}
	encoded, err := MarshalPreparedChatFileManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := DetectPreparedChatSchema(encoded)
	if err != nil || schema != PreparedChatSchemaFileManifestV1 {
		t.Fatalf("schema=%q err=%v", schema, err)
	}
	decoded, err := ParsePreparedChatFileManifest(encoded)
	if err != nil {
		t.Fatal(err)
	}
	encodedAgain, err := MarshalPreparedChatFileManifest(decoded)
	if err != nil || string(encodedAgain) != string(encoded) {
		t.Fatalf("manifest is not canonical:\nfirst=%s\nsecond=%s\nerr=%v", encoded, encodedAgain, err)
	}

	inline, err := DetectPreparedChatSchema([]byte(`{"model":"gpt-test"}`))
	if err != nil || inline != PreparedChatSchemaInlineV1 {
		t.Fatalf("inline schema=%q err=%v", inline, err)
	}
}

func TestPreparedResponsesInlineEnvelopeRoundTripAndSchemaDetection(t *testing.T) {
	envelope := PreparedChatInlineEnvelope{
		Schema:      PreparedChatSchemaResponsesInlineV1,
		APIProtocol: APIProtocolResponses,
		Request:     json.RawMessage(`{"model":"gpt-5.6","input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}],"stream":true,"store":false}`),
	}
	encoded, err := MarshalPreparedChatInlineEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := DetectPreparedChatSchema(encoded)
	if err != nil || schema != PreparedChatSchemaResponsesInlineV1 {
		t.Fatalf("schema=%q err=%v", schema, err)
	}
	decoded, err := ParsePreparedChatInlineEnvelope(encoded)
	if err != nil {
		t.Fatal(err)
	}
	encodedAgain, err := MarshalPreparedChatInlineEnvelope(decoded)
	if err != nil || string(encodedAgain) != string(encoded) {
		t.Fatalf("inline envelope is not canonical:\nfirst=%s\nsecond=%s\nerr=%v", encoded, encodedAgain, err)
	}
	unknown := strings.Replace(string(encoded), `"request":`, `"unknown":true,"request":`, 1)
	if _, err := ParsePreparedChatInlineEnvelope([]byte(unknown)); err == nil {
		t.Fatal("Responses inline envelope accepted an unknown field")
	}
}

func TestPreparedChatFileManifestRejectsUnknownOrMismatchedFacts(t *testing.T) {
	valid := `{"schema":"openai_chat_file_manifest_v1","file_input_mode":"chat_completions","request":{"model":"gpt-test","messages":[{"role":"user","content":[{"type":"file_ref","ref":"file-1"}]}]},"files":[{"ref":"file-1","object_key":"ai_chat_attachments/report.pdf","etag":"\"etag-v1\"","size":4,"mime_type":"application/pdf","filename":"report.pdf"}]}`
	for _, candidate := range []string{
		strings.Replace(valid, `"filename":"report.pdf"`, `"filename":"report.pdf","secret":"no"`, 1),
		strings.Replace(valid, `"ref":"file-1"`, `"ref":"file-2"`, 1),
		strings.Replace(valid, `"file_input_mode":"chat_completions"`, `"file_input_mode":"disabled"`, 1),
		strings.Replace(valid, `"schema":"openai_chat_file_manifest_v1"`, `"schema":"future"`, 1),
	} {
		if _, err := DetectPreparedChatSchema([]byte(candidate)); err == nil {
			t.Fatalf("invalid manifest was accepted: %s", candidate)
		}
	}
}

func stringListJSON(values []string) string {
	raw, _ := json.Marshal(values)
	return string(raw)
}

func TestUsageSnapshotReportedWithoutItemsIsNotComplete(t *testing.T) {
	snapshot := UsageSnapshot{Status: UsageStatusReported}
	if snapshot.Complete() {
		t.Fatal("reported usage without items must fail closed")
	}
}

func TestUsageSnapshotAllowsExplicitZeroQuantity(t *testing.T) {
	snapshot, err := NewUsageSnapshot(UsageStatusReported, []byte(`{"usage":{"input_tokens":0}}`), []UsageItem{{Category: UsageCategoryInput, Unit: "token", Quantity: 0}})
	if err != nil {
		t.Fatalf("explicit zero usage item rejected: %v", err)
	}
	if !snapshot.Complete() {
		t.Fatalf("explicit zero usage item is not complete: %+v", snapshot)
	}
}

func TestUsageSnapshotJSONRoundTripPreservesRawProviderBytes(t *testing.T) {
	raw := []byte("{\n  \"type\": \"response.completed\",\n  \"output\": \"<think>&done</think>\",\n  \"usage\": { \"input_tokens\": 1 }\n}")
	snapshot, err := NewUsageSnapshot(UsageStatusComplete, raw, []UsageItem{{
		Category: UsageCategoryInput,
		Unit:     "token",
		Quantity: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var restored UsageSnapshot
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(restored.RawProviderJSON, raw) {
		t.Fatalf("raw provider evidence changed across persistence: got %q want %q", restored.RawProviderJSON, raw)
	}
	if restored.ResponseSHA256 != snapshot.ResponseSHA256 {
		t.Fatalf("provider evidence hash changed across persistence: got %x want %x", restored.ResponseSHA256, snapshot.ResponseSHA256)
	}
}

func TestUsageSnapshotUnmarshalRestoresHashVerifiedLegacyHTMLEscapes(t *testing.T) {
	raw := []byte(`{"type":"response.completed","response":{"output":[{"text":"<think>&done</think>"}],"usage":{"input_tokens":1}}}`)
	snapshot, err := NewUsageSnapshot(UsageStatusComplete, raw, []UsageItem{{
		Category: UsageCategoryInput,
		Unit:     "token",
		Quantity: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	legacy := struct {
		Status          string          `json:"status"`
		RawProviderJSON json.RawMessage `json:"raw_provider_json,omitempty"`
		Items           []UsageItem     `json:"items,omitempty"`
		ResponseSHA256  [32]byte        `json:"response_sha256,omitempty"`
	}{
		Status: snapshot.Status, RawProviderJSON: snapshot.RawProviderJSON,
		Items: snapshot.Items, ResponseSHA256: snapshot.ResponseSHA256,
	}
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`\u003c`)) || !bytes.Contains(encoded, []byte(`\u0026`)) {
		t.Fatalf("legacy fixture did not reproduce encoding/json HTML escaping: %s", encoded)
	}

	var restored UsageSnapshot
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored.RawProviderJSON, raw) {
		t.Fatalf("legacy provider evidence was not restored: got %q want %q", restored.RawProviderJSON, raw)
	}
}
