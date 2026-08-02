package contextengine

import (
	"crypto/sha256"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestContextPlanValidateTerminalShape(t *testing.T) {
	ready := validReadyPlan()
	if err := ready.Validate(); err != nil {
		t.Fatal(err)
	}

	failed := ready
	failed.State = PlanFailed
	failed.RetrievalOutcome = RetrievalFailed
	failed.PlanSHA256 = nil
	failed.Error = &PlanError{Stage: "retrieval", Code: ErrCodeRetrievalFailed}
	failed.Items = nil
	if err := failed.Validate(); err != nil {
		t.Fatal(err)
	}

	invalid := failed
	hash := sha256.Sum256([]byte("failed-plan"))
	invalid.PlanSHA256 = &hash
	if err := invalid.Validate(); err == nil {
		t.Fatal("failed plan must not have a hash")
	}

	invalid = failed
	invalid.Items = ready.Items
	if err := invalid.Validate(); err == nil {
		t.Fatal("failed plan must not carry items")
	}
}

func TestContextPlanValidateRejectsInvalidAggregateFacts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ContextPlan)
	}{
		{name: "unknown block kind", mutate: func(plan *ContextPlan) { plan.Items[0].Block.Kind = BlockKind("unknown") }},
		{name: "citation on current message", mutate: func(plan *ContextPlan) { plan.Items[0].CitationKey = stringPointer("C1") }},
		{name: "empty citation as null", mutate: func(plan *ContextPlan) { plan.Items[0].CitationKey = stringPointer("") }},
		{name: "negative token bound", mutate: func(plan *ContextPlan) { plan.Items[0].Block.TokenUpperBound = -1 }},
		{name: "missing source hash", mutate: func(plan *ContextPlan) { plan.Items[0].Block.SourceSHA256 = [sha256.Size]byte{} }},
		{name: "excluded without reason", mutate: func(plan *ContextPlan) { plan.Items[0].Decision = DecisionExcluded }},
		{name: "selected with reason", mutate: func(plan *ContextPlan) {
			reason := ExclusionBudgetExceeded
			plan.Items[0].ExclusionReason = &reason
		}},
		{name: "noncontiguous ordinal", mutate: func(plan *ContextPlan) { plan.Items[0].Ordinal = 2 }},
		{name: "invalid budget", mutate: func(plan *ContextPlan) { plan.Budget.KnownInputBudget++ }},
		{name: "negative budget", mutate: func(plan *ContextPlan) { plan.Budget.KnownInputBudget = -1 }},
		{name: "empty failed message", mutate: func(plan *ContextPlan) {
			plan.State = PlanFailed
			plan.RetrievalOutcome = RetrievalFailed
			plan.PlanSHA256 = nil
			plan.Items = nil
			plan.Error = &PlanError{Stage: "retrieval", Code: ErrCodeRetrievalFailed, Message: stringPointer("")}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := validReadyPlan()
			test.mutate(&plan)
			if err := plan.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestClosedContextEnumsRejectUnknownValues(t *testing.T) {
	tests := []struct {
		name     string
		validate func() error
	}{
		{name: "retrieval outcome", validate: func() error { return RetrievalOutcome("unknown").Validate() }},
		{name: "plan state", validate: func() error { return PlanState("unknown").Validate() }},
		{name: "budget proof", validate: func() error { return BudgetProof("unknown").Validate() }},
		{name: "block kind", validate: func() error { return BlockKind("unknown").Validate() }},
		{name: "decision", validate: func() error { return Decision("unknown").Validate() }},
		{name: "exclusion reason", validate: func() error { return ExclusionReason("unknown").Validate() }},
		{name: "profile index state", validate: func() error { return ProfileIndexState("unknown").Validate() }},
		{name: "error code", validate: func() error { return ErrorCode("ai.context.unknown").Validate() }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.validate(); err == nil {
				t.Fatal("unknown enum value was accepted")
			}
		})
	}
}

func TestDocumentEvidenceCitationPlacement(t *testing.T) {
	plan := validReadyPlan()
	content := "bounded evidence"
	plan.Items[0].Block.Kind = BlockDocumentEvidence
	plan.Items[0].Block.ContentSnapshot = &content
	plan.Items[0].CitationKey = stringPointer("C1")
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}

	plan.Items[0].CitationKey = stringPointer("C0")
	if err := plan.Validate(); err == nil {
		t.Fatal("invalid citation key was accepted")
	}
}

func TestProfileIndexValidateAndTransition(t *testing.T) {
	one, two := uint64(1), uint64(2)
	provisioning := ProfileIndex{State: ProfileIndexProvisioning, TargetGeneration: &one}
	readyOne := ProfileIndex{State: ProfileIndexReady, ActiveGeneration: &one}
	rebuilding := ProfileIndex{State: ProfileIndexRebuilding, ActiveGeneration: &one, TargetGeneration: &two}
	readyTwo := ProfileIndex{State: ProfileIndexReady, ActiveGeneration: &two}

	for _, index := range []ProfileIndex{provisioning, readyOne, rebuilding, readyTwo} {
		if err := index.Validate(); err != nil {
			t.Fatalf("valid index %#v: %v", index, err)
		}
	}
	if err := provisioning.ValidateTransition(readyOne); err != nil {
		t.Fatal(err)
	}
	if err := readyOne.ValidateTransition(rebuilding); err != nil {
		t.Fatal(err)
	}
	if err := rebuilding.ValidateTransition(readyTwo); err != nil {
		t.Fatal(err)
	}
	if err := rebuilding.ValidateTransition(readyOne); err == nil {
		t.Fatal("healthy rebuild rollback discarded its safe error fact")
	}
	rebuildError := ErrCodeIndexFailed
	readyAfterFailedRebuild := ProfileIndex{State: ProfileIndexReady, ActiveGeneration: &one, ErrorCode: &rebuildError}
	if err := rebuilding.ValidateTransition(readyAfterFailedRebuild); err != nil {
		t.Fatalf("healthy rebuild rollback with safe error fact: %v", err)
	}

	invalidShapes := []ProfileIndex{
		{State: ProfileIndexProvisioning, ActiveGeneration: &one, TargetGeneration: &two},
		{State: ProfileIndexReady, TargetGeneration: &one},
		{State: ProfileIndexRebuilding, ActiveGeneration: &two, TargetGeneration: &one},
		{State: ProfileIndexFailed},
	}
	for _, index := range invalidShapes {
		if err := index.Validate(); err == nil {
			t.Fatalf("invalid index shape accepted: %#v", index)
		}
	}
	if err := readyOne.ValidateTransition(readyTwo); err == nil {
		t.Fatal("ready generation changed without rebuilding")
	}
	wrongFailure := ErrCodeRetrievalFailed
	if err := readyOne.ValidateTransition(ProfileIndex{State: ProfileIndexFailed, ActiveGeneration: &one, ErrorCode: &wrongFailure}); err == nil {
		t.Fatal("ready profile failed without an index consistency error")
	}
	inconsistent := ErrCodeIndexInconsistent
	failed := ProfileIndex{State: ProfileIndexFailed, ActiveGeneration: &one, TargetGeneration: &two, ErrorCode: &inconsistent}
	three := uint64(3)
	if err := failed.ValidateTransition(ProfileIndex{State: ProfileIndexRebuilding, ActiveGeneration: &one, TargetGeneration: &three}); err != nil {
		t.Fatalf("strictly newer repair generation: %v", err)
	}
	if err := failed.ValidateTransition(ProfileIndex{State: ProfileIndexRebuilding, ActiveGeneration: &one, TargetGeneration: &two}); err == nil {
		t.Fatal("failed profile reused a non-increasing target generation")
	}
	if err := rebuilding.ValidateTransition(ProfileIndex{State: ProfileIndexReady, ActiveGeneration: &three}); err == nil {
		t.Fatal("rebuild activated a generation other than active or target")
	}
}

func TestPlanErrorRejectsUnregisteredPersistedMessage(t *testing.T) {
	secret := "dial provider with secret-key"
	planError := PlanError{Stage: "retrieval", Code: ErrCodeRetrievalFailed, Message: &secret}
	if err := planError.Validate(); err == nil {
		t.Fatal("unregistered persisted error message was accepted")
	}

	safe, err := NewPlanError("retrieval", ErrCodeRetrievalFailed)
	if err != nil {
		t.Fatal(err)
	}
	if safe.Message == nil || strings.Contains(*safe.Message, "secret-key") {
		t.Fatalf("safe plan error = %#v", safe)
	}
	if err := safe.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestFixedScoreNormalizesAndRejectsNonFiniteValues(t *testing.T) {
	score, err := ParseFixedScore("1.25")
	if err != nil {
		t.Fatal(err)
	}
	if score.String() != "1.250000" {
		t.Fatalf("normalized score = %q", score.String())
	}
	if _, err := ParseFixedScore("1.0000001"); err == nil {
		t.Fatal("more than six fractional digits were accepted")
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := FixedScoreFromFloat64(value); err == nil {
			t.Fatalf("non-finite score %v was accepted", value)
		}
	}
}

func TestSHA256FromBytesRequiresExactLength(t *testing.T) {
	if _, err := SHA256FromBytes(make([]byte, sha256.Size-1)); err == nil {
		t.Fatal("short hash was accepted")
	}
	want := sha256.Sum256([]byte("context"))
	got, err := SHA256FromBytes(want[:])
	if err != nil || got != want {
		t.Fatalf("hash = %x, err = %v", got, err)
	}
}

func TestClosedJSONAndContextErrorDoNotLeakAdapterDetails(t *testing.T) {
	var metrics ContextPlanMetricsV1
	if err := strictJSONDecode(`{"schema":"context_plan_metrics_v1","unexpected":true}`, &metrics); err == nil {
		t.Fatal("unknown metrics field was accepted")
	}
	if err := strictJSONDecode(`{"schema":"context_plan_metrics_v1"} trailing`, &metrics); err == nil {
		t.Fatal("trailing JSON was accepted")
	}

	cause := errors.New("dial provider with secret-key")
	appError, err := NewContextAppError(ErrCodeRetrievalFailed, cause)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(appError, cause) {
		t.Fatal("adapter cause was not wrapped")
	}
	if strings.Contains(appError.Error(), "secret-key") {
		t.Fatalf("public error leaked cause: %q", appError.Error())
	}
	if _, err := NewContextAppError(ErrorCode("ai.context.unknown"), nil); err == nil {
		t.Fatal("unknown context error code was accepted")
	}
}

func validReadyPlan() ContextPlan {
	inputHash := sha256.Sum256([]byte("input"))
	planHash := sha256.Sum256([]byte("plan"))
	modelHash := sha256.Sum256([]byte("model"))
	sourceHash := sha256.Sum256([]byte("message:1"))
	content := "hello"
	return ContextPlan{
		RunID:                  44,
		PolicyVersion:          "context_policy_v1",
		InputFingerprintSHA256: inputHash,
		PlanSHA256:             &planHash,
		ModelCapabilitySHA256:  modelHash,
		APIProtocol:            APIProtocolResponses,
		TokenCounterID:         "utf8_bytes_v1",
		Budget: Budget{
			ContextWindowTokens:          1000,
			EffectiveOutputTokens:        100,
			ProviderProtocolUpperBound:   50,
			ToolContinuationInputReserve: 25,
			PolicySafetyMargin:           50,
			KnownInputBudget:             800,
			KnownInputUpperBound:         10,
			Proof:                        BudgetExact,
		},
		RetrievalOutcome: RetrievalSkipped,
		State:            PlanReady,
		Metrics:          ContextPlanMetricsV1{Schema: ContextPlanMetricsSchemaV1},
		Items: []ContextPlanItem{{
			Ordinal: 1,
			Block: ContextBlock{
				Kind:            BlockCurrentUserMessage,
				SourceType:      "message",
				SourceRef:       "message:1",
				SourceSHA256:    sourceHash,
				AtomicGroupKey:  "turn:1",
				Required:        true,
				Priority:        100,
				TokenUpperBound: 10,
				ContentSnapshot: &content,
				Metadata:        ContextBlockMetadataV1{Schema: ContextBlockMetadataSchemaV1},
			},
			Decision: DecisionSelected,
		}},
	}
}

func stringPointer(value string) *string { return &value }
