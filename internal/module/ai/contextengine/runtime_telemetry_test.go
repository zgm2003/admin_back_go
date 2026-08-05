package contextengine

import (
	"testing"

	"admin_back_go/internal/telemetry"
)

func TestContextTelemetryEmitsOnlyClosedPlanFacts(t *testing.T) {
	recorder := telemetry.NewMemoryRecorder()
	plan := degradedReadyPlan(t)
	plan.ID = 91
	plan.Metrics = ContextPlanMetricsV1{
		Schema: ContextPlanMetricsSchemaV1, QueryEmbeddingMS: 10, RetrievalMS: 20,
		AuthorizationMS: 30, RerankMS: 40, PackingMS: 50, QueryEmbeddingRequestCount: 1,
	}
	rehashContextPlan(t, &plan)
	recordContextPlanTelemetry(recorder, plan)

	events := recorder.Events()
	wantNames := map[string]int{
		"context_plan_total": 1, "context_degraded_total": 1,
		"context_plan_duration_seconds": 4, "context_embedding_requests_total": 1,
	}
	for _, event := range events {
		wantNames[event.Name]--
		for key, value := range event.Attributes {
			if key != "outcome" && key != "error.code" && key != "context.stage" {
				t.Fatalf("unexpected telemetry attribute %q=%v", key, value)
			}
		}
	}
	for name, remaining := range wantNames {
		if remaining != 0 {
			t.Fatalf("event %s remaining=%d events=%+v", name, remaining, events)
		}
	}
}

func TestTerminalPlanReuseEmitsNoContextExecutionTelemetry(t *testing.T) {
	plan := degradedReadyPlan(t)
	plan.ID = 91
	rehashContextPlan(t, &plan)
	repository := &fakePlannerRepository{existing: &plan}
	recorder := telemetry.NewMemoryRecorder()
	service := NewRuntimeService(&runtimeMaterializerFixture{}, NewPlanner(PlannerDependencies{
		Repository: repository, GuardFactory: fixedGuardFactory{hash: testSHA256("authority")},
	})).WithTelemetry(recorder)

	if _, err := service.BuildPlan(t.Context(), RuntimeInput{RunID: plan.RunID}); err != nil {
		t.Fatal(err)
	}
	if events := recorder.Events(); len(events) != 0 {
		t.Fatalf("terminal Plan reuse replayed telemetry: %+v", events)
	}
}
