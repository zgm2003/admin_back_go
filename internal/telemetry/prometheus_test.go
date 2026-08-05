package telemetry

import (
	"context"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPrometheusRecordsSanitizedCountersHistogramsAndSpans(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder, err := NewPrometheus(registry)
	if err != nil {
		t.Fatalf("NewPrometheus: %v", err)
	}
	recorder.Count("http.requests", 1, Attributes{
		"http.method":   "GET",
		"http.route":    "/api/admin/v1/users/:id?token=private",
		"http.status":   200,
		"authorization": "Bearer private",
	})
	recorder.Observe("http.duration_seconds", 0.25, Attributes{"http.route": "/health"})
	_, finish := recorder.Start(context.Background(), "provider.request", Attributes{
		"provider.name":     "openai",
		"provider.modality": "chat",
		"prompt":            "private prompt",
	})
	finish(nil)

	count := testutil.ToFloat64(recorder.events.WithLabelValues(
		"http.requests", "GET", "/api/admin/v1/users/:id", "200", "", "", "", "", "", "", "", "",
	))
	if count != 1 {
		t.Fatalf("http counter=%v", count)
	}

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	var exposition strings.Builder
	for _, family := range families {
		exposition.WriteString(family.String())
	}
	text := exposition.String()
	for _, want := range []string{"admin_telemetry_events_total", "admin_telemetry_observation", "provider.request", "openai", "chat"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in metrics: %s", want, text)
		}
	}
	if strings.Contains(strings.ToLower(text), "private") || strings.Contains(strings.ToLower(text), "authorization") || strings.Contains(strings.ToLower(text), "prompt") {
		t.Fatalf("secret/high-cardinality data leaked: %s", text)
	}
}

func TestPrometheusPublishesClosedContextStageLabel(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder, err := NewPrometheus(registry)
	if err != nil {
		t.Fatal(err)
	}
	recorder.Count("context_plan_total", 1, Attributes{"outcome": "degraded", "context.stage": "embedding"})
	count := testutil.ToFloat64(recorder.events.WithLabelValues(
		"context_plan_total", "", "", "", "", "", "", "degraded", "", "", "", "embedding",
	))
	if count != 1 {
		t.Fatalf("context counter=%v", count)
	}
}
