package ai

import (
	"encoding/json"
	"testing"
)

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
