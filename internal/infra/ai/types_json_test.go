package ai

import (
	"encoding/json"
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
