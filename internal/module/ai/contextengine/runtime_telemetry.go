package contextengine

import "admin_back_go/internal/telemetry"

func recordContextPlanTelemetry(recorder telemetry.Recorder, plan ContextPlan) {
	if recorder == nil || plan.Validate() != nil {
		return
	}
	attributes := telemetry.Attributes{"outcome": string(plan.RetrievalOutcome)}
	if plan.Error != nil && plan.Error.Code.Validate() == nil {
		attributes["error.code"] = string(plan.Error.Code)
		stage := EnhancementStage(plan.Error.Stage)
		if validEnhancementFailurePair(stage, plan.Error.Code) {
			attributes["context.stage"] = string(stage)
		}
	}
	recorder.Count("context_plan_total", 1, attributes)
	if plan.RetrievalOutcome == RetrievalDegraded {
		recorder.Count("context_degraded_total", 1, attributes)
	}

	recordContextStageDuration(recorder, plan.RetrievalOutcome, EnhancementStageEmbedding, plan.Metrics.QueryEmbeddingMS)
	recordContextStageDuration(recorder, plan.RetrievalOutcome, EnhancementStageRetrieval, plan.Metrics.RetrievalMS)
	recordContextStageDuration(recorder, plan.RetrievalOutcome, EnhancementStageIndex, plan.Metrics.AuthorizationMS)
	recordContextStageDuration(recorder, plan.RetrievalOutcome, EnhancementStageRerank, plan.Metrics.RerankMS)
	if plan.Metrics.QueryEmbeddingRequestCount > 0 {
		recorder.Count("context_embedding_requests_total", float64(plan.Metrics.QueryEmbeddingRequestCount), telemetry.Attributes{
			"outcome": string(plan.RetrievalOutcome),
		})
	}
}

func recordContextStageDuration(recorder telemetry.Recorder, outcome RetrievalOutcome, stage EnhancementStage, milliseconds uint64) {
	if milliseconds == 0 || !validContextTelemetryStage(stage) {
		return
	}
	recorder.Observe("context_plan_duration_seconds", float64(milliseconds)/1000, telemetry.Attributes{
		"outcome": string(outcome), "context.stage": string(stage),
	})
}

func validContextTelemetryStage(stage EnhancementStage) bool {
	switch stage {
	case EnhancementStageProfile, EnhancementStageMemory, EnhancementStageEmbedding,
		EnhancementStageIndex, EnhancementStageRetrieval, EnhancementStageRerank:
		return true
	}
	return false
}
